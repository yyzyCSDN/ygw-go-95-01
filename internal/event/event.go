package event

import (
	"errors"
	"sync"
)

var errClosed = errors.New("event bus is closed")

type Event struct {
	Topic   string
	ID      string
	Payload map[string]string
}

type Handler func(Event)

type Bus struct {
	mu      sync.RWMutex
	subs    map[string][]Handler
	closed  bool
	pending sync.WaitGroup
}

func NewBus() *Bus {
	return &Bus{subs: make(map[string][]Handler)}
}

func (b *Bus) Subscribe(topic string, h Handler) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return errClosed
	}
	b.subs[topic] = append(b.subs[topic], h)
	return nil
}

func (b *Bus) Publish(ev Event) {
	b.mu.RLock()
	handlers := append([]Handler(nil), b.subs[ev.Topic]...)
	b.mu.RUnlock()
	for _, h := range handlers {
		b.pending.Add(1)
		go func(handler Handler, event Event) {
			defer b.pending.Done()
			handler(event)
		}(h, ev)
	}
}

func (b *Bus) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return errClosed
	}
	b.closed = true
	b.subs = make(map[string][]Handler)
	return nil
}
