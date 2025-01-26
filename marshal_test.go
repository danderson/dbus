package dbus

import (
	"bytes"
	"context"
	"reflect"
	"testing"

	"github.com/danderson/dbus/fragments"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

func TestMarshalUnmarshal(t *testing.T) {
	type testCase struct {
		name       string
		raw        []byte
		wantDecode any
		toEncode   any
		sigStr     string
		wantErr    bool
	}
	ok := func(name string, sig string, want any, raw ...byte) testCase {
		return testCase{name, raw, want, want, sig, false}
	}
	asymmetric := func(name string, sig string, decoded any, toEncode any, raw ...byte) testCase {
		return testCase{name, raw, decoded, toEncode, sig, false}
	}

	tests := []testCase{
		ok("true", "b", true,
			0, 0, 0, 1),
		ok("false", "b", false,
			0, 0, 0, 0),

		ok("byte", "y", byte(42),
			42),
		ok("i16", "n", int16(0x1234),
			0x12, 0x34),
		ok("u16", "q", uint16(0x1234),
			0x12, 0x34),
		ok("i32", "i", int32(0x12345678),
			0x12, 0x34, 0x56, 0x78),
		ok("u32", "u", uint32(0x12345678),
			0x12, 0x34, 0x56, 0x78),
		ok("i64", "x", int64(0x1abbccdd12345678),
			0x1a, 0xbb, 0xcc, 0xdd,
			0x12, 0x34, 0x56, 0x78),
		ok("u64", "t", uint64(0x1abbccdd12345678),
			0x1a, 0xbb, 0xcc, 0xdd,
			0x12, 0x34, 0x56, 0x78),

		ok("f64", "d", float64(3402823700),
			0x41, 0xE9, 0x5A, 0x5F,
			0x02, 0x80, 0x00, 0x00),

		ok("string", "s", "foobar",
			// Length
			0, 0, 0, 6,
			// Value
			'f', 'o', 'o', 'b', 'a', 'r',
			// Terminator
			0),

		ok("bytes", "ay", []byte("foobar"),
			// Length
			0, 0, 0, 6,
			// Value
			'f', 'o', 'o', 'b', 'a', 'r'),

		ok("[]string", "as", []string{"fo", "obar"},
			// array length
			0, 0, 0, 17,
			// "fo"
			0, 0, 0, 2, 'f', 'o', 0,
			// pad
			0,
			// "obar"
			0, 0, 0, 4, 'o', 'b', 'a', 'r', 0),
		ok("[][]string", "aas", [][]string{{"fo", "obar"}, {"qux"}},
			// outer array length
			0, 0, 0, 36,

			// array length
			0, 0, 0, 17,
			// "fo"
			0, 0, 0, 2, 'f', 'o', 0,
			// pad
			0,
			// "obar"
			0, 0, 0, 4, 'o', 'b', 'a', 'r', 0,

			// pad
			0, 0, 0,

			// array length
			0, 0, 0, 8,
			0, 0, 0, 3, 'q', 'u', 'x', 0,
		),

		ok("sig(byte)", "g",
			mustSignatureFor[byte](),
			1, 'y', 0),
		ok("sig(ObjectPath)", "g",
			mustSignatureFor[[]ObjectPath](),
			2, 'a', 'o', 0),

		ok("struct simple", "(nb)",
			Simple{42, true},
			// .A
			0, 42,
			// pad
			0, 0,
			// .B
			0, 0, 0, 1),

		ok("struct any", "(qv)",
			WithAny{42, uint32(66)},
			// .A
			0, 42,
			// .B
			// signature: uint32
			1, 'u', 0,
			// pad
			0, 0, 0,
			// value
			0, 0, 0, 66,
		),

		ok("struct nested", "(y(nb))",
			Nested{66, Simple{42, true}},
			// .A
			66,
			// pad to struct
			0, 0, 0,
			0, 0, 0, 0,
			// .B.A
			0, 42,
			// pad
			0, 0,
			// .B.B
			0, 0, 0, 1),

		ok("struct embedded", "(nby)",
			Embedded{Simple{42, true}, 66},
			// .Simple.A
			0, 42,
			// pad
			0, 0,
			// .Simple.B
			0, 0, 0, 1,
			// .C
			66),

		ok("struct embedded ptr", "(nby)",
			Embedded_P{&Simple{42, true}, 66},
			// .Simple.A
			0, 42,
			// pad
			0, 0,
			// .Simple.B
			0, 0, 0, 1,
			// .C
			66),
		asymmetric("struct embedded nilptr", "(nby)",
			Embedded_P{&Simple{}, 66},
			Embedded_P{nil, 66},
			// .Simple.A
			0, 0,
			// pad
			0, 0,
			// .Simple.B
			0, 0, 0, 0,
			// .C
			66),

		asymmetric("struct embedded PVP", "(nbyy)",
			Embedded_PVP{
				&Embedded_PV{
					Embedded_P{
						&Simple{},
						0,
					},
				},
				66,
			},
			Embedded_PVP{D: 66},
			// .Embedded_PV.Embedded_P.Simple.A
			0,
			// pad
			0, 0, 0,
			// .Embedded_PV.Embedded_P.Simple.B
			0, 0, 0, 0,
			// .Embedded_PV.Embedded_P.C
			0,
			// .D
			66),

		ok("struct embedded shadow", "(ny)",
			EmbeddedShadow{Simple{42, false}, 66},
			// .Simple.A
			0, 42,
			// .B
			66),

		ok("struct nested selfmarshaler ptr", "(yq)",
			&NestedSelfMarshalerPtr{42, SelfMarshalerPtr{41}},
			42,
			0, 0,
			0, 42,
		),
		ok("struct nested selfmarshaler ptr ptr", "(yq)",
			&NestedSelfMarshalerPtrPtr{42, &SelfMarshalerPtr{41}},
			42,
			0, 0,
			0, 42,
		),

		ok("struct selfmarshaler ptr", "q",
			&SelfMarshalerPtr{41},
			0, 42),

		ok("map", "a{qy}", map[uint16]uint8{1: 2, 3: 4},
			// dict length
			0, 0, 0, 11,
			// pad
			0, 0, 0, 0,
			// key=1
			0, 1,
			// val=2
			2,
			// pad
			0, 0, 0, 0, 0,
			// key=3
			0, 3,
			// val=4
			4),
		ok("map ptr vals", "a{qy}",
			map[uint16]*uint8{
				1: ptr[uint8](2),
				3: ptr[uint8](4),
			},
			// dict length
			0, 0, 0, 11,
			// pad
			0, 0, 0, 0,
			// key=1
			0, 1,
			// val=2
			2,
			// pad
			0, 0, 0, 0, 0,
			// key=3
			0, 3,
			// val=4
			4),

		ok("vardict", "(a{sv})",
			VarDict{
				A: 1,
				B: 2,
				C: "foo",
			},
			// dict length
			0, 0, 0, 54,
			// pad
			0, 0, 0, 0,

			// key="C"
			0, 0, 0, 1, 'C', 0,
			// signature (string)
			1, 's', 0,
			// pad
			0, 0, 0,
			// val="foo"
			0, 0, 0, 3, 'f', 'o', 'o', 0,

			// pad to struct
			0, 0, 0, 0,

			// key="bar"
			0, 0, 0, 3, 'b', 'a', 'r', 0,
			// signature (uint32)
			1, 'u', 0,
			// pad
			0,
			// val=2
			0, 0, 0, 2,

			// key="foo"
			0, 0, 0, 3, 'f', 'o', 'o', 0,
			// signature (uint16)
			1, 'q', 0,
			// pad
			0,
			// val=1
			0, 1),

		ok("vardict unknown", "(a{sv})",
			VarDict{
				D: 1,
				Other: map[string]any{
					"a": uint8(2),
					"z": uint16(3),
				},
			},
			// dict length
			0, 0, 0, 60,
			// pad
			0, 0, 0, 0,

			// key="D"
			0, 0, 0, 1, 'D', 0,
			// signature (uint8)
			1, 'y', 0,
			// val=1
			1,

			// pad to struct
			0, 0, 0, 0, 0, 0,

			// key="bar"
			0, 0, 0, 3, 'b', 'a', 'r', 0,
			// signature (uint32)
			1, 'u', 0,
			// pad
			0,
			// val=0, encodeZero
			0, 0, 0, 0,

			// key="a"
			0, 0, 0, 1, 'a', 0,
			// signature (uint8)
			1, 'y', 0,
			// val=2
			2,

			// pad to struct
			0, 0, 0, 0, 0, 0,

			// key="z"
			0, 0, 0, 1, 'z', 0,
			// signature (uint16)
			1, 'q', 0,
			// pad
			0,
			// val=3
			0, 3,
		),

		ok("vardict byte", "(a{yv})",
			VarDictByte{
				A: 42,
				B: "foo",
			},
			// dict length
			0, 0, 0, 20,
			// pad
			0, 0, 0, 0,

			// key=1
			1,
			// signature (uint16)
			1, 'q', 0,
			// val=42
			0, 42,

			// pad to struct
			0, 0,

			// key=2
			2,
			// signature (string)
			1, 's', 0,
			// val="foo"
			0, 0, 0, 3, 'f', 'o', 'o', 0),
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			v := reflect.New(reflect.TypeOf(tc.wantDecode))
			got := v.Interface()
			dec := fragments.Decoder{
				Order:  fragments.BigEndian,
				Mapper: decoderFor,
				In:     bytes.NewBuffer(tc.raw),
			}
			enc := fragments.Encoder{
				Order:  fragments.BigEndian,
				Mapper: encoderFor,
			}
			if tc.wantErr {
				if err := dec.Value(context.Background(), got); err == nil {
					t.Fatalf("decode succeeded, wanted error\n  raw: % x\n  got: %#v", tc.raw, got)
				}
				if err := enc.Value(context.Background(), tc.toEncode); err == nil {
					t.Fatalf("encode succeeded, wanted error\n  val: %#v\n  got: % x", tc.toEncode, enc.Out)
				}
				if sig, err := SignatureOf(tc.toEncode); err == nil {
					t.Fatalf("SignatureOf succeeded, wanted error\n  val: %#v\n  sig: %s", tc.toEncode, sig)
				}
			} else {
				if err := dec.Value(context.Background(), got); err != nil {
					t.Fatalf("decode failed: %v\n  raw: % x\n  want: %#v", err, tc.raw, tc.wantDecode)
				}
				if diff := cmp.Diff(v.Elem().Interface(), tc.wantDecode, cmpopts.EquateComparable(Signature{})); diff != "" {
					t.Fatalf("decode wrong encoding (-got+want):\n%s", diff)
				}
				if err := enc.Value(context.Background(), tc.toEncode); err != nil {
					t.Fatalf("encode failed: %v\n  val: %#v\n want: % x", err, tc.toEncode, tc.raw)
				}
				if !bytes.Equal(enc.Out, tc.raw) {
					t.Fatalf("encode wrong encoding:\n  val: %#v\n  got: % x\n want: % x", tc.toEncode, enc.Out, tc.raw)
				}
				sig, err := SignatureOf(tc.toEncode)
				if err != nil {
					t.Fatalf("SignatureOf failed: %v", err)
				}
				if s := sig.String(); s != tc.sigStr {
					t.Fatalf("wrong signature, got %q want %q", s, tc.sigStr)
				}
			}
		})
	}
}

