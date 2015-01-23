// This package provides a simple LRU cache. It is based on the
// LRU implementation in groupcache:
// https://github.com/golang/groupcache/tree/master/lru
package lru

import (
    "container/list"
    "errors"
    "sync"
    "time"
    "log"
    "github.com/boltdb/bolt"
)

// Cache is a thread-safe fixed size LRU cache.
type Cache struct {
    size      int
    evictList *list.List
    lock      sync.Mutex
    onRemove  func(interface{})
    db        *bolt.DB
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
    db, err := bolt.Open("my.db", 0600, nil)
    if err != nil {
        log.Fatal(err)
    }
    c := &Cache{
        size:      size,
        evictList: list.New(),
        onRemove:  onRemove,
        db:        db,
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
func (c *Cache) Add(bucket, key string, value interface{}) {
    c.lock.Lock()
    defer c.lock.Unlock()
    // Check for existing item
    if err := c.db.View(func(tx *bolt.Tx) error {
        v := tx.Bucket([]byte(bucket)).Get([]byte(key))
        if len(v) > 0 {
            return errors.New("Duplicate Key.")
        }
        return nil
    }); err != nil {
        log.Println(err)
    }

    // Add new item
    ent := &entry{key, value}
    c.evictList.PushFront(ent)
    if err := c.db.Update(func(tx *bolt.Tx) error {
            b, err := tx.CreateBucketIfNotExists([]byte(bucket))
            if err != nil {
                log.Println(err)
                return err
            }
            if b.Put([]byte(key), value.([]byte)); err != nil {
                return err
            }
            return nil
        }); err != nil {
            log.Println(err)
            return
        }

    // Verify size not exceeded
    if c.evictList.Len() > c.size {
        c.removeOldest()
    }
}

func (c *Cache) Update(bucket, key string, f func(val interface{})) bool {
    c.lock.Lock()
    defer c.lock.Unlock()
    var v interface{}
    if err := c.db.View(func(tx *bolt.Tx) error {
            value := tx.Bucket([]byte(bucket)).Get([]byte(key))
            if len(value) == 0 {
                return errors.New("No item found")
            }
            v = interface{}(value)
            return nil
        }); err != nil {
            return false
        }
    f(v)

    if err := c.db.Update(func(tx *bolt.Tx) error {
            if err := tx.Bucket([]byte(bucket)).Put([]byte(key), v.([]byte)); err != nil {
                return err
            }
            return nil
        }); err != nil {
            log.Println(err)
            return false
        }

    return true
}

// Get looks up a key's value from the cache.
func (c *Cache) Get(bucket, key string) (value interface{}, ok bool) {
    if err := c.db.View(func(tx *bolt.Tx) error {
            v := tx.Bucket([]byte(bucket)).Get([]byte(key))
            if len(v) == 0 {
                return errors.New("No item found")
            }
            value = interface{}(v)
            ok = true
            return nil
        }); err != nil {
            return
        }
    return
}

// Remove removes the provided key from the cache.
func (c *Cache) Remove(key interface{}) {
    c.lock.Lock()
    defer c.lock.Unlock()

    if ent, ok := c.Get("url", key.(string)); ok {
        c.removeElement(ent.(*list.Element))
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
        var v interface{}
        if err := c.db.View(func(tx *bolt.Tx) error {
                value := tx.Bucket([]byte("url")).Get(ent.Value.([]byte))
                if len(value) == 0 {
                    return errors.New("No item found")
                }
                v = interface{}(value)
                return nil
            }); err != nil {
                return
            }
        c.onRemove(v)
    }
}

// removeElement is used to remove a given list element from the cache
func (c *Cache) removeElement(e *list.Element) {
    c.evictList.Remove(e)
    kv := e.Value.(*entry)
    if err := c.db.Update(func(tx *bolt.Tx) error {
            return tx.Bucket([]byte("url")).Delete(kv.key.([]byte))
        }); err != nil {
        log.Println(err)
            return
        }
}

// Len returns the number of items in the cache.
func (c *Cache) Len() int {
    c.lock.Lock()
    defer c.lock.Unlock()
    return c.evictList.Len()
}
