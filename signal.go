package dbus

import (
	"fmt"
	"reflect"
	"sync"
)

var (
	signalsMu sync.Mutex

	signalNameToType = map[interfaceMember]reflect.Type{}
	signalTypeToName = map[reflect.Type]interfaceMember{}

	propNameToType = map[interfaceMember]reflect.Type{}
	propTypeToName = map[reflect.Type]interfaceMember{}
)

// RegisterPropertyChangeType registers T as the type to use when
// reporting a property change for the given property.
//
// Panics if the property already has a registered type.
func RegisterPropertyChangeType[T any](interfaceName, propertyName string) {
	k := interfaceMember{interfaceName, propertyName}
	t := reflect.TypeFor[T]()
	if _, err := SignatureFor[T](); err != nil {
		panic(fmt.Errorf("cannot use %s as dbus type for property change %s: %w", t, k, err))
	}

	signalsMu.Lock()
	defer signalsMu.Unlock()
	if prev := propNameToType[k]; prev != nil {
		panic(fmt.Errorf("duplicate property change type registration for %s, existing registration %s", k, prev))
	}
	if prev, ok := propTypeToName[t]; ok {
		panic(fmt.Errorf("duplicate property change type registration for %s, already in use by %s", t, prev))
	}
	propNameToType[k] = t
	propTypeToName[t] = k
}

// RegisterSignalType registers T as the struct type to use when
// decoding the body of the given signal name.
//
// Panics if the signal already has a registered type.
func RegisterSignalType[T any](interfaceName, signalName string) {
	k := interfaceMember{interfaceName, signalName}
	t := reflect.TypeFor[T]()
	if t.Kind() != reflect.Struct {
		panic(fmt.Errorf("cannot use type %s (%s) as the payload type for signal %s.%s, signal payloads must be structs", t, t.Kind(), k.Interface, k.Member))
	}
	if _, err := SignatureFor[T](); err != nil {
		panic(fmt.Errorf("cannot use %s as dbus type for signal %s.%s: %w", t, k.Interface, k.Member, err))
	}

	signalsMu.Lock()
	defer signalsMu.Unlock()
	if prev := signalNameToType[k]; prev != nil {
		panic(fmt.Errorf("duplicate signal type registration for %s, existing registration %s", k, prev))
	}
	if prev, ok := signalTypeToName[t]; ok {
		panic(fmt.Errorf("duplicate signal type registration for %s, already in use by %s", t, prev))
	}
	signalNameToType[k] = t
	signalTypeToName[t] = k
}

func signalNameFor(t reflect.Type) (interfaceMember, bool) {
	signalsMu.Lock()
	defer signalsMu.Unlock()
	ret, ok := signalTypeToName[t]
	return ret, ok
}

func signalTypeFor(interfaceName, signalName string) reflect.Type {
	signalsMu.Lock()
	defer signalsMu.Unlock()
	return signalNameToType[interfaceMember{interfaceName, signalName}]
}

func propNameFor(t reflect.Type) (interfaceMember, bool) {
	signalsMu.Lock()
	defer signalsMu.Unlock()
	ret, ok := propTypeToName[t]
	return ret, ok
}

func propTypeFor(interfaceName, propName string) reflect.Type {
	signalsMu.Lock()
	defer signalsMu.Unlock()
	return propNameToType[interfaceMember{interfaceName, propName}]
}
