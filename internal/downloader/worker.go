package downloader

import (
	"context"
	"io"

	"github.com/abihf/gonzb/internal/decoder"
	"github.com/abihf/gonzb/internal/nntp"
)

type Worker struct {
	conn *nntp.Conn
}

func (w *Worker) Close() error {
	return w.conn.Close()
}

type Job struct {
	Ctx     context.Context
	Group   string
	Article string
	Decoder decoder.Decoder
	Writer  io.WriteCloser

	cancelled bool
	done      chan struct{}
	Err       error
}

func (w *Worker) Consume(c chan *Job) {
	for {
		select {
		case <-w.conn.Closed:
			return

		case job := <-c:
			if job == nil {
				return
			}
			go func() {
				job.Err = w.Process(job)
				close(job.done)
			}()
			select {
			case <-job.Ctx.Done():
				job.cancelled = true
			case <-job.done:
			}
		}
	}
}

func (w *Worker) Process(j *Job) error {
	if j.cancelled {
		return j.Ctx.Err()
	}
	_, err := w.conn.Group(j.Group)
	if err != nil {
		return err
	}

	if j.cancelled {
		return j.Ctx.Err()
	}
	reader, err := w.conn.Body(j.Article)
	if err != nil {
		return err
	}
	defer reader.Close()
	go func() {
		select {
		case <-j.Ctx.Done():
			reader.Close()
		case <-j.done:
			// do nothing
		}
	}()

	if j.cancelled {
		return j.Ctx.Err()
	}
	_, err = io.Copy(j.Writer, j.Decoder(reader))
	if err != nil {
		return err
	}
	return j.Writer.Close()
}
