package dbus_test

import (
	"bytes"
	"fmt"
	"io"

	"github.com/danderson/dbus"
	"github.com/danderson/dbus/fragments"
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

func sampleWireMessage() io.Reader {
	v := UnmarshalWithoutVardict{
		Name: "Weather station",
		Extensions: map[uint8]dbus.Variant{
			1: {string("Helsinki")},
			2: {float64(-4.2)},
		},
	}
	bs, err := dbus.Marshal(v, fragments.BigEndian)
	if err != nil {
		panic(err)
	}
	return bytes.NewReader(bs)
}

func ExampleUnmarshal_vardict() {
	var s UnmarshalWithVardict

	err := dbus.Unmarshal(sampleWireMessage(), fragments.BigEndian, &s)
	if err != nil {
		panic(err)
	}

	fmt.Println("Name:", s.Name)
	fmt.Println("Location:", s.Location)
	fmt.Println("Temperature:", s.Temperature)
	fmt.Println("Unknown extensions:", len(s.UnknownExtensions))
	// Output:
	// Name: Weather station
	// Location: Helsinki
	// Temperature: -4.2
	// Unknown extensions: 0
}
