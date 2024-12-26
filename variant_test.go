package dbus_test

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/danderson/dbus"
)

func TestMarshalVariant(t *testing.T) {
	tests := []struct {
		in   any
		want []byte // empty for error
	}{
		{},
		{byte(5), []byte{
			// Signature string "y"
			0x01, 0x79, 0x00,
			// val
			0x05,
		}},
		{true, []byte{
			// Signature string "b"
			0x01, 0x62, 0x00,
			// pad to bool
			0x00,
			// val
			0x00, 0x00, 0x00, 0x01,
		}},
		{struct{ A, B uint16 }{1, 2}, []byte{
			// Signature string "(qq)"
			0x04, 0x28, 0x71, 0x71, 0x29, 0x00,
			// pad to struct
			0x00, 0x00,
			// val
			0x00, 0x01,
			0x00, 0x02,
		}},
		{dbus.Variant{uint16(42)}, []byte{
			// Signature string "v"
			0x01, 0x76, 0x00,
			// Inner signature string "q"
			0x01, 0x71, 0x00,
			// val
			0x00, 0x2a,
		}},
	}

	for _, tc := range tests {
		v := dbus.Variant{tc.in}
		got, err := dbus.Marshal(v, binary.BigEndian)
		if err != nil {
			if len(tc.want) != 0 {
				t.Errorf("Marshal(dbus.Variant{%T}) got err: %v", tc.in, err)
			} else if testing.Verbose() {
				t.Logf("Marshal(dbus.Variant{%T}) = err: %v", tc.in, err)
			}
		} else if len(tc.want) == 0 {
			t.Errorf("Marshal(dbus.Variant{%T}) encoded successfully, want error", tc.in)
		} else if !bytes.Equal(got, tc.want) {
			t.Errorf("Marshal(dbus.Variant{%T}) wrong encoding:\n  got: % x\n want: % x", tc.in, got, tc.want)
		} else if testing.Verbose() {
			t.Logf("Marshal(dbus.Variant{%T:%#v}) = % x", tc.in, tc.in, got)
		}
	}
}
