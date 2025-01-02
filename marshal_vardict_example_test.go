package dbus_test

import (
	"bytes"
	"fmt"

	"github.com/danderson/dbus"
	"github.com/danderson/dbus/fragments"
)

// MarshalNoVardict is a translation of a (hypothetical) DBus message
// that uses the "vardict" idiom.
type MarshalNoVardict struct {
	Name string

	// This example DBus protocol documents two extension fields:
	// key 1 is a location string, key 2 is a temperature float64.
	Extensions map[uint8]dbus.Variant
}

// MarshalWithVardict is the same DBus message, with extension fields
// expressed as vardict fields.
type MarshalWithVardict struct {
	Name        string
	Location    string  `dbus:"key=1"`
	Temperature float64 `dbus:"key=2"`

	UnknownExtensions map[uint8]dbus.Variant `dbus:"vardict"`
}

func marshalsTheSame(a, b any) bool {
	ab, err := dbus.Marshal(a, fragments.BigEndian)
	if err != nil {
		panic(err)
	}
	bb, err := dbus.Marshal(b, fragments.BigEndian)
	if err != nil {
		panic(err)
	}
	return bytes.Equal(ab, bb)
}

func ExampleMarshal_vardict() {
	a := MarshalNoVardict{
		Name: "Weather station",
		Extensions: map[uint8]dbus.Variant{
			1: {string("Helsinki")},
			2: {float64(-4.2)},
		},
	}

	b := MarshalWithVardict{
		Name:        "Weather station",
		Location:    "Helsinki",
		Temperature: -4.2,
	}

	fmt.Println(marshalsTheSame(a, b))
	// Output: true
}
