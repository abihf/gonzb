package downloader

import (
	"context"
	"os"
	"sync"

	"github.com/abihf/gonzb/internal/decoder/yenc"
	"github.com/abihf/gonzb/internal/nntp"
	"github.com/abihf/gonzb/internal/nzb"
	"github.com/go-mmap/mmap"
)

type DownloadFileRequest struct {
	Info   *nzb.File
	Folder string
	errCh  chan error
}

func (d *Downloader) fileWorker() {
	for {
		req := <-d.fileCh
		if req == nil {
			return
		}
		worker := &FileWorker{d: d, req: req, ch: make(chan *filePart, 1)}
		go worker.process()

		for _, segment := range req.Info.Segments {
			d.sem.Acquire(context.Background(), int(segment.Bytes))
			worker.wg.Add(1)
			d.articleCh <- &nntp.ArticleRequest{
				Groups: req.Info.Groups,
				Id:     segment.ID,
				Handler: func(b nntp.Body) error {
					defer d.sem.Release(int(segment.Bytes))
					return worker.handleBody(b)
				},
			}
		}
	}
}

type FileWorker struct {
	d    *Downloader
	req  *DownloadFileRequest
	file *mmap.File
	wg   sync.WaitGroup
	ch   chan *filePart
	mu   sync.RWMutex
}

type filePart struct {
	buffer []byte
	offset int64
}

func (w *FileWorker) process() {
	w.mu.Lock()
	fileName := w.req.Folder + "/" + w.req.Info.FileName()
	var totalSize int64
	for _, segment := range w.req.Info.Segments {
		totalSize += segment.Bytes
	}

	_, err := os.Stat(fileName)
	if os.IsNotExist(err) {
		file, err := os.Create(fileName)
		if err != nil {
			w.req.errCh <- err
			return
		}
		file.Close()
	}

	err = os.Truncate(fileName, totalSize)
	if err != nil {
		w.req.errCh <- err
		return
	}

	w.file, err = mmap.OpenFile(fileName, mmap.Write)
	if err != nil {
		w.req.errCh <- err
		return
	}
	defer w.file.Close()
	defer w.file.Sync()

	w.mu.Unlock()

	w.wg.Wait()
	close(w.ch)
	w.req.errCh <- nil
}

func (w *FileWorker) handleBody(body nntp.Body) error {
	w.mu.RLock()
	defer w.mu.RUnlock()
	defer w.wg.Done()
	return yenc.Decode(w.file, body)
}
