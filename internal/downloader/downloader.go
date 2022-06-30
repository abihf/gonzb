//go:build linux || darwin || freebsd
// +build linux darwin freebsd

package downloader

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"sync/atomic"

	"github.com/abihf/gonzb/internal/batch"
	"github.com/abihf/gonzb/internal/decoder/yenc"
	"github.com/abihf/gonzb/internal/nntp"
	"github.com/abihf/gonzb/internal/nzb"
	"github.com/abihf/gonzb/internal/reporter"
	syscall "golang.org/x/sys/unix"
)

type Downloader struct {
	reporter *reporter.Reporter
	client   *nntp.Client

	// mutex sync.Mutex
	// sem semaphore.Semaphore
	sem chan struct{}

	// resizer  bool
	// workerId int32
	// qEmpty   chan bool
}

func New(conf *Config) *Downloader {
	d := &Downloader{client: nntp.New(conf.Servers[0])}

	return d
}

func (d *Downloader) Download(ctx context.Context, n *nzb.Nzb) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	b := batch.New()
	// errChan := make(chan error)

	// go func() {
	for _, file := range n.Files {
		if strings.HasSuffix(file.FileName(), ".par2") {
			continue
		}
		b.Run(d.downloadFile(ctx, file))
		for range file.Segments {
			d.sem <- struct{}{}
		}
	}
	return b.Wait()
	// 	close(errChan)
	// }()

	// for {
	// 	select {
	// 	case err := <-errChan:
	// 		return err
	// 	case <-ctx.Done():
	// 		return ctx.Err()
	// 	}
	// }
}

func (d *Downloader) downloadFile(ctx context.Context, nzbFile *nzb.File) func() error {
	return func() error {
		semCount := int32(len(nzbFile.Segments))
		defer func() {
			for i := int32(0); i < semCount; i++ {
				<-d.sem
			}
		}()

		fileName := nzbFile.FileName()
		// logger := log.With().Str("file", fileName).Logger()
		file, err := os.Create("files/" + fileName)
		if err != nil {
			return fmt.Errorf("can not create file %s: %w", fileName, err)
		}
		defer file.Close()

		var size int64
		for _, s := range nzbFile.Segments {
			size += int64(s.Bytes)
		}

		err = file.Truncate(size)
		if err != nil {
			return fmt.Errorf("can not grow file %s to %d: %w", fileName, size, err)
		}

		buff, err := syscall.Mmap(int(file.Fd()), 0, int(size), syscall.PROT_READ|syscall.PROT_WRITE, syscall.MAP_SHARED)
		if err != nil {
			return fmt.Errorf("can not mmap file %s: %w", fileName, err)
		}
		defer syscall.Msync(buff, syscall.MS_ASYNC)
		defer syscall.Munmap(buff)

		b := batch.New()
		for _, segment := range nzbFile.Segments {
			id := segment.ID
			b.Run(func() error {
				defer func() {
					atomic.AddInt32(&semCount, -1)
					<-d.sem
				}()

				return d.client.Download(ctx, nzbFile.Groups, id, func(r io.Reader) error {
					return yenc.Decode(buff, r)
				})
			})
		}

		return b.Wait()
	}
}

// func (d *Downloader) GrowWorker() {
// 	d.mutex.Lock()
// 	defer d.mutex.Unlock()

// 	if d.resizer || d.queue.Len() == 0 {
// 		return
// 	}

// 	d.resizer = true
// 	go d.runResizer()

// 	ticker := time.NewTicker(10 * time.Second)
// 	go func() {
// 		for {
// 			<-ticker.C
// 			if !d.runResizer() {
// 				ticker.Stop()
// 				return
// 			}
// 		}
// 	}()
// }

// func (d *Downloader) runResizer() bool {
// 	d.mutex.Lock()
// 	qLen := d.queue.Len()
// 	if qLen <= 0 {
// 		d.resizer = false
// 		d.mutex.Unlock()
// 		d.qEmpty <- true
// 		return false
// 	}
// 	d.mutex.Unlock()

// 	for _, c := range d.clients {
// 		qLen -= c.BusyLen()
// 	}

// 	cLen := len(d.clients)
// 	lastIndex := 0
// 	for qLen > 0 {
// 		batch := qLen / cLen
// 		if batch < 1 {
// 			batch = 1
// 		}
// 		for i := 0; i < cLen; i++ {
// 			c := d.clients[lastIndex]
// 			lastIndex = (lastIndex + 1) % cLen

// 			conns, errs := c.ConnectN(batch)
// 			qLen -= len(conns)
// 			for _, err := range errs {
// 				if err != nil {
// 					log.Error().Stack().Err(err).Str("host", c.Host).Msg("connection failed")
// 				}
// 			}
// 			for _, conn := range conns {
// 				id := atomic.AddInt32(&d.workerId, 1)
// 				worker := &Worker{id, conn}
// 				log.Debug().Int32("id", id).Str("host", c.Host).Msg("creating worker")
// 				go worker.Consume(&d.queue)
// 			}
// 		}
// 	}
// 	return true
// }
