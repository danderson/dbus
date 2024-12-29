package fragments_test

import (
	"bytes"
	"fmt"
	"reflect"
	"testing"

	"github.com/danderson/dbus/fragments"
	"github.com/google/go-cmp/cmp"
)

type mustDecoder struct {
	t *testing.T
	*fragments.Decoder
}

func (d *mustDecoder) MustRead(n int, want []byte) {
	got, err := d.Read(n)
	if err != nil {
		d.t.Fatalf("Read(%d) got err: %v", n, err)
	}
	if !bytes.Equal(got, want) {
		d.t.Fatalf("Read(%d) wrong output:\n  got: % x\n want: % x", n, got, want)
	}
	if testing.Verbose() {
		d.t.Logf("Read(%d) = % x", n, got)
	}
}

func (d *mustDecoder) MustBytes(want []byte) {
	got, err := d.Bytes()
	if err != nil {
		d.t.Fatalf("Bytes() got err: %v", err)
	}
	if !bytes.Equal(got, want) {
		d.t.Fatalf("Bytes() wrong output:\n  got: % x\n want: % x", got, want)
	}
	if testing.Verbose() {
		d.t.Logf("Bytes() = % x", got)
	}
}

func (d *mustDecoder) MustString(want string) {
	got, err := d.String()
	if err != nil {
		d.t.Fatalf("String() got err: %v", err)
	}
	if got != want {
		d.t.Fatalf("String() got %q, want %q", got, want)
	}
	if testing.Verbose() {
		d.t.Logf("String() = %q", got)
	}
}

func (d *mustDecoder) MustUint8(want uint8) {
	got, err := d.Uint8()
	if err != nil {
		d.t.Fatalf("Uint8() got err: %v", err)
	}
	if got != want {
		d.t.Fatalf("Uint8() got %d, want %d", got, want)
	}
	if testing.Verbose() {
		d.t.Logf("Uint8() = %d", got)
	}
}

func (d *mustDecoder) MustUint16(want uint16) {
	got, err := d.Uint16()
	if err != nil {
		d.t.Fatalf("Uint16() got err: %v", err)
	}
	if got != want {
		d.t.Fatalf("Uint16() got %d, want %d", got, want)
	}
	if testing.Verbose() {
		d.t.Logf("Uint16() = %d", got)
	}
}

func (d *mustDecoder) MustUint32(want uint32) {
	got, err := d.Uint32()
	if err != nil {
		d.t.Fatalf("Uint32() got err: %v", err)
	}
	if got != want {
		d.t.Fatalf("Uint32() got %d, want %d", got, want)
	}
	if testing.Verbose() {
		d.t.Logf("Uint32() = %d", got)
	}
}

func (d *mustDecoder) MustUint64(want uint64) {
	got, err := d.Uint64()
	if err != nil {
		d.t.Fatalf("Uint64() got err: %v", err)
	}
	if got != want {
		d.t.Fatalf("Uint64() got %d, want %d", got, want)
	}
	if testing.Verbose() {
		d.t.Logf("Uint64() = %d", got)
	}
}

func (d *mustDecoder) MustValue(want any) {
	got := reflect.New(reflect.TypeOf(want).Elem()).Interface()
	if err := d.Value(got); err != nil {
		d.t.Fatalf("Value() got err: %v", err)
	}
	if diff := cmp.Diff(got, want); diff != "" {
		d.t.Fatalf("Value() got diff (-got+want):\n%s", diff)
	}
	if testing.Verbose() {
		d.t.Logf("Value() = %#v", reflect.ValueOf(got).Elem().Interface())
	}
}

func (d *mustDecoder) MustArray(containsStructs bool, wantLen int) {
	gotLen, err := d.Array(containsStructs)
	if err != nil {
		d.t.Fatalf("Array() got err: %v", err)
	}
	if gotLen != wantLen {
		d.t.Fatalf("Array() got size %d, want %d", gotLen, wantLen)
	}
	if testing.Verbose() {
		d.t.Logf("Array(%v) = %d elements", containsStructs, gotLen)
	}
}

func (d *mustDecoder) MustByteOrderFlag(want fragments.ByteOrder) {
	if err := d.ByteOrderFlag(); err != nil {
		d.t.Fatalf("ByteOrderFlag() got err: %v", err)
	}
	if got := d.Order; got != want {
		d.t.Fatalf("ByteOrderFlag() set byte order %s, want %s", got, want)
	}
}

