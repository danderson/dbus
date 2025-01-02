package dbus_test

import (
	"bytes"
	"context"
	"fmt"

	"github.com/danderson/dbus"
	"github.com/danderson/dbus/fragments"
)

func mustMarshal(a any) []byte {
	bs, err := dbus.Marshal(context.Background(), a, fragments.BigEndian)
	if err != nil {
		panic(err)
	}
	return bs
}

func ExampleMarshal_vardict() {
	type NoVardict struct {
		Name       string
		Extensions map[uint8]dbus.Variant
	}
	noVardict := mustMarshal(NoVardict{
		Name: "Weather",
		Extensions: map[uint8]dbus.Variant{
			1: {string("Helsinki")},
			2: {float64(-4.2)},
		},
	})

	type WithVardict struct {
		Name        string
		Location    string  `dbus:"key=1"`
		Temperature float64 `dbus:"key=2"`

		UnknownExtensions map[uint8]dbus.Variant `dbus:"vardict"`
	}
	withVardict := mustMarshal(WithVardict{
		Name:        "Weather",
		Location:    "Helsinki",
		Temperature: -4.2,
	})

	fmt.Println(bytes.Equal(noVardict, withVardict))
	// Output: true
}
