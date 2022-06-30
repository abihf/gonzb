package pool

import (
	"context"
	"sync"
)

type Config[C any] struct {
	Factory func(context.Context) (C, error)
	Close   func(C) error
	Valid   func(C) bool

	MaxCon int32
}

type Pool[C any] struct {
	conf  *Config[C]
	conns chan C
	reqs  []chan C
	count int32
	mu    sync.Mutex
}

func New[C any](conf *Config[C]) *Pool[C] {
	return &Pool[C]{
		conns: make(chan C, conf.MaxCon),
		conf:  conf,
	}
}

func (p *Pool[C]) Get(ctx context.Context) (C, error) {
	for {
		select {
		case conn := <-p.conns:
			if p.conf.Valid != nil && !p.conf.Valid(conn) {
				p.mu.Lock()
				p.count--
				p.mu.Unlock()
				continue
			}
			return conn, nil

		default:
			p.mu.Lock()
			if p.count >= p.conf.MaxCon {
				req := make(chan C, 1)
				p.reqs = append(p.reqs, req)
				p.mu.Unlock()
				return <-req, nil
			}
			p.count++
			p.mu.Unlock()
			conn, err := p.conf.Factory(ctx)
			if err != nil {
				p.mu.Lock()
				p.count--
				p.mu.Unlock()
				return conn, err
			}
			return conn, nil
		}
	}
}

func (p *Pool[C]) Put(conn C) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.conf.Valid != nil && !p.conf.Valid(conn) {
		p.count--
		return
	}

	if len(p.reqs) == 0 {
		p.conns <- conn
		return
	}
	req := p.reqs[0]
	p.reqs = p.reqs[1:]
	req <- conn
}

func (p *Pool[C]) Release() {
	p.mu.Lock()
	defer p.mu.Unlock()

	for i := 0; i < len(p.conns); i++ {
		conn := <-p.conns
		p.conf.Close(conn)
	}
}
