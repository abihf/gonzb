//go:build linux || darwin || freebsd
// +build linux darwin freebsd

package downloader

import (
	"context"
	"strings"

	semaphore "github.com/marusama/semaphore/v2"

	"github.com/abihf/gonzb/internal/nntp"
	"github.com/abihf/gonzb/internal/nzb"
	"github.com/abihf/gonzb/internal/reporter"
)

type Downloader struct {
	reporter *reporter.Reporter
	clients  []*nntp.Client

	sem semaphore.Semaphore

	resizer   bool
	workerId  int32
	fileCh    chan *DownloadFileRequest
	articleCh chan *nntp.ArticleRequest
}

func New(conf *Config) *Downloader {
	d := &Downloader{articleCh: make(chan *nntp.ArticleRequest), fileCh: make(chan *DownloadFileRequest)}

	qSize := 0
	for _, s := range conf.Servers {
		qSize += s.MaxConn
		client := nntp.New(&s)
		client.StartWorker(d.articleCh)
		d.clients = append(d.clients, client)
	}
	d.sem = semaphore.New(500_000_000)
	go d.fileWorker()
	return d
}

func (d *Downloader) Download(ctx context.Context, n *nzb.Nzb) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	errChan := make(chan error, len(n.Files))
	for _, file := range n.Files {
		if strings.HasSuffix(file.FileName(), ".par2") {
			continue
		}
		select {
		case d.fileCh <- &DownloadFileRequest{Info: file, errCh: errChan, Folder: "out"}:
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	select {
	case err := <-errChan:
		if err != nil {
			return err
		}
	case <-ctx.Done():
		return ctx.Err()
	}

	return nil
}
