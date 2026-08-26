package grid

import (
	"errors"
	"sync"
)

type Breaker interface {
	Close() error
	Open() error
	IsClosed() bool
}

var (
	errCloseFault = errors.New("breaker close fault")
)

type SimpleBreaker struct {
	mu        sync.Mutex
	closed    bool
	failClose bool
}

func NewSimpleBreaker() *SimpleBreaker {
	return &SimpleBreaker{}
}

func (b *SimpleBreaker) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.failClose {
		return errCloseFault
	}
	b.closed = true
	return nil
}

func (b *SimpleBreaker) Open() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.closed = false
	return nil
}

func (b *SimpleBreaker) IsClosed() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.closed
}

func (b *SimpleBreaker) SetCloseFault(fault bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.failClose = fault
}
