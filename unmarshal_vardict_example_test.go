package dbus_test

import (
	"bytes"
	"fmt"

	"github.com/danderson/dbus"
	"github.com/danderson/dbus/fragments"
)

func ExampleUnmarshal_vardict() {
	var noVardict struct {
		Name       string
		Extensions map[uint8]dbus.Variant
	}
	mustUnmarshal(sampleWireMessage, &noVardict)

	fmt.Println("Name:", noVardict.Name)
	fmt.Println("Location:", noVardict.Extensions[1].Value.(string))
	fmt.Println("Temperature:", noVardict.Extensions[2].Value.(float64))
	fmt.Println("Extensions:", len(noVardict.Extensions))
	fmt.Println("")

	var withVardict struct {
		Name        string
		Location    string  `dbus:"key=1"`
		Temperature float64 `dbus:"key=2"`

		UnknownExtensions map[uint8]dbus.Variant `dbus:"vardict"`
	}
	mustUnmarshal(sampleWireMessage, &withVardict)

	fmt.Println("Name:", withVardict.Name)
	fmt.Println("Location:", withVardict.Location)
	fmt.Println("Temperature:", withVardict.Temperature)
	fmt.Println("Extensions:", len(withVardict.UnknownExtensions))

	// Output:
	// Name: Weather station
	// Location: Helsinki
	// Temperature: -4.2
	// Extensions: 2
	//
	// Name: Weather station
	// Location: Helsinki
	// Temperature: -4.2
	// Extensions: 0
}

var sampleWireMessage = []byte{
	0x00, 0x00, 0x00, 0x0f, 0x57, 0x65, 0x61, 0x74,
	0x68, 0x65, 0x72, 0x20, 0x73, 0x74, 0x61, 0x74,
	0x69, 0x6f, 0x6e, 0x00, 0x00, 0x00, 0x00, 0x28,
	0x01, 0x01, 0x73, 0x00, 0x00, 0x00, 0x00, 0x08,
	0x48, 0x65, 0x6c, 0x73, 0x69, 0x6e, 0x6b, 0x69,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x02, 0x01, 0x64, 0x00, 0x00, 0x00, 0x00, 0x00,
	0xc0, 0x10, 0xcc, 0xcc, 0xcc, 0xcc, 0xcc, 0xcd,
}

func mustUnmarshal(bs []byte, v any) {
	err := dbus.Unmarshal(bytes.NewReader(bs), fragments.BigEndian, v)
	if err != nil {
		panic(err)
	}
}
