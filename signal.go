package dbus

import (
	"fmt"
	"reflect"
	"sync"
)

var (
	signalsMu   sync.Mutex
	signalTypes = map[string]reflect.Type{
		"org.freedesktop.DBus.NameOwnerChanged":                reflect.TypeFor[NameOwnerChanged](),
		"org.freedesktop.DBus.NameLost":                        reflect.TypeFor[NameLost](),
		"org.freedesktop.DBus.NameAcquired":                    reflect.TypeFor[NameAcquired](),
		"org.freedesktop.DBus.ActivatableServicesChanged":      reflect.TypeFor[ActivatableServicesChanged](),
		"org.freedesktop.DBus.Properties.PropertiesChanged":    reflect.TypeFor[PropertiesChanged](),
		"org.freedesktop.DBus.ObjectManager.InterfacesAdded":   reflect.TypeFor[InterfacesAdded](),
		"org.freedesktop.DBus.ObjectManager.InterfacesRemoved": reflect.TypeFor[InterfacesRemoved](),
	}
)

func RegisterSignalType[T any](interfaceName, signalName string) {
	name := interfaceName + "." + signalName
	t := reflect.TypeFor[T]()
	if _, err := SignatureFor[T](); err != nil {
		panic(fmt.Errorf("cannot use %s as dbus type for signal %s: %w", t, name, err))
	}
	signalsMu.Lock()
	defer signalsMu.Unlock()
	if prev := signalTypes[name]; t != nil {
		panic(fmt.Errorf("duplicate signal type registration for %s, existing registration %s", name, prev))
	}
	signalTypes[name] = t
}
