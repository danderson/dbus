package dbus

import (
	"os"
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
		'v': reflect.TypeFor[any](),
		'g': reflect.TypeFor[Signature](),
		'o': reflect.TypeFor[ObjectPath](),
		'h': reflect.TypeFor[*os.File](),
	}

	// typeToStr maps basic DBus types that aren't basic Go types to
	// their DBus type signature identifier.
	typeToStr = map[reflect.Type]byte{
		reflect.TypeFor[any]():        'v',
		reflect.TypeFor[Signature]():  'g',
		reflect.TypeFor[ObjectPath](): 'o',
		reflect.TypeFor[*os.File]():   'h',
	}

	// kindToStr maps reflect.Kinds to their corresponding DBus type
	// signature identifier, if any.
	kindToStr = map[reflect.Kind]byte{
		reflect.Bool:    'b',
		reflect.Uint8:   'y',
		reflect.Int16:   'n',
		reflect.Uint16:  'q',
		reflect.Int32:   'i',
		reflect.Uint32:  'u',
		reflect.Int64:   'x',
		reflect.Uint64:  't',
		reflect.Float64: 'd',
		reflect.String:  's',
	}

	// kindToType reflect.Kinds of DBus basic types to their
	// corresponding reflect.Type.
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

	// mapKeyKinds is the set of reflect.Kinds that can be DBus map
	// keys.
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
