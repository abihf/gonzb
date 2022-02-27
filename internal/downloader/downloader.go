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

	semaphore "github.com/marusama/semaphore/v2"

	"github.com/abihf/gonzb/internal/decoder/yenc"
	"github.com/abihf/gonzb/internal/nntp"
	"github.com/abihf/gonzb/internal/nzb"
	"github.com/abihf/gonzb/internal/reporter"
	syscall "golang.org/x/sys/unix"
)

type Downloader struct {
	r *reporter.Reporter
	c []*nntp.Client
	q Queue
	m sync.Mutex

	workerId int32
	sem      semaphore.Semaphore

	// mmapSem semaphore.Semaphore

}

func New(conf *Config) *Downloader {
	d := &Downloader{}

	qSize := 0
	for _, s := range conf.Servers {
		qSize += s.MaxConn
		d.c = append(d.c, nntp.New(s))
	}
	d.sem = semaphore.New(qSize)

	return d
}

func (d *Downloader) Download(ctx context.Context, n *nzb.Nzb) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	nd := nzbDownloader{
		Downloader: d,
		n:          n,
		err:        make(chan error),
	}

	return nd.download(ctx)
}

type nzbDownloader struct {
	*Downloader
	n   *nzb.Nzb
	err chan error
}

func (d *nzbDownloader) download(ctx context.Context) error {
	go func() {
		for _, file := range d.n.Files {
			if strings.HasSuffix(file.FileName(), ".par2") {
				continue
			}
			c := make(chan bool)
			go d.downloadFile(ctx, file, c)
			<-c
		}
		close(d.err)
	}()

	for {
		select {
		case err := <-d.err:
			return err

		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func (d *nzbDownloader) downloadFile(ctx context.Context, nzbFile *nzb.File, c chan bool) {
	fileName := nzbFile.FileName()
	file, err := os.Create("files/" + fileName)
	if err != nil {
		d.err <- fmt.Errorf("can not create file %s: %w", fileName, err)
		return
	}
	defer file.Close()
	var size int64

	for _, s := range nzbFile.Segments {
		size += int64(s.Bytes)
	}

	err = file.Truncate(size)
	if err != nil {
		d.err <- fmt.Errorf("can not grow file %s to %d: %w", fileName, size, err)
		return
	}

	buff, err := syscall.Mmap(int(file.Fd()), 0, int(size), syscall.PROT_READ|syscall.PROT_WRITE, syscall.MAP_SHARED)
	if err != nil {
		d.err <- fmt.Errorf("can not mmap file %s: %w", fileName, err)
		return
	}
	defer syscall.Munmap(buff)
	defer syscall.Msync(buff, syscall.MS_SYNC)

	var offset uint32
	var wg sync.WaitGroup
	wg.Add(len(nzbFile.Segments))

	for _, segment := range nzbFile.Segments {
		err := d.sem.Acquire(ctx, 1)
		if err != nil {
			close(c)
			d.err <- fmt.Errorf("can not acquire semaphore for %s[%d]: %w", fileName, segment.Number, err)
			return
		}

		fmt.Printf("queueing %s[%d]\n", fileName, segment.Number)
		j := &Job{
			Name:    fileName,
			Groups:  nzbFile.Groups,
			Segment: segment,
			Decode:  yenc.Decode,
			Buff:    buff[offset : offset+segment.Bytes],

			OnDone: func(e error) {
				d.sem.Release(1)
				wg.Done()
				if err != nil {
					d.err <- e
				}
			},
		}
		d.q.Add(j)
		go d.GrowWorker()
		offset += segment.Bytes
	}

	close(c)
	wg.Wait()
}

func (d *Downloader) GrowWorker() {
	d.m.Lock()
	defer d.m.Unlock()

	qLen := d.q.Len()

	for _, c := range d.c {
		// todo calculate weight
		size := qLen / len(d.c)
		if size < 1 {
			size = 1
		}
		conns, errs := c.ConnectN(size)
		for _, err := range errs {
			if err != nil {
				fmt.Printf("connection failed: %v\n", err)
			}
		}
		for _, conn := range conns {
			id := atomic.AddInt32(&d.workerId, 1)
			worker := &Worker{id, conn}
			fmt.Printf("creating worker %d from %s\n", id, c.Host)
			go worker.Consume(&d.q)
		}
	}
}
