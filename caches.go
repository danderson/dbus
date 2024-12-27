package dbus

import (
	"fmt"
	"reflect"
	"sync"
)

type cache[V any] struct {
	OnRecursive func(reflect.Type) V
	m           sync.Map
}

func (c *cache[V]) Get(t reflect.Type) (val V, found bool) {
	ent, loaded := c.m.LoadOrStore(t, nil)
	if !loaded {
		var zero V
		return zero, false
	}
	if ent == nil {
		if c.OnRecursive == nil {
			panic(unrepresentable(t, "recursive type"))
		}
		ret := c.OnRecursive(t)
		c.m.CompareAndSwap(t, nil, ret)
		return ret, true
	}
	if val, ok := ent.(V); ok {
		return val, true
	}
	panic(fmt.Sprintf("mystery value %v (%T) in cache", ent, ent))
}

func (c *cache[V]) Put(t reflect.Type, val V) {
	c.m.CompareAndSwap(t, nil, val)
}
