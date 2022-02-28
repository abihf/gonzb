package downloader

import (
	"github.com/abihf/gonzb/internal/nntp"
	"github.com/rs/zerolog/log"
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
			log.Debug().Int32("id", w.id).Msg("no more job for worker, mark connection as idle")
			return
		}
		logger := log.With().Str("file", job.Name).Uint32("seg", job.Segment.Number).Str("server", w.conn.Name()).Logger()

		logger.Debug().Msg("downloading")
		job.OnDone(w.Process(job))
		logger.Debug().Msg("downloaded")
	}
}

func (w *Worker) Process(j *Job) error {
	var err error
	_, err = w.conn.Groups(j.Groups)
	if err != nil {
		return err
	}

	// if j.cancelled {
	// 	return j.Ctx.Err()
	// }
	return w.conn.Body("<"+j.Segment.ID+">", func(b nntp.Body) error {
		return j.Decode(j.Buff, b)
	})
}
