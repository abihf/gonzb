package batch

import (
	"sync"
	"sync/atomic"
)

type Batch struct {
	wg      sync.WaitGroup
	errChan chan error
	err     int32
}

func New() *Batch {
	return &Batch{errChan: make(chan error, 1)}
}

func (b *Batch) Run(fn func() error) {
	b.wg.Add(1)
	go func() {
		defer b.wg.Done()
		err := fn()
		if err != nil && atomic.CompareAndSwapInt32(&b.err, 0, 1) {
			b.errChan <- err
		}
	}()
}

func (b *Batch) Wait() error {
	done := make(chan any)
	go func() {
		b.wg.Wait()
		done <- 0
	}()
	select {
	case err := <-b.errChan:
		return err
	case <-done:
	}
	return nil
}
