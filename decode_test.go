package dbus_test

import (
	"encoding/binary"
	"reflect"
	"testing"

	"github.com/danderson/dbus"
	"github.com/google/go-cmp/cmp"
)

func TestTypeDecoder(t *testing.T) {
	type testCase struct {
		in      []byte
		want    any
		wantErr bool
	}
	ok := func(raw []byte, want any) testCase {
		return testCase{raw, want, false}
	}
	fail := func(raw []byte, want any) testCase {
		return testCase{raw, want, true}
	}
	tests := []testCase{
		ok([]byte{0x00, 0x00, 0x00, 0x01}, ptr(true)),

		ok([]byte{0x42}, ptr(int8(0x42))),

		ok([]byte{0x42}, ptr(uint8(0x42))),

		ok([]byte{0x41, 0x42}, ptr(int16(0x4142))),

		ok([]byte{0x41, 0x42}, ptr(uint16(0x4142))),

		ok([]byte{0x39, 0x40, 0x41, 0x42}, ptr(int32(0x39404142))),

		ok([]byte{0x39, 0x40, 0x41, 0x42}, ptr(uint32(0x39404142))),

		ok([]byte{
			0x35, 0x36, 0x37, 0x38,
			0x39, 0x40, 0x41, 0x42},
			ptr(int64(0x3536373839404142))),

		ok([]byte{
			0x35, 0x36, 0x37, 0x38,
			0x39, 0x40, 0x41, 0x42},
			ptr(uint64(0x3536373839404142))),

		ok([]byte{
			0x41, 0xE9, 0x5A, 0x5F,
			0x02, 0x80, 0x00, 0x00},
			ptr(float32(3402823700))),

		ok([]byte{
			0x41, 0xE9, 0x5A, 0x5F,
			0x02, 0x80, 0x00, 0x00},
			ptr(float64(3402823700))),

		ok([]byte{
			// Length
			0x00, 0x00, 0x00, 0x03,
			// "foo"
			0x66, 0x6f, 0x6f,
			// Terminator
			0x00,
		}, ptr("foo")),

		ok([]byte{
			0x00, 0x00, 0x00, 0x02,
			0x00, 0x01,
			0x00, 0x02},
			ptr([]uint16{1, 2})),

		ok([]byte{
			0x00, 0x00, 0x00, 0x02,
			0x00, 0x00, 0x00, 0x01,
			0x00, 0x01,
			0x00, 0x00, // padding
			0x00, 0x00, 0x00, 0x02,
			0x00, 0x02,
			0x00, 0x03},
			ptr([][]uint16{{1}, {2, 3}})),

		ok([]byte{
			0x00, 0x2a,
			0x00, 0x00, // padding
			0x00, 0x00, 0x00, 0x01},
			ptr(Simple{42, true})),

		ok([]byte{
			0x42,
			0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // padding
			0x00, 0x2a,
			0x00, 0x00, // padding
			0x00, 0x00, 0x00, 0x01},
			ptr(Nested{66, Simple{42, true}})),

		ok([]byte{
			0x00, 0x2a,
			0x00, 0x00, // padding
			0x00, 0x00, 0x00, 0x01,
			0x42},
			ptr(Embedded{Simple{42, true}, 66})),

		ok([]byte{
			0x00, 0x2a,
			0x00, 0x00, // padding
			0x00, 0x00, 0x00, 0x01,
			0x42},
			ptr(Embedded_P{&Simple{42, true}, 66})),

		ok([]byte{
			0x00, 0x2a,
			0x00, 0x00, // padding
			0x00, 0x00, 0x00, 0x01,
			0x42},
			ptr(Embedded_PV{Embedded_P{&Simple{42, true}, 66}})),

		ok([]byte{
			0x00, 0x2a,
			0x00, 0x00, // padding
			0x00, 0x00, 0x00, 0x01,
			0x42,
			0x2a},
			ptr(Embedded_PVP{&Embedded_PV{Embedded_P{&Simple{42, true}, 66}}, 42})),
		ok([]byte{
			0x00, 0x2a,
			0x42},
			ptr(EmbeddedShadow{Simple{42, false}, 66})),

		ok([]byte{
			0x2a,
			0x00, 0x00,
			0x00, 0x2a,
		}, ptr(NestedSelfMarshalerPtr{42, SelfMarshalerPtr{41}})),

		ok([]byte{
			0x2a,
			0x00, 0x00,
			0x00, 0x2a,
		}, ptr(NestedSelfMarshalerPtrPtr{42, &SelfMarshalerPtr{41}})),

		ok([]byte{
			0x00, 0x00, 0x00, 0x02, // dict len

			0x00, 0x00, 0x00, 0x00, // pad to struct
			0x00, 0x01, // key=1
			0x02, // val=2

			0x00, 0x00, 0x00, 0x00, 0x00, // pad to struct
			0x00, 0x03, // key=3
			0x04, // val=4
		}, ptr(map[uint16]uint8{1: 2, 3: 4})),

		ok([]byte{
			0x00, 0x00, 0x00, 0x02, // dict len

			0x00, 0x00, 0x00, 0x00, // pad to struct
			0x00, 0x01, // key=1
			0x02, // val=2

			0x00, 0x00, 0x00, 0x00, 0x00, // pad to struct
			0x00, 0x03, // key=3
			0x04, // val=4
		}, ptr(map[uint16]*uint8{
			1: ptr[uint8](2),
			3: ptr[uint8](4),
		})),

		ok([]byte{0x00, 0x2a}, ptr(SelfMarshalerPtr{41})),

		fail(nil, nil),
		fail([]byte{0x00, 0x2a}, ptr(SelfMarshalerVal{})),
		fail([]byte{
			0x2a,
			0x00, 0x00,
			0x00, 0x2a},
			ptr(NestedSelfMashalerVal{0, SelfMarshalerVal{0}})),
	}

	for _, tc := range tests {
		testUnmarshal(t, tc.in, tc.want, tc.wantErr)
		if !tc.wantErr {
			testRoundTrip(t, tc.want)
		}

		// For cases with a non-nil want pointer, run the test again
		// with another layer of pointer indirection.
		v := reflect.ValueOf(tc.want)
		if !v.IsValid() {
			continue
		}
		p := reflect.New(v.Type())
		p.Elem().Set(v)
		testUnmarshal(t, tc.in, p.Interface(), tc.wantErr)
		if !tc.wantErr {
			testRoundTrip(t, p.Interface())
		}
	}
}

