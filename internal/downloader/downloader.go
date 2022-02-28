//go:build linux || darwin || freebsd
// +build linux darwin freebsd

package downloader

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	semaphore "github.com/marusama/semaphore/v2"

	"github.com/abihf/gonzb/internal/decoder/yenc"
	"github.com/abihf/gonzb/internal/nntp"
	"github.com/abihf/gonzb/internal/nzb"
	"github.com/abihf/gonzb/internal/reporter"
	"github.com/rs/zerolog/log"
	syscall "golang.org/x/sys/unix"
)

type Downloader struct {
	reporter *reporter.Reporter
	clients  []*nntp.Client
	queue    Queue

	mutex sync.Mutex
	sem   semaphore.Semaphore

	resizer  bool
	workerId int32
	qEmpty   chan bool
}

func New(conf *Config) *Downloader {
	d := &Downloader{qEmpty: make(chan bool)}

	qSize := 0
	for _, s := range conf.Servers {
		qSize += s.MaxConn
		d.clients = append(d.clients, nntp.New(s))
	}
	d.sem = semaphore.New(qSize)

	return d
}

func (d *Downloader) Download(ctx context.Context, n *nzb.Nzb) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	errChan := make(chan error)
	go func() {
		var wg sync.WaitGroup
		for _, file := range n.Files {
			if strings.HasSuffix(file.FileName(), ".par2") {
				continue
			}
			done := make(chan bool)
			wg.Add(1)
			go func(file *nzb.File) {
				defer wg.Done()
				d.downloadFile(ctx, file, done, errChan)
			}(file)
			<-done
		}
		wg.Wait()
		close(errChan)
	}()

	for {
		select {
		case err := <-errChan:
			return err
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func (d *Downloader) downloadFile(ctx context.Context, nzbFile *nzb.File, done chan bool, errChan chan error) {
	fileName := nzbFile.FileName()
	logger := log.With().Str("file", fileName).Logger()
	file, err := os.Create("files/" + fileName)
	if err != nil {
		errChan <- fmt.Errorf("can not create file %s: %w", fileName, err)
		return
	}
	defer file.Close()
	var size int64

	for _, s := range nzbFile.Segments {
		size += int64(s.Bytes)
	}

	err = file.Truncate(size)
	if err != nil {
		errChan <- fmt.Errorf("can not grow file %s to %d: %w", fileName, size, err)
		return
	}

	buff, err := syscall.Mmap(int(file.Fd()), 0, int(size), syscall.PROT_READ|syscall.PROT_WRITE, syscall.MAP_SHARED)
	if err != nil {
		errChan <- fmt.Errorf("can not mmap file %s: %w", fileName, err)
		return
	}
	defer syscall.Munmap(buff)
	defer syscall.Msync(buff, syscall.MS_SYNC)

	var offset uint32
	var wg sync.WaitGroup
	wg.Add(len(nzbFile.Segments))

	for _, segment := range nzbFile.Segments {
		slog := logger.With().Uint32("seg", segment.Number).Logger()
		err := d.sem.Acquire(ctx, 1)
		if err != nil {
			close(done)
			errChan <- fmt.Errorf("can not acquire semaphore for %s[%d]: %w", fileName, segment.Number, err)
			return
		}

		slog.Debug().Msg("queueing")
		j := &Job{
			Name:    fileName,
			Groups:  nzbFile.Groups,
			Segment: segment,
			Decode:  yenc.Decode,
			Buff:    buff,

			OnDone: func(e error) {
				d.sem.Release(1)
				wg.Done()
				if err != nil {
					errChan <- e
				}
			},
		}
		d.queue.Add(j)
		d.GrowWorker()
		offset += segment.Bytes
	}

	close(done)
	wg.Wait()
	logger.Info().Msg("file downloaded")
}

func (d *Downloader) GrowWorker() {
	d.mutex.Lock()
	defer d.mutex.Unlock()

	if d.resizer || d.queue.Len() == 0 {
		return
	}

	d.resizer = true
	go d.runResizer()

	ticker := time.NewTicker(10 * time.Second)
	go func() {
		for {
			<-ticker.C
			if !d.runResizer() {
				ticker.Stop()
				return
			}
		}
	}()
}

func (d *Downloader) runResizer() bool {
	d.mutex.Lock()
	qLen := d.queue.Len()
	if qLen <= 0 {
		d.resizer = false
		d.mutex.Unlock()
		d.qEmpty <- true
		return false
	}
	d.mutex.Unlock()

	for _, c := range d.clients {
		qLen -= c.BusyLen()
	}

	cLen := len(d.clients)
	lastIndex := 0
	for qLen > 0 {
		batch := qLen / cLen
		if batch < 1 {
			batch = 1
		}
		for i := 0; i < cLen; i++ {
			c := d.clients[lastIndex]
			lastIndex = (lastIndex + 1) % cLen

			conns, errs := c.ConnectN(batch)
			qLen -= len(conns)
			for _, err := range errs {
				if err != nil {
					log.Error().Stack().Err(err).Str("host", c.Host).Msg("connection failed")
				}
			}
			for _, conn := range conns {
				id := atomic.AddInt32(&d.workerId, 1)
				worker := &Worker{id, conn}
				log.Debug().Int32("id", id).Str("host", c.Host).Msg("creating worker")
				go worker.Consume(&d.queue)
			}
		}
	}
	return true
}
