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

	sem chan struct{}
}

func New(conf *Config) *Downloader {
	d := &Downloader{client: nntp.New(conf.Servers[0])}

	return d
}

func (d *Downloader) Close() error {
	return d.client.Close()
}

func (d *Downloader) Download(ctx context.Context, n *nzb.Nzb) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	b := batch.New()
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
}

func (d *Downloader) downloadFile(ctx context.Context, nzbFile *nzb.File) func() error {
	return func() error {
		semCount := int32(len(nzbFile.Segments))
		defer func() {
			for atomic.LoadInt32(&semCount) > 0 {
				atomic.AddInt32(&semCount, -1)
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