func testUnmarshal(t *testing.T, in []byte, want any, wantErr bool) {
	var got any
	if want != nil {
		if reflect.TypeOf(want).Kind() != reflect.Pointer {
			panic("tc.want must be a pointer")
		}
		got = reflect.New(reflect.TypeOf(want).Elem()).Interface()
	}

	err := dbus.Unmarshal(in, binary.BigEndian, got)
	if err != nil {
		if !wantErr {
			t.Errorf("Unmarshal(..., %T) got err: %v", want, err)
		} else if testing.Verbose() {
			t.Logf("Unmarshal(..., %T) = err: %v", want, err)
		}
	} else if wantErr {
		t.Errorf("Unmarshal(..., %T) decoded successfully, want error", want)
	} else if diff := cmp.Diff(got, want); diff != "" {
		t.Errorf("Unmarshal(..., %T) decoded incorrectly (-got+want):\n%s", want, diff)
	} else if testing.Verbose() {
		t.Logf("Unmarshal(..., %T) = ptr(%#v)", want, reflect.ValueOf(got).Elem().Interface())
	}
}

func testRoundTrip(t *testing.T, val any) {
	bs, err := dbus.Marshal(val, binary.BigEndian)
	if err != nil {
		t.Errorf("re-Marshal(%T) got err: %v", val, err)
	}
	testUnmarshal(t, bs, val, false)
}
