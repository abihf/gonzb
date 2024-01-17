package downloader

import (
	"context"
	"fmt"
	"os"
	"sync"

	"github.com/abihf/gonzb/internal/decoder/yenc"
	"github.com/abihf/gonzb/internal/nntp"
	"github.com/abihf/gonzb/internal/nzb"
	syscall "golang.org/x/sys/unix"
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
		worker := &FileWorker{d: d, req: req}
		worker.mu.Lock()
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
	d        *Downloader
	fileName string
	req      *DownloadFileRequest
	buff     []byte
	wg       sync.WaitGroup
	mu       sync.Mutex
}

func (w *FileWorker) process() {
	w.req.errCh <- w.processE()
}

func (w *FileWorker) processE() error {
	fileName := w.req.Folder + "/" + w.req.Info.FileName()
	w.fileName = fileName
	var totalSize int64
	for _, segment := range w.req.Info.Segments {
		totalSize += segment.Bytes
	}

	w.d.reporter.Init(fileName, totalSize)
	file, err := os.Create(fileName)
	if err != nil {
		return fmt.Errorf("can not create file %s: %w", fileName, err)
	}
	defer file.Close()

	err = file.Truncate(totalSize)
	if err != nil {
		return fmt.Errorf("can not grow file %s to %d: %w", fileName, totalSize, err)
	}

	buff, err := syscall.Mmap(int(file.Fd()), 0, int(totalSize), syscall.PROT_READ|syscall.PROT_WRITE, syscall.MAP_SHARED)
	if err != nil {
		return fmt.Errorf("can not mmap file %s: %w", fileName, err)
	}
	w.buff = buff
	defer syscall.Munmap(buff)
	defer syscall.Msync(buff, syscall.MS_SYNC)

	w.mu.Unlock()
	w.wg.Wait()
	w.d.reporter.Done(fileName)
	return nil
}

func (w *FileWorker) handleBody(body nntp.Body) error {
	defer w.wg.Done()
	return yenc.Decode(w, body)
}

// WriteAt implements io.WriterAt.
func (w *FileWorker) WriteAt(p []byte, off int64) (n int, err error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.d.reporter.Increment(w.fileName, int64(len(p)))
	return copy(w.buff[off:], p), nil
}