func TestDecoder(t *testing.T) {
	tests := []struct {
		name   string
		in     []byte
		decode func(d *mustDecoder)
	}{
		{
			"raw bytes",
			[]byte{0x01, 0x02, 0x03},
			func(d *mustDecoder) {
				d.MustRead(3, []byte{1, 2, 3})
			},
		},

		{
			"byte array",
			[]byte{
				0x00, 0x00, 0x00, 0x03,
				0x01, 0x02, 0x03,
			},
			func(d *mustDecoder) {
				d.MustBytes([]byte{1, 2, 3})
			},
		},

		{
			"string",
			[]byte{
				0x00, 0x00, 0x00, 0x03,
				0x66, 0x6f, 0x6f,
				0x00,
			},
			func(d *mustDecoder) {
				d.MustString("foo")
			},
		},

		{
			"uints",
			[]byte{
				0x2a,
				0x00, // pad
				0x00, 0x42,
				0x00, 0x00, 0x00, 0x2a,
				0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x42,
			},
			func(d *mustDecoder) {
				d.MustUint8(42)
				d.MustUint16(66)
				d.MustUint32(42)
				d.MustUint64(66)
			},
		},

		{
			"uints padding",
			[]byte{
				0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x42,
				0x00,             // raw
				0x00, 0x00, 0x00, // pad
				0x00, 0x00, 0x00, 0x2a,
				0x00, // raw
				0x00, // pad
				0x00, 0x42,
				0x00, // raw
				0x2a,
			},
			func(d *mustDecoder) {
				d.MustUint64(66)
				d.MustRead(1, []byte{0})
				d.MustUint32(42)
				d.MustRead(1, []byte{0})
				d.MustUint16(66)
				d.MustRead(1, []byte{0})
				d.MustUint8(42)
			},
		},

		{
			"struct padding",
			[]byte{
				0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x42,
				0x00, 0x00, 0x00, 0x2a,
				0x00, 0x00, 0x00, 0x00, // pad
				0x00, 0x42,
				0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // pad
				0x2a,
			},
			func(d *mustDecoder) {
				d.Struct()
				d.MustUint64(66)
				d.Struct()
				d.MustUint32(42)
				d.Struct()
				d.MustUint16(66)
				d.Struct()
				d.MustUint8(42)
			},
		},

		{
			"array",
			[]byte{
				0x00, 0x00, 0x00, 0x02, // length
				0x00, 0x01,
				0x00, 0x02,
			},
			func(d *mustDecoder) {
				d.MustArray(false, 2)
				d.MustUint16(1)
				d.MustUint16(2)
			},
		},

		{
			"empty array",
			[]byte{
				0x00, 0x00, 0x00, 0x00, // length
			},
			func(d *mustDecoder) {
				d.MustArray(false, 0)
			},
		},

		{
			"struct array",
			[]byte{
				0x00, 0x00, 0x00, 0x02, // length
				0x00, 0x00, 0x00, 0x00, // pad
				0x00, 0x01,
				0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // pad
				0x00, 0x02,
			},
			func(d *mustDecoder) {
				d.MustArray(true, 2)
				d.Struct()
				d.MustUint16(1)
				d.Struct()
				d.MustUint16(2)
			},
		},

		{
			"empty struct array",
			[]byte{
				0x00, 0x00, 0x00, 0x00, // length
				0x00, 0x00, 0x00, 0x00, // pad
			},
			func(d *mustDecoder) {
				d.MustArray(true, 0)
			},
		},

		{
			"mapper",
			[]byte{
				0x73, 0x74, 0x72, 0x69, 0x6e, 0x67, // "string"
				0x75, 0x69, 0x6e, 0x74, 0x31, 0x36, // "uint16"
			},
			func(d *mustDecoder) {
				d.Mapper = func(t reflect.Type) fragments.DecoderFunc {
					return func(d *fragments.Decoder, v reflect.Value) error {
						want := v.Type().String()
						gotBs, err := d.Read(len(want))
						if err != nil {
							return err
						}
						if got := string(gotBs); got != want {
							return fmt.Errorf("custom mapper got %q, want %q", got, want)
						}
						v.Set(reflect.Zero(t))
						return nil
					}
				}
				var s string
				d.MustValue(&s)
				var u16 uint16
				d.MustValue(&u16)
			},
		},

		{
			"byte order flag",
			[]byte{'B', 'l', '?'},
			func(d *mustDecoder) {
				d.MustByteOrderFlag(fragments.BigEndian)
				d.MustByteOrderFlag(fragments.LittleEndian)
				if err := d.ByteOrderFlag(); err == nil {
					t.Fatalf("ByteOrderFlag did not error on invalid byte order")
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			d := mustDecoder{
				t: t,
				Decoder: &fragments.Decoder{
					Order: fragments.BigEndian,
					In:    tc.in,
				},
			}
			tc.decode(&d)
			if remain := d.Remaining(); remain > 0 {
				t.Fatalf("decoder failed to consume %d trailing bytes", remain)
			}
		})
	}
}
