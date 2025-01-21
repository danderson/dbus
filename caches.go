package dbus

import (
	"errors"
	"fmt"
	"sync"
)

type cache[K, V any] struct {
	m sync.Map
}

var errNotFound = errors.New("key not found in cache")

// Get returns the value associated with t, constructing it if
// necessary.
func (c *cache[K, V]) Get(k K) (ret V, err error) {
	ent, loaded := c.m.Load(k)
	if !loaded {
		var zero V
		return zero, errNotFound
	}
	if e, ok := ent.(error); ok {
		var zero V
		return zero, e
	}
	if v, ok := ent.(V); ok {
		return v, nil
	}
	panic(fmt.Errorf("unknown value %v (%T) stored in cache", ent, ent))
}

func (c *cache[K, V]) Set(k K, v V) {
	c.m.Store(k, v)
}

func (c *cache[K, V]) SetErr(k K, err error) {
	c.m.Store(k, err)
}
