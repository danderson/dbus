package dbus

import (
	"reflect"

	"github.com/creachadair/mds/mapset"
)

var (
	// strToType maps the DBus type signature identifier of a type to its
	// reflect.Type.
	strToType = map[byte]reflect.Type{
		'b': reflect.TypeFor[bool](),
		'y': reflect.TypeFor[uint8](),
		'n': reflect.TypeFor[int16](),
		'q': reflect.TypeFor[uint16](),
		'i': reflect.TypeFor[int32](),
		'u': reflect.TypeFor[uint32](),
		'x': reflect.TypeFor[int64](),
		't': reflect.TypeFor[uint64](),
		'd': reflect.TypeFor[float64](),
		's': reflect.TypeFor[string](),
		'v': reflect.TypeFor[Variant](),
		'g': reflect.TypeFor[Signature](),
		'o': reflect.TypeFor[ObjectPath](),
		'h': reflect.TypeFor[FileDescriptor](),
	}

	// typeToStr is the inverse of strToType.
	typeToStr = map[reflect.Type]byte{
		reflect.TypeFor[bool]():           'b',
		reflect.TypeFor[uint8]():          'y',
		reflect.TypeFor[int16]():          'n',
		reflect.TypeFor[uint16]():         'q',
		reflect.TypeFor[int32]():          'i',
		reflect.TypeFor[uint32]():         'u',
		reflect.TypeFor[int64]():          'x',
		reflect.TypeFor[uint64]():         't',
		reflect.TypeFor[float64]():        'd',
		reflect.TypeFor[string]():         's',
		reflect.TypeFor[Variant]():        'v',
		reflect.TypeFor[Signature]():      'g',
		reflect.TypeFor[ObjectPath]():     'o',
		reflect.TypeFor[FileDescriptor](): 'h',
	}

	// kindToType maps the reflect.Kinds of the basic types representable
	// by DBus to the corresponding reflect.Type.
	kindToType = map[reflect.Kind]reflect.Type{
		reflect.Bool:    reflect.TypeFor[bool](),
		reflect.Uint8:   reflect.TypeFor[uint8](),
		reflect.Int16:   reflect.TypeFor[int16](),
		reflect.Uint16:  reflect.TypeFor[uint16](),
		reflect.Int32:   reflect.TypeFor[int32](),
		reflect.Uint32:  reflect.TypeFor[uint32](),
		reflect.Int64:   reflect.TypeFor[int64](),
		reflect.Uint64:  reflect.TypeFor[uint64](),
		reflect.Float64: reflect.TypeFor[float64](),
		reflect.String:  reflect.TypeFor[string](),
	}

	// mapKeyKinds is the set of reflect.Kinds that can be in a DBus map
	// key.
	mapKeyKinds = mapset.New(
		reflect.Bool,
		reflect.Uint8,
		reflect.Int16,
		reflect.Uint16,
		reflect.Int32,
		reflect.Uint32,
		reflect.Int64,
		reflect.Uint64,
		reflect.Float64,
		reflect.String,
	)
)
