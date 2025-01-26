package dbus

import (
	"bytes"
	"context"
	"reflect"
	"testing"

	"github.com/danderson/dbus/fragments"
	"github.com/google/go-cmp/cmp"
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
	fail := func(name string, toEncode any, raw ...byte) testCase {
		return testCase{name, raw, toEncode, toEncode, "", true}
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

		ok("bytes", "ay", []byte("foobar"),
			// Length
			0, 0, 0, 6,
			// Value
			'f', 'o', 'o', 'b', 'a', 'r'),

		ok("string", "s", "foobar",
			// Length
			0, 0, 0, 6,
			// Value
			'f', 'o', 'o', 'b', 'a', 'r',
			// Terminator
			0),
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

		ok("[]uint16", "aq",
			[]uint16{1, 2},
			// array length
			0, 0, 0, 4,
			// first val
			0, 1,
			// second val
			0, 2),
		ok("[][]uint16", "aaq",
			[][]uint16{{1}, {2, 3}},
			// outer array length
			0, 0, 0, 16,
			// first array
			0, 0, 0, 2,
			0, 1,
			// pad
			0, 0,
			// second array
			0, 0, 0, 4,
			0, 2,
			0, 3),

		ok("sig(byte)", "g",
			mustSignatureFor[byte](),
			1, 'y', 0),
		ok("sig(ObjectPath)", "g",
			mustSignatureFor[[]ObjectPath](),
			2, 'a', 'o', 0),
		ok("sig(struct)", "g",
			mustSignatureFor[struct {
				A byte
				B bool
			}](),
			4, '(', 'y', 'b', ')', 0),

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

		ok("struct selfmarshaler ptr", "q",
			&SelfMarshalerPtr{41},
			0, 42),

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

		ok("object path", "o",
			ObjectPath("foo"),
			0, 0, 0, 3, 'f', 'o', 'o', 0),

		ok("map string", "a{qs}",
			map[uint16]string{
				1: "foo",
				2: "bar",
			},
			// dict length
			0, 0, 0, 28,
			// pad
			0, 0, 0, 0,

			// key=1
			0, 1,
			// pad
			0, 0,
			// val="foo"
			0, 0, 0, 3, 'f', 'o', 'o', 0,

			// pad to struct
			0, 0, 0, 0,

			// key=2
			0, 2,
			// pad
			0, 0,
			// val="bar"
			0, 0, 0, 3, 'b', 'a', 'r', 0,
		),

		ok("map ints", "a{qy}",
			map[uint16]uint8{
				1: 2,
				3: 4,
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

		fail("func",
			func() int { return 2 }),
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
				sigt := cmp.Transformer("sig", func(s Signature) string {
					return s.String()
				})
				if diff := cmp.Diff(v.Elem().Interface(), tc.wantDecode, sigt); diff != "" {
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

func TestMarshalInvalid(t *testing.T) {
	enc := fragments.Encoder{
		Order:  fragments.BigEndian,
		Mapper: encoderFor,
	}
	dec := fragments.Decoder{
		Order:  fragments.BigEndian,
		Mapper: decoderFor,
	}

	if err := enc.Value(context.Background(), SelfMarshalerVal{41}); err != nil {
		t.Fatalf("encode SelfMarshalerVal{41} failed: %v", err)
	}
	if got, want := enc.Out, []byte{42, 0}; !bytes.Equal(got, want) {
		t.Fatalf("encode SelfMarshalerVal{41} wrong output:\n  got: % x\n want: % x", got, want)
	}
	var v SelfMarshalerVal
	if err := dec.Value(context.Background(), &v); err == nil {
		t.Fatal("decode SelfMarshalerVal succeeded, want error")
	}

	enc.Out = nil
	n := NestedSelfMarshalerVal{
		A: 42,
		B: SelfMarshalerVal{
			B: 41,
		},
	}
	if err := enc.Value(context.Background(), n); err != nil {
		t.Fatalf("encode NestedSelfMarshalerVal failed: %v", err)
	}
	want := []byte{
		// .A
		42,
		// pad to 3 (SelfMarshalerVal has deliberately weird padding)
		0, 0,
		// .B.B
		42, 0,
	}
	if got := enc.Out; !bytes.Equal(got, want) {
		t.Fatalf("encode NestedSelfMarshalerVal wrong output:\n  got: % x\n want: % x", got, want)
	}
	var nv NestedSelfMarshalerVal
	if err := dec.Value(context.Background(), &nv); err == nil {
		t.Fatal("decode NestedSelfMarshalerVal succeeded, want error")
	}

	enc.Out = nil
	p := SelfMarshalerPtr{41}
	if err := enc.Value(context.Background(), p); err == nil {
		t.Fatal("encode SelfMarshalerPtr value succeeded, want error")
	}
	dec.In = bytes.NewBuffer([]byte{0, 42})
	if err := dec.Value(context.Background(), &p); err != nil {
		t.Fatalf("decode SelfMarshalerPtr value failed: %v", err)
	}

	if err := enc.Value(context.Background(), nil); err == nil {
		t.Fatal("encode nil succeeded, want error")
	}
	if err := dec.Value(context.Background(), nil); err == nil {
		t.Fatal("decode nil succeeded, want error")
	}
	var nilp *uint16
	if err := dec.Value(context.Background(), nilp); err == nil {
		t.Fatal("decode nilptr succeeded, want error")
	}
	var nonp uint16
	if err := dec.Value(context.Background(), nonp); err == nil {
		t.Fatal("decode nilptr succeeded, want error")
	}
}
