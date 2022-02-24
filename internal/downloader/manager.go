package downloader

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"

	"github.com/abihf/gonzb/internal/decoder"
	"github.com/abihf/gonzb/internal/nntp"
	"github.com/abihf/gonzb/internal/nzb"
	"github.com/abihf/gonzb/internal/reporter"
)

type Manager struct {
	tb       TempBuff
	r        *reporter.Reporter
	clients  []*nntp.Client
	jobQueue chan *Job
	mutex    sync.Mutex
}

func NewManager(r *reporter.Reporter) *Manager {
	return &Manager{
		r: r,
		tb: TempBuff{
			MaxSize: 2 ^ 30,
		},
		jobQueue: make(chan *Job),
	}
}

type fileWithResult struct {
	nzb.File
	segments []segmentWithResult
}

type segmentWithResult struct {
	nzb.Segment
	buff ReadWriteSeekCloser
	job  *Job
}

func (m *Manager) Download(ctx context.Context, n *nzb.Nzb) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var wg sync.WaitGroup
	var err error
	for _, file := range n.Files {
		wg.Add(1)
		go func(file *nzb.File) {
			defer wg.Done()
			dErr := m.downloadFile(ctx, file)
			if dErr != nil {
				err = dErr
				cancel()
			}
		}(file)
	}
	wg.Wait()

	return err
}

func (m *Manager) downloadFile(ctx context.Context, nzbFile *nzb.File) error {
	file, err := os.Create(nzbFile.FileName())
	if err != nil {
		return err
	}
	defer file.Close()
	var size uint32
	for _, s := range nzbFile.Segments {
		size += s.Bytes
	}
	file.Truncate(int64(size))

	var wg sync.WaitGroup
	var mutex sync.Mutex
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	setErr := func(e error) {
		err = e
		cancel()
	}

	var offset uint32
	for _, segment := range nzbFile.Segments {
		wg.Add(1)
		go func(s *nzb.Segment, offset uint32) {
			defer wg.Done()

			buff, tbErr := m.tb.Allocate(s.ID, int(s.Bytes))
			if tbErr != nil {
				setErr(tbErr)
				return
			}
			defer buff.Close()

			job := Job{
				Ctx:     ctx,
				Group:   nzbFile.Groups[0],
				Article: s.ID,
				done:    make(chan struct{}),
				Decoder: decoder.Yenc,
				Writer:  buff,
			}
			m.jobQueue <- &job
			<-job.done
			if job.Err != nil {
				setErr(job.Err)
				return
			}

			_, sErr := buff.Seek(0, io.SeekStart)
			if sErr != nil {
				setErr(sErr)
				return
			}

			mutex.Lock()
			defer mutex.Unlock()

			file.Seek(int64(offset), io.SeekStart)
			_, cErr := io.CopyN(file, buff, int64(s.Bytes))
			if cErr != nil {
				setErr(cErr)
				return
			}

		}(segment, offset)
		offset += segment.Bytes
	}

	wg.Wait()

	return err
}

func (m *Manager) GrowWorker(ctx context.Context) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	count := len(m.jobQueue)
	for _, c := range m.clients {
		count -= c.Len()
		if count <= 0 {
			break
		}
	}
	for i := 0; i < count; i++ {
		ok := false
		for j := 0; j < len(m.clients); j++ {
			clientIndex := (j + i) % len(m.clients)
			conn, err := m.clients[clientIndex].Connect(ctx)
			if err != nil {
				if !errors.Is(err, nntp.ErrLimitExceeded) {
					fmt.Printf("Error connecting client %v", err)
				}
				continue
			}
			worker := &Worker{conn}
			go worker.Consume(m.jobQueue)
			ok = true
			break
		}
		if !ok {
			// can't grow any more
			return
		}
	}
}
