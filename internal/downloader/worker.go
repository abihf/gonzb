package downloader

import (
	"fmt"

	"github.com/abihf/gonzb/internal/nntp"
)

type Worker struct {
	id   int32
	conn *nntp.Conn
}

func (w *Worker) Close() error {
	return w.conn.Close()
}

func (w *Worker) Consume(q *Queue) {
	defer w.conn.Idle()
	for {
		job := q.Get()
		if job == nil {
			fmt.Printf("no more job for worker %d, mark connection as idle\n", w.id)
			return
		}

		fmt.Printf("downloading %s[%d] via %s\n", job.Name, job.Segment.Number, w.conn.Name())
		job.OnDone(w.Process(job))
		fmt.Printf("downloaded %s[%d]\n", job.Name, job.Segment.Number)
	}
}

func (w *Worker) Process(j *Job) error {
	ok := false
	var err error
	for _, group := range j.Groups {
		// if j.cancelled {
		// 	return j.Ctx.Err()
		// }
		_, err = w.conn.Group(group)
		if err == nil {
			ok = true
			break
		}
	}
	if !ok {
		return err
	}

	// if j.cancelled {
	// 	return j.Ctx.Err()
	// }
	return w.conn.Body("<"+j.Segment.ID+">", func(b nntp.Body) error {
		return j.Decode(j.Buff, b)
		// if err != nil {
		// 	return fmt.Errorf("failed to decode segment %d: %w", j.Segment.Number, err)
		// }
		// _, err = j.Writer.Seek(int64(j.Offset), io.SeekStart)
		// if err != nil {
		// 	return fmt.Errorf("can not seek to offset %d %w", j.Offset, err)
		// }
		// _, err = j.Writer.Write(j.Buff)
		// return err
	})
}
