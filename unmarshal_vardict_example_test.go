package dbus_test

import (
	"bytes"
	"fmt"

	"github.com/danderson/dbus"
)

// UnmarshalWithoutVardict is a translation of a (hypothetical) DBus
// message that uses the "vardict" idiom.
type UnmarshalWithoutVardict struct {
	Name string

	// This example DBus protocol documents two extension fields:
	// key 1 is a location string, key 2 is a temperature float64.
	Extensions map[uint8]dbus.Variant
}

// UnmarshalWithVardict is the same DBus message, with extension
// fields expressed as vardict fields.
type UnmarshalWithVardict struct {
	Name        string
	Location    string  `dbus:"key=1"`
	Temperature float64 `dbus:"key=2"`

	UnknownExtensions map[uint8]dbus.Variant `dbus:"vardict"`
}

func ExampleUnmarshal_vardict() {
	a := UnmarshalWithoutVardict{
		Name: "Weather station",
		Extensions: map[uint8]dbus.Variant{
			1: {string("Helsinki")},
			2: {float64(-4.2)},
		},
	}

	b := UnmarshalWithVardict{
		Name:        "Weather station",
		Location:    "Helsinki",
		Temperature: -4.2,
	}

	fmt.Println(bytes.Equal(mustMarshal(a), mustMarshal(b)))
	// Output: true
}
