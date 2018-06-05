// Package cache 内存缓存, 包括 过期，定时清理，lru，自动加载，互斥更新
package cache

import (
	"container/list"
	"sync"
	"time"
)

type UpFunc func(key interface{}) (interface{}, int)

// call is an in-flight or completed Do call, refer to groupcache-singleflight.go
type call struct {
	wg  sync.WaitGroup
	val interface{}
	ret int
}

// Group represents a class of work and forms a namespace in which
// units of work can be executed with duplicate suppression.
// refer to groupcache-singleflight.go
type Group struct {
	mu sync.Mutex       // protects m
	m  map[interface{}]*call // lazily initialized
}

// Value cache value with put heartbeat
type Value struct {
	key       interface{}
	value     interface{}
	heartbeat time.Time
}

// Cache with map data
type Cache struct {
	lru          *list.List
	data         map[interface{}]*list.Element
	lock         sync.RWMutex
	expire       time.Duration
	interval     time.Duration
	maxKeyCount  int
	upfunc       UpFunc
	initUpStatus map[interface{}]bool
	g 			 Group
}

// New new a cache obj
func New() *Cache {
	c := &Cache{
		data:         make(map[interface{}]*list.Element),
		lru:          list.New(),
		initUpStatus: make(map[interface{}]bool),
	}
	return c
}

// SetExpire set cache expire duration time
func (c *Cache) SetExpire(e time.Duration) {
	c.expire = e
}

// SetMaxKeyCount set cache max key count, put will clear expired keys if key num > maxKeyCount
func (c *Cache) SetMaxKeyCount(cnt int) {
	c.maxKeyCount = cnt
}

// SetUpFunc set cache upfunc
func (c *Cache) SetUpFunc(f UpFunc) {
	c.upfunc = f
}

// SetClearInterval start a gorountine to clear expired data every duration time, must larger than 1 second
func (c *Cache) SetClearInterval(d time.Duration) {
	if d < time.Second {
		return
	}
	if c.interval == 0 {
		c.interval = d
		go func() {
			for {
				time.Sleep(c.interval)
				c.clearExpired()
			}
		}()
	} else {
		c.interval = d
	}
}

func (c *Cache) clearExpired() {
	if c.expire == 0 {
		return
	}

	now := time.Now()

	c.lock.Lock()
	for k, e := range c.data {
		v := e.Value.(*Value)
		if v.heartbeat.Add(c.expire).Before(now) {
			delete(c.data, k)
			c.lru.Remove(e)
		}
	}
	c.lock.Unlock()
}

// Get return cache value if exists, if expired and upfunc != nil, do getDataSingle or upData
func (c *Cache) Get(k interface{}) (interface{}, int) {
	c.lock.RLock()
	e, ok := c.data[k]
	c.lock.RUnlock()

	if !ok {
		if c.upfunc != nil {
			return c.getData(k)
		}
		return nil, -1
	}

	v := e.Value.(*Value)
	if c.expire != 0 && v.heartbeat.Add(c.expire).Before(time.Now()) {
		if c.upfunc != nil {
			c.lock.Lock()
			if v.heartbeat.Add(c.expire).Before(time.Now()) {
				v.heartbeat = v.heartbeat.Add(time.Second * 3)
				c.upValue(k)
			}
			c.lock.Unlock()
			return v.value, 0
		}
		return v.value, -1
	}

	if c.maxKeyCount > 0 {
		c.lock.Lock()
		c.lru.MoveToFront(e)
		c.lock.Unlock()
	}
	return v.value, 0
}

// Put update cache value
func (c *Cache) Put(k, v interface{}) {

	c.lock.Lock()
	defer c.lock.Unlock()
	e, ok := c.data[k]
	if ok {
		vv := e.Value.(*Value)
		vv.value = v
		vv.heartbeat = time.Now()
		c.lru.MoveToFront(e)
	} else {
		vv := &Value{
			key:       k,
			value:     v,
			heartbeat: time.Now(),
		}
		e := c.lru.PushFront(vv)
		c.data[k] = e
	}

	if c.maxKeyCount > 0 && len(c.data) >= c.maxKeyCount {
		e := c.lru.Back()
		if e != nil {
			c.lru.Remove(e)
			v := e.Value.(*Value)
			delete(c.data, v.key)
		}
	}
}

// getData get data, if mul goroutine call, only one do get, refer to groupcache-singleflight.goß
func (c *Cache) getData(key interface{}) (interface{}, int) {
	c.g.mu.Lock()
	if c.g.m == nil {
		c.g.m = make(map[interface{}]*call)
	}
	if key_call, ok := c.g.m[key]; ok {
		c.g.mu.Unlock()
		key_call.wg.Wait()
		return key_call.val, key_call.ret
	}
	key_call := new(call)
	key_call.wg.Add(1)
	c.g.m[key] = key_call
	c.g.mu.Unlock()

	key_call.val, key_call.ret = c.upfunc(key)
	if key_call.ret == 0 {
		c.Put(key, key_call.val)
	}
	key_call.wg.Done()

	c.g.mu.Lock()
	delete(c.g.m, key)
	c.g.mu.Unlock()
	return key_call.val, key_call.ret
}

// Update using register func
func (c *Cache) upValue(key interface{}) {
	go func() {
		v, e := c.upfunc(key)
		if e == 0 {
			c.Put(key, v)
		}
	}()
}

// Len return data map len
func (c *Cache) Len() int {
	return len(c.data)
}

// Del delete data by key
func (c *Cache) Del(k interface{}) {
	c.lock.Lock()
	e, ok := c.data[k]
	if ok {
		c.lru.Remove(e)
		delete(c.data, k)
	}
	c.lock.Unlock()
}

// Clear empty map
func (c *Cache) Clear() {
	c.data = make(map[interface{}]*list.Element)
	c.lru.Init()
}
