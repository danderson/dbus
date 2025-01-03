package dbus

import (
	"reflect"

	"github.com/danderson/dbus/fragments"
)

func init() {
	// These need to be an init func to break the initialization cycle
	// between the caches and the calls to the cache within their getters.
	encoders.Init(uncachedTypeEncoder, func(t reflect.Type) fragments.EncoderFunc {
		return newErrEncoder(t, "recursive type")
	})
	decoders.Init(uncachedTypeDecoder, func(t reflect.Type) fragments.DecoderFunc {
		return newErrDecoder(t, "recursive type")
	})
	signatures.Init(
		func(t reflect.Type) sigCacheEntry {
			return sigCacheEntry{uncachedSignatureOf(t), nil}
		},
		func(t reflect.Type) sigCacheEntry {
			sigErr(t, "recursive type")
			panic("unreachable")
		})

	// These use the caches above, so must come after.
	RegisterSignalType[NameOwnerChanged]("org.freedesktop.DBus", "NameOwnerChanged")
	RegisterSignalType[NameLost]("org.freedesktop.DBus", "NameLost")
	RegisterSignalType[NameAcquired]("org.freedesktop.DBus", "NameAcquired")
	RegisterSignalType[ActivatableServicesChanged]("org.freedesktop.DBus", "ActivatableServicesChanged")
	RegisterSignalType[PropertiesChanged]("org.freedesktop.DBus.Properties", "PropertiesChanged")
	RegisterSignalType[InterfacesAdded]("org.freedesktop.DBus", "InterfacesAdded")
	RegisterSignalType[InterfacesRemoved]("org.freedesktop.DBus", "InterfacesRemoved")
}
