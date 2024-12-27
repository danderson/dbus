package dbus

import (
	"reflect"
	"sync"
)

// cache is a pull-through cache of values derived from reflected
// types.
type cache[V any] struct {
	get          func(reflect.Type) V
	getRecursive func(reflect.Type) V
	m            sync.Map
}

// Init sets the constructors to use on cache miss. The cache calls
// get(t) to construct a value for type t, and getRecursive(t) when a
// cache miss occurs for t while executing get(t).
func (c *cache[V]) Init(get, getRecursive func(reflect.Type) V) {
	c.get = get
	c.getRecursive = getRecursive
}

// Get returns the value associated with t, constructing it if
// necessary.
func (c *cache[V]) Get(t reflect.Type) (ret V) {
	ent, loaded := c.m.LoadOrStore(t, nil)
	if loaded && ent != nil {
		return ent.(V)

	}
	defer func() {
		if e := recover(); e != nil {
			if v, ok := e.(errUnwind[V]); ok {
				c.m.CompareAndSwap(t, nil, v.v)
			}
			panic(e)
		}
	}()

	if loaded {
		ret = c.getRecursive(t)
	} else {
		ret = c.get(t)
	}
	c.m.CompareAndSwap(t, nil, ret)
	return ret
}

// Unwind unwinds the call stack to the nearest [cache.GetRecover],
// caching v as the result of all intermediate [cache.Get] calls.
func (c *cache[V]) Unwind(v V) {
	panic(errUnwind[V]{v})
}

// GetRecover is like Get, but stops the propagation of [cache.Unwind]
// further up the stack. It returns either the result of Get, or if an
// unwind occurs, the value passed to [cache.Unwind].
func (c *cache[V]) GetRecover(t reflect.Type) (ret V) {
	defer func() {
		if e := recover(); e != nil {
			if v, ok := e.(errUnwind[V]); ok {
				// Get() already poked v into the cache, we just need
				// to eat the panic.
				ret = v.v
			} else {
				panic(e)
			}
		}
	}()
	return c.Get(t)
}

type errUnwind[V any] struct {
	v V
}
