package telegram

import "sync"

type UserContext map[string]string

type UserContexts struct {
	store map[int]UserContext
	mu    sync.RWMutex
}

func (c *UserContexts) Set(userID int, key, value string) {
	c.mu.RLock()
	if c.store == nil {
		c.store = make(map[int]UserContext)
	}
	if c.store[userID] == nil {
		c.store[userID] = make(UserContext)
	}
	c.mu.RUnlock()

	c.mu.Lock()
	c.store[userID][key] = value
	c.mu.Unlock()
}
func (c *UserContexts) Get(userID int, key string) string {
	c.mu.RLock()
	if c.store == nil || c.store[userID] == nil {
		return ""
	}

	value := c.store[userID][key]

	c.mu.RUnlock()
	return value
}