func TestMarshal(t *testing.T) {
	var be, le = fragments.BigEndian, fragments.LittleEndian
	encName := map[fragments.ByteOrder]string{
		be: "BE",
		le: "LE",
	}

	tests := []struct {
		in   any
		enc  fragments.ByteOrder
		want []byte // empty means want error
	}{
		{byte(5), le, []byte{0x05}},
		{byte(5), be, []byte{0x05}},
		{true, le, []byte{0x01, 0x00, 0x00, 0x00}},
		{true, be, []byte{0x00, 0x00, 0x00, 0x01}},
		{false, le, []byte{0x00, 0x00, 0x00, 0x00}},
		{false, be, []byte{0x00, 0x00, 0x00, 0x00}},
		{int16(0x2bff), le, []byte{0xff, 0x2b}},
		{int16(0x2bff), be, []byte{0x2b, 0xff}},
		{uint16(0x2bff), le, []byte{0xff, 0x2b}},
		{uint16(0x2bff), be, []byte{0x2b, 0xff}},
		{int32(0x12342bff), le, []byte{0xff, 0x2b, 0x34, 0x12}},
		{int32(0x12342bff), be, []byte{0x12, 0x34, 0x2b, 0xff}},
		{uint32(0x12342bff), le, []byte{0xff, 0x2b, 0x34, 0x12}},
		{uint32(0x12342bff), be, []byte{0x12, 0x34, 0x2b, 0xff}},
		{int64(0x1abbccdd12342bff), le, []byte{
			0xff, 0x2b, 0x34, 0x12,
			0xdd, 0xcc, 0xbb, 0x1a,
		}},
		{int64(0x1abbccdd12342bff), be, []byte{
			0x1a, 0xbb, 0xcc, 0xdd,
			0x12, 0x34, 0x2b, 0xff,
		}},
		{uint64(0xaabbccdd12342bff), le, []byte{
			0xff, 0x2b, 0x34, 0x12,
			0xdd, 0xcc, 0xbb, 0xaa,
		}},
		{uint64(0xaabbccdd12342bff), be, []byte{
			0xaa, 0xbb, 0xcc, 0xdd,
			0x12, 0x34, 0x2b, 0xff,
		}},
		{float64(3402823700), le, []byte{
			0x00, 0x00, 0x80, 0x02,
			0x5F, 0x5A, 0xE9, 0x41,
		}},
		{float64(3402823700), be, []byte{
			0x41, 0xE9, 0x5A, 0x5F,
			0x02, 0x80, 0x00, 0x00,
		}},
		{"foobar", le, []byte{
			0x06, 0x00, 0x00, 0x00, // length
			0x66, 0x6f, 0x6f, 0x62, 0x61, 0x72, // str
			0x00, // terminator
		}},
		{"foobar", be, []byte{
			0x00, 0x00, 0x00, 0x06, // length
			0x66, 0x6f, 0x6f, 0x62, 0x61, 0x72, // str
			0x00, // terminator
		}},
		{[]byte{1, 2, 3}, le, []byte{
			0x03, 0x00, 0x00, 0x00, // length
			0x01, 0x02, 0x03, // bytes
		}},
		{[]byte{1, 2, 3}, be, []byte{
			0x00, 0x00, 0x00, 0x03, // length
			0x01, 0x02, 0x03, // bytes
		}},
		{[][]string{{"fo", "bar"}, {"qux"}}, le, []byte{
			0x20, 0x00, 0x00, 0x00, // length (arr)

			0x10, 0x00, 0x00, 0x00, // length (arr[0])

			0x02, 0x00, 0x00, 0x00, // length ("fo")
			0x66, 0x6f, // "fo"
			0x00, // terminator
			0x00, // pad to next str

			0x03, 0x00, 0x00, 0x00, // length ("bar")
			0x62, 0x61, 0x72, // "bar",
			0x00, // terminator

			0x08, 0x00, 0x00, 0x00, // length (arr[1])

			0x03, 0x00, 0x00, 0x00, // length ("qux")
			0x71, 0x75, 0x78, // "qux"
			0x00, // terminator
		}},
		{[][]string{{"fo", "bar"}, {"qux"}}, be, []byte{
			0x00, 0x00, 0x00, 0x20, // length (arr)

			0x00, 0x00, 0x00, 0x10, // length (arr[0])

			0x00, 0x00, 0x00, 0x02, // length ("fo")
			0x66, 0x6f, // "fo"
			0x00, // terminator
			0x00, // pad to next str

			0x00, 0x00, 0x00, 0x03, // length ("bar")
			0x62, 0x61, 0x72, // "bar",
			0x00, // terminator

			0x00, 0x00, 0x00, 0x08, // length (arr[1])

			0x00, 0x00, 0x00, 0x03, // length ("qux")
			0x71, 0x75, 0x78, // "qux"
			0x00, // terminator
		}},

		{Simple{42, true}, le, []byte{
			0x2a, 0x00, // Simple.A
			0x00, 0x00, // pad to bool alignment
			0x01, 0x00, 0x00, 0x00, // Simple.B
		}},
		{Simple{42, true}, be, []byte{
			0x00, 0x2a, // Simple.A
			0x00, 0x00, // pad to bool alignment
			0x00, 0x00, 0x00, 0x01, // Simple.B
		}},

		{WithAny{42, uint32(66)}, le, []byte{
			// .A
			0x2a, 0x00,

			// Signature for uint32
			0x01, 'u', 0x00,
			// Pad
			0x00, 0x00, 0x00,
			// .B
			0x42, 0x00, 0x00, 0x00,
		}},
		{WithAny{42, uint32(66)}, be, []byte{
			// .A
			0x00, 0x2a,

			// Signature for uint32
			0x01, 'u', 0x00,
			// Pad
			0x00, 0x00, 0x00,
			// .B
			0x00, 0x00, 0x00, 0x42,
		}},

		{Nested{66, Simple{42, true}}, le, []byte{
			// Nested.A
			0x42,
			// pad to struct
			0x00, 0x00, 0x00,
			0x00, 0x00, 0x00, 0x00,
			// Nested.B.A
			0x2a, 0x00,
			// pad to bool
			0x00, 0x00,
			// Nested.B.B
			0x01, 0x00, 0x00, 0x00,
		}},
		{Nested{66, Simple{42, true}}, be, []byte{
			// Nested.A
			0x42,
			// pad to struct
			0x00, 0x00, 0x00,
			0x00, 0x00, 0x00, 0x00,
			// Nested.B.A
			0x00, 0x2a,
			// pad to bool
			0x00, 0x00,
			// Nested.B.B
			0x00, 0x00, 0x00, 0x01,
		}},

		{Embedded{Simple{42, true}, 66}, le, []byte{
			// Embedded.Simple.A
			0x2a, 0x00,
			// pad to bool
			0x00, 0x00,
			// Embedded.Simple.B
			0x01, 0x00, 0x00, 0x00,
			// Embedded.C
			0x42,
		}},
		{Embedded{Simple{42, true}, 66}, be, []byte{
			// Embedded.Simple.A
			0x00, 0x2a,
			// pad to bool
			0x00, 0x00,
			// Embedded.Simple.B
			0x00, 0x00, 0x00, 0x01,
			// Embedded.C
			0x42,
		}},

		{Embedded_P{&Simple{42, true}, 66}, le, []byte{
			// Embedded.Simple.A
			0x2a, 0x00,
			// pad to bool
			0x00, 0x00,
			// Embedded.Simple.B
			0x01, 0x00, 0x00, 0x00,
			// Embedded.C
			0x42,
		}},
		{Embedded_P{&Simple{42, true}, 66}, be, []byte{
			// Embedded.Simple.A
			0x00, 0x2a,
			// pad to bool
			0x00, 0x00,
			// Embedded.Simple.B
			0x00, 0x00, 0x00, 0x01,
			// Embedded.C
			0x42,
		}},

		{Embedded_P{C: 66}, le, []byte{
			// Embedded.Simple.A
			0x00, 0x00,
			// pad to bool
			0x00, 0x00,
			// Embedded.Simple.B
			0x00, 0x00, 0x00, 0x00,
			// Embedded.C
			0x42,
		}},
		{Embedded_P{C: 66}, be, []byte{
			// Embedded.Simple.A
			0x00, 0x00,
			// pad to bool
			0x00, 0x00,
			// Embedded.Simple.B
			0x00, 0x00, 0x00, 0x00,
			// Embedded.C
			0x42,
		}},

		{Embedded_PVP{D: 66}, le, []byte{
			// Embedded_PVP.Embedded_PV.Embedded_P.Simple.A
			0x00, 0x00,
			// pad to bool
			0x00, 0x00,
			// Embedded_PVP.Embedded_PV.Embedded_P.Simple.B
			0x00, 0x00, 0x00, 0x00,
			// Embedded_PVP.Embedded_PV.Embedded_P.C
			0x00,
			// Embedded_PVP.D
			0x42,
		}},
		{Embedded_PVP{D: 66}, be, []byte{
			// Embedded_PVP.Embedded_PV.Embedded_P.Simple.A
			0x00, 0x00,
			// pad to bool
			0x00, 0x00,
			// Embedded_PVP.Embedded_PV.Embedded_P.Simple.B
			0x00, 0x00, 0x00, 0x00,
			// Embedded_PVP.Embedded_PV.Embedded_P.C
			0x00,
			// Embedded_PVP.D
			0x42,
		}},

		{EmbeddedShadow{Simple{42, true}, 66}, le, []byte{
			// Embedded.Simple.A
			0x2a, 0x00,
			// Embedded.B (shadowing Embedded.Simple.B)
			0x42,
		}},
		{EmbeddedShadow{Simple{42, true}, 66}, be, []byte{
			// Embedded.Simple.A
			0x00, 0x2a,
			// Embedded.B (shadowing Embedded.Simple.B)
			0x42,
		}},

		// SelfMarshalerVal ignores the overall byte order and always
		// writes big-endian.
		{SelfMarshalerVal{66}, le, []byte{
			0x00, 0x43,
		}},
		{SelfMarshalerVal{66}, be, []byte{
			0x00, 0x43,
		}},

		// SelfMarshalerVal ignores the overall byte order and always
		// writes big-endian.
		{&SelfMarshalerPtr{66}, le, []byte{0x00, 0x43}},
		{&SelfMarshalerPtr{66}, be, []byte{0x00, 0x43}},

		{&NestedSelfMarshalerPtr{66, SelfMarshalerPtr{42}}, le, []byte{
			// s.A
			0x42,
			// pad to marshaler value (deliberately weird/invalid for
			// dbus, to verify we're delegating to the Marshaler)
			0x00, 0x00,
			// s.B
			0x00, 0x2b,
		}},
		{&NestedSelfMarshalerPtr{66, SelfMarshalerPtr{42}}, be, []byte{
			// s.A
			0x42,
			// pad to marshaler value (deliberately weird/invalid for
			// dbus, to verify we're delegating to the Marshaler)
			0x00, 0x00,
			// s.B
			0x00, 0x2b,
		}},

		{[]SelfMarshalerVal{{1}, {2}}, le, []byte{
			// array length
			0x07, 0x00, 0x00, 0x00,
			// pad to multiple of 3
			0x00, 0x00,
			// arr[0]
			0x00, 0x02,
			// pad to multiple of 3
			0x00,
			0x00, 0x03,
		}},

		{ObjectPath("foo"), be, []byte{
			0x00, 0x00, 0x00, 0x03, // length
			0x66, 0x6f, 0x6f, // "foo"
			0x00, // terminator
		}},
		{ObjectPath("foo"), le, []byte{
			0x03, 0x00, 0x00, 0x00, // length
			0x66, 0x6f, 0x6f, // "foo"
			0x00, // terminator
		}},
		{mustSignatureFor[struct{ A, B uint32 }](), be, []byte{
			0x04,
			0x28, 0x75, 0x75, 0x29,
			0x00,
		}},
		{mustSignatureFor[struct{ A, B uint32 }](), le, []byte{
			0x04,
			0x28, 0x75, 0x75, 0x29,
			0x00,
		}},

		{map[uint16]string{
			1: "foo",
			2: "bar",
		}, be, []byte{
			0x00, 0x00, 0x00, 0x1c, // array len
			0x00, 0x00, 0x00, 0x00, // pad to struct

			0x00, 0x01, // key=1
			0x00, 0x00, // pad
			0x00, 0x00, 0x00, 0x03, // str len
			0x66, 0x6f, 0x6f, // "foo"
			0x00, // str terminator

			0x00, 0x00, 0x00, 0x00, // pad to struct
			0x00, 0x02, // key=2
			0x00, 0x00, // pad
			0x00, 0x00, 0x00, 0x03, // str len
			0x62, 0x61, 0x72, // "bar"
			0x00, // str terminator
		}},
		{map[uint16]string{
			1: "foo",
			2: "bar",
		}, le, []byte{
			0x1c, 0x00, 0x00, 0x00, // array len
			0x00, 0x00, 0x00, 0x00, // pad to struct

			0x01, 0x00, // key=1
			0x00, 0x00, // pad
			0x03, 0x00, 0x00, 0x00, // str len
			0x66, 0x6f, 0x6f, // "foo"
			0x00, // str terminator

			0x00, 0x00, 0x00, 0x00, // pad to struct
			0x02, 0x00, // key=2
			0x00, 0x00, // pad
			0x03, 0x00, 0x00, 0x00, // str len
			0x62, 0x61, 0x72, // "bar"
			0x00, // str terminator
		}},

		{VarDict{
			A: 1,
			B: 2,
			C: "foo",
		}, le, []byte{
			0x36, 0x00, 0x00, 0x00, // length
			0x00, 0x00, 0x00, 0x00, // pad to struct

			0x01, 0x00, 0x00, 0x00, // str length
			0x43, // key="C"
			0x00, // str terminator

			0x01, 0x73, 0x00, // signature (string)
			0x00, 0x00, 0x00, // pad
			0x03, 0x00, 0x00, 0x00, // str length
			0x66, 0x6f, 0x6f, // "foo"
			0x00, // str terminator

			0x00, 0x00, 0x00, 0x00, // pad to struct

			0x03, 0x00, 0x00, 0x00, // str length
			0x62, 0x61, 0x72, // key="bar"
			0x00, // str terminator

			0x01, 0x75, 0x00, // signature (uint32)
			0x00,                   // pad
			0x02, 0x00, 0x00, 0x00, // val=2

			// no struct pad needed

			0x03, 0x00, 0x00, 0x00, // str length
			0x66, 0x6f, 0x6f, // key="foo"
			0x00, // str terminator

			0x01, 0x71, 0x00, // signature (uint16)
			0x00,       // pad
			0x01, 0x00, // val=1

			// key D omitted because zero
		}},
		{VarDict{
			A: 1,
			B: 2,
			C: "foo",
		}, be, []byte{
			0x00, 0x00, 0x00, 0x36, // length
			0x00, 0x00, 0x00, 0x00, // pad to struct

			0x00, 0x00, 0x00, 0x01, // str length
			0x43, // key="C"
			0x00, // str terminator

			0x01, 0x73, 0x00, // signature (string)
			0x00, 0x00, 0x00, // pad
			0x00, 0x00, 0x00, 0x03, // str length
			0x66, 0x6f, 0x6f, // "foo"
			0x00, // str terminator

			0x00, 0x00, 0x00, 0x00, // pad to struct

			0x00, 0x00, 0x00, 0x03, // str length
			0x62, 0x61, 0x72, // key="bar"
			0x00, // str terminator

			0x01, 0x75, 0x00, // signature (uint32)
			0x00,                   // pad
			0x00, 0x00, 0x00, 0x02, // val=2

			// no struct pad needed

			0x00, 0x00, 0x00, 0x03, // str length
			0x66, 0x6f, 0x6f, // key="foo"
			0x00, // str terminator

			0x01, 0x71, 0x00, // signature (uint16)
			0x00,       // pad
			0x00, 0x01, // val=1

			// key D omitted because zero
		}},

		{VarDict{
			D: 1,
			Other: map[string]any{
				"a": uint8(2),
				"z": uint16(3),
			},
		}, le, []byte{
			0x3c, 0x00, 0x00, 0x00, // length
			0x00, 0x00, 0x00, 0x00, // pad to struct

			0x01, 0x00, 0x00, 0x00, // str len
			0x44, // key="D"
			0x00, // str terminator

			0x01, 0x79, 0x00, // signature (uint8)
			0x01, // val=1

			0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // pad to struct

			0x03, 0x00, 0x00, 0x00, // str len
			0x62, 0x61, 0x72, // key="bar"
			0x00, // str terminator

			0x01, 0x75, 0x00, // signature (uint32)
			0x00,
			0x00, 0x00, 0x00, 0x00, // val=0, encodeZero

			// no struct padding needed

			0x01, 0x00, 0x00, 0x00, // str length
			0x61, // key="a"
			0x00, // str terminator

			0x01, 0x79, 0x00, // signature (uint8)
			0x02, // val=2

			0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // pad to struct

			0x01, 0x00, 0x00, 0x00, // str length
			0x7a, // key="z"
			0x00, // str terminator

			0x01, 0x71, 0x00, // signature (uint16)
			0x00,       // pad
			0x03, 0x00, // val=3
		}},
		{VarDict{
			D: 1,
			Other: map[string]any{
				"a": uint8(2),
				"z": uint16(3),
			},
		}, be, []byte{
			0x00, 0x00, 0x00, 0x3c, // length
			0x00, 0x00, 0x00, 0x00, // pad to struct

			0x00, 0x00, 0x00, 0x01, // str len
			0x44, // key="D"
			0x00, // str terminator

			0x01, 0x79, 0x00, // signature (uint8)
			0x01, // val=1

			0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // pad to struct

			0x00, 0x00, 0x00, 0x03, // str len
			0x62, 0x61, 0x72, // key="bar"
			0x00, // str terminator

			0x01, 0x75, 0x00, // signature (uint32)
			0x00,
			0x00, 0x00, 0x00, 0x00, // val=0, encodeZero

			// no struct padding needed

			0x00, 0x00, 0x00, 0x01, // str length
			0x61, // key="a"
			0x00, // str terminator

			0x01, 0x79, 0x00, // signature (uint8)
			0x02, // val=2

			0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // pad to struct

			0x00, 0x00, 0x00, 0x01, // str length
			0x7a, // key="z"
			0x00, // str terminator

			0x01, 0x71, 0x00, // signature (uint16)
			0x00,       // pad
			0x00, 0x03, // val=3
		}},

		{func() int { return 2 }, le, nil},
		{func() int { return 2 }, be, nil},

		// Not addressable, and no exported fields - can't convert.
		{SelfMarshalerPtr{66}, le, []byte{}},
		{SelfMarshalerPtr{66}, be, []byte{}},
	}

	for _, tc := range tests {
		enc := fragments.Encoder{
			Order:  tc.enc,
			Mapper: encoderFor,
		}
		if err := enc.Value(context.Background(), tc.in); err != nil {
			if len(tc.want) != 0 {
				t.Errorf("Marshal(%T) got err: %v", tc.in, err)
			} else if testing.Verbose() {
				t.Logf("Marshal(%T:%#v, %s) = err: %v", tc.in, tc.in, encName[tc.enc], err)
			}
		} else if len(tc.want) == 0 {
			t.Errorf("Marshal(%T) encoded successfully, want error", tc.in)
		} else if !bytes.Equal(enc.Out, tc.want) {
			t.Errorf("Marshal(%T) wrong encoding:\n  got: % x\n want: % x", tc.in, enc.Out, tc.want)
		} else if testing.Verbose() {
			t.Logf("Marshal(%T:%#v, %s) = % x", tc.in, tc.in, encName[tc.enc], enc.Out)
		}
	}
}
