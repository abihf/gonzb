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

type Fn func() error

func (b *Batch) Run(fn Fn) {
	b.wg.Add(1)
	go func() {
		defer b.wg.Done()
		err := fn()
		if err != nil && atomic.CompareAndSwapInt32(&b.err, 0, 1) {
			b.errChan <- err
			close(b.errChan)
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

func All[Arg any](args []Arg, fn func(Arg) error) error {
	b := New()
	for _, arg := range args {
		b.Run(func() error {
			return fn(arg)
		})
	}
	return b.Wait()
}
