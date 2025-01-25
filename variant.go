package dbus

import (
	"reflect"
)

// A Variant is a value of any valid DBus type.
//
// Variant corresponds to the DBus "variant" basic type, which is used
// in APIs where a value's type is only known at runtime.
type Variant struct {
	Value any
}

var variantType = reflect.TypeFor[Variant]()
