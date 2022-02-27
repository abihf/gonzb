package downloader

import (
	"sync"

	"github.com/abihf/gonzb/internal/decoder"
	"github.com/abihf/gonzb/internal/nzb"
)

type Job struct {
	Name string
	// Ctx     context.Context
	Groups  []string
	Segment *nzb.Segment
	Decode  decoder.Decoder
	Buff    []byte
	// Offset  uint32
	// Done    chan error

	OnDone func(error)

	cancelled bool
}

type queueItem struct {
	value *Job
	next  *queueItem
}

type Queue struct {
	m     sync.Mutex
	first *queueItem
	last  *queueItem
	len   int
}

func (q *Queue) Add(j *Job) {
	q.m.Lock()
	defer q.m.Unlock()

	item := &queueItem{j, nil}
	if q.first == nil {
		q.first = item
	}
	if q.last != nil {
		q.last.next = item
	}
	q.len++
	q.last = item
}

func (q *Queue) Get() *Job {
	q.m.Lock()
	defer q.m.Unlock()

	item := q.first
	if item == nil {
		return nil
	}
	q.first = item.next
	if item == q.last {
		q.last = nil
	}
	q.len--
	return item.value
}

func (q *Queue) Len() int {
	q.m.Lock()
	defer q.m.Unlock()
	return q.len
}
