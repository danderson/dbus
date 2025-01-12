package dbus

import (
	"fmt"
	"reflect"
	"sync"
)

var (
	signalsMu        sync.Mutex
	signalNameToType = map[signalKey]reflect.Type{}
	signalTypeToName = map[reflect.Type]signalKey{}
)

type signalKey struct {
	Interface, Signal string
}

// RegisterSignalType registers T as the struct type to use when
// decoding the body of the given signal name.
//
// RegisterSignalType panics if the signal already has a registered
// type.
func RegisterSignalType[T any](interfaceName, signalName string) {
	k := signalKey{interfaceName, signalName}
	t := reflect.TypeFor[T]()
	if t.Kind() != reflect.Struct {
		panic(fmt.Errorf("cannot use type %s (%s) as the payload type for signal %s.%s, signal payloads must be structs", t, t.Kind(), k.Interface, k.Signal))
	}
	if _, err := SignatureFor[T](); err != nil {
		panic(fmt.Errorf("cannot use %s as dbus type for signal %s.%s: %w", t, k.Interface, k.Signal, err))
	}
	signalsMu.Lock()
	defer signalsMu.Unlock()
	if prev := signalNameToType[k]; prev != nil {
		panic(fmt.Errorf("duplicate signal type registration for %s.%s, existing registration %s", k.Interface, k.Signal, prev))
	}
	if prev, ok := signalTypeToName[t]; ok {
		panic(fmt.Errorf("duplicate signal type registration for %s, already in use by %s.%s", t, prev.Interface, prev.Signal))
	}
	signalNameToType[k] = t
	signalTypeToName[t] = k
}
