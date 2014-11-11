// This package provides a simple LRU cache. It is based on the
// LRU implementation in groupcache:
// https://github.com/golang/groupcache/tree/master/lru
package lru

import (
	"container/list"
	"errors"
	"sync"
	"time"
)

// Cache is a thread-safe fixed size LRU cache.
type Cache struct {
	size      int
	evictList *list.List
	items     map[interface{}]*list.Element
	lock      sync.Mutex
	onRemove  func(interface{})
}

// entry is used to hold a value in the evictList
type entry struct {
	key   interface{}
	value interface{}
}

// New creates an LRU of the given size
func New(size int) (*Cache, error) {
	if size <= 0 {
		return nil, errors.New("Must provide a positive size")
	}
	onRemove := func(val interface{}) {
		return
	}
	c := &Cache{
		size:      size,
		evictList: list.New(),
		items:     make(map[interface{}]*list.Element, size),
		onRemove:  onRemove,
	}
	return c, nil
}

// Purge is used to completely clear the cache
func (c *Cache) Purge() {
	c.lock.Lock()
	defer c.lock.Unlock()            
	for c.Len() > 0 {
        c.removeOldest()
    }
	c.items = nil
	c.items = make(map[interface{}]*list.Element, c.size)
}

// Schedule routine purges. May be used to free up memory or 
// ensure that the OnRemove function is called within a certain
// amount of time.
func (cache *Cache) ScheduleClear(d time.Duration) {
    t := time.Tick(d)
    for {
        select {
        case <-t:
            cache.Purge()
        }
    }
}

// Add adds a value to the cache.
func (c *Cache) Add(key, value interface{}) {
	c.lock.Lock()
	defer c.lock.Unlock()

	// Check for existing item
	if ent, ok := c.items[key]; ok {
		c.evictList.MoveToFront(ent)
		ent.Value.(*entry).value = value
		return
	}

	// Add new item
	ent := &entry{key, value}
	entry := c.evictList.PushFront(ent)
	c.items[key] = entry

	// Verify size not exceeded
	if c.evictList.Len() > c.size {
		c.removeOldest()
	}
}

func (c *Cache) Update(key interface{}, f func(val interface{})) bool {
	ent, ok := c.items[key]
	if !ok {
		return false
	}
	f(ent.Value.(*entry).value)
	return true
}

// Get looks up a key's value from the cache.
func (c *Cache) Get(key interface{}) (value interface{}, ok bool) {
	c.lock.Lock()
	defer c.lock.Unlock()

	if ent, ok := c.items[key]; ok {
		c.evictList.MoveToFront(ent)
		return ent.Value.(*entry).value, true
	}
	return
}

// Remove removes the provided key from the cache.
func (c *Cache) Remove(key interface{}) {
	c.lock.Lock()
	defer c.lock.Unlock()

	if ent, ok := c.items[key]; ok {
		c.removeElement(ent)
	}
}

// Specify a function to run before deleting an item from the cache
func (c *Cache) OnRemove(f func(interface{})) {
	c.onRemove = f
}

// RemoveOldest removes the oldest item from the cache.
func (c *Cache) RemoveOldest() {
	c.lock.Lock()
	defer c.lock.Unlock()
	c.removeOldest()
}

// removeOldest removes the oldest item from the cache.
func (c *Cache) removeOldest() {
	ent := c.evictList.Back()
	if ent != nil {
	    c.removeElement(ent)
	    c.onRemove(ent.Value.(*entry).value)
	}
}

// removeElement is used to remove a given list element from the cache
func (c *Cache) removeElement(e *list.Element) {
	c.evictList.Remove(e)
	kv := e.Value.(*entry)
    c.items[kv.key] = nil  // force garbage collection
	delete(c.items, kv.key)
}

// Len returns the number of items in the cache.
func (c *Cache) Len() int {
	c.lock.Lock()
	defer c.lock.Unlock()
	return c.evictList.Len()
}
