package berth

import "sync"

type Cache struct {
	mu      sync.RWMutex
	entries map[string]State
}

func NewCache() *Cache {
	return &Cache{entries: make(map[string]State)}
}

func (c *Cache) Put(id string, st State) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[id] = st
}

func (c *Cache) Get(id string) (State, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	st, ok := c.entries[id]
	return st, ok
}

func (c *Cache) Reset(entries map[string]State) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = make(map[string]State, len(entries))
	for id, st := range entries {
		c.entries[id] = st
	}
}
