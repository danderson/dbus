package dbus

// marshal returns the DBus wire encoding of v, using the given byte
// ordering.
//
// Marshal traverses the value v recursively. If an encountered value
// implements [Marshaler], Marshal calls MarshalDBus on it to produce
// its encoding.
//
// Otherwise, Marshal uses the following type-dependent default
// encodings:
//
// uint{8,16,32,64}, int{16,32,64}, float64, bool and string values
// encode to the corresponding DBus basic type.
//
// Array and slice values encode as DBus arrays. Nil slices encode the
// same as an empty slice.
//
// Struct values encode as DBus structs. Each exported struct field is
// encoded in declaration order, according to its own type. Embedded
// struct fields are encoded as if their inner exported fields were
// fields in the outer struct, subject to the usual Go visibility
// rules.
//
// Map values encode as a DBus dictionary, i.e. an array of key/value
// pairs. The map's key underlying type must be uint{8,16,32,64},
// int{16,32,64}, float64, bool, or string.
//
// Several DBus protocols use map[K]any values to extend structs with
// new fields in a backwards compatible way. To support this "vardict"
// idiom, structs may contain a single "vardict" field and several
// "associated" fields:
//
//	struct Vardict{
//	    // A "vardict" map for the struct.
//	    M map[uint8]any `dbus:"vardict"`
//
//	    // "associated" fields. Associated fields can be declared
//	    // anywhere in the struct, before or after the vardict field.
//	    Foo string `dbus:"key=1"`
//	    Bar uint32 `dbus:"key=2"`
//	}
//
// A vardict field encodes as a DBus dictionary just like a regular
// map, except that associated fields with nonzero values are encoded
// as additional key/value pairs. An associated field can be tagged
// with `dbus:"key=X,encodeZero"` to encode its zero value as well.
//
// Pointer values encode as the value pointed to. A nil pointer
// encodes as the zero value of the type pointed to.
//
// [Signature], [ObjectPath], and [File] values encode to the
// corresponding DBus types.
//
// 'any' values encode as DBus variants. The interface's inner value
// must be a valid value according to these rules, or Marshal will
// return a [TypeError].
//
// int8, int, uint, uintptr, complex64, complex128, interface,
// channel, and function values cannot be encoded. Attempting to
// encode such values causes Marshal to return a [TypeError].
//
// DBus cannot represent cyclic or recursive types. Attempting to
// encode such values causes Marshal to return a [TypeError].

// unmarshal reads a DBus message from r and stores the result in the
// value pointed to by v. If v is nil or not a pointer, Unmarshal
// returns a [TypeError].
//
// Generally, Unmarshal applies the inverse of the rules used by
// [Marshal]. The layout of the wire message must be compatible with
// the target's DBus signature. Since messages generally do not embed
// their signature, it is up to the caller to know the expected
// message format and match it.
//
// Unmarshal traverses the value v recursively. If an encountered
// value implements [Unmarshaler], Unmarshal calls UnmarshalDBus to
// unmarshal it. Types implementing [Unmarshaler] must implement
// UnmarshalDBus with a pointer receiver. Attempting to unmarshal
// using an UnmarshalDBus method with a value receiver results in a
// [TypeError].
//
// Otherwise, Unmarshal uses the following type-dependent default
// encodings:
//
// uint{8,16,32,64}, int{16,32,64}, float64, bool and string values
// encode the corresponding DBus basic types.
//
// Array and slice values decode DBus arrays. When decoding into an
// array, the message's array length must match the target array's
// length. When decoding into a slice, Unmarshal resets the slice
// length to zero and then appends each element to the slice.
//
// Struct values decode DBus structs. The message's fields decode into
// the target struct's fields in declaration order. Embedded struct
// fields are decoded as if their inner exported fields were fields in
// the outer struct, subject to the usual Go visibility rules.
//
// Maps decode DBus dictionaries. When decoding into a map, Unmarshal
// first clears the map, or allocates a new one if the target map is
// nil. Then, the incoming key-value pairs are stored into the map in
// message order. If the incoming dictionary contains duplicate values
// for a key, all but the last value are discarded.
//
// Several DBus protocols use map[K]any values to extend structs with
// new fields in a backwards compatible way. To support this "vardict"
// idiom, structs may contain a single "vardict" field and several
// "associated" fields:
//
//	struct Vardict{
//	    // A "vardict" map for the struct.
//	    M map[uint8]any `dbus:"vardict"`
//
//	    // "associated" fields. Associated fields can be declared
//	    // anywhere in the struct, before or after the vardict field.
//	    Foo string `dbus:"key=1"`
//	    Bar uint32 `dbus:"key=2"`
//	}
//
// A vardict field decodes a DBus dictionary just like regular map,
// except that if an incoming key matches an associated field's tag,
// the corresponding value decodes into that associated field
// instead. If the associated field's type is incompatible with the
// received map value, Unmarshal returns a [TypeError].
//
// Pointers decode as the value pointed to. Unmarshal allocates zero
// values as needed when it encounters nil pointers.
//
// [Signature], [ObjectPath], and [File] decode the corresponding DBus
// types.
//
// 'any' values decode DBus variants. The type of the variant's inner
// value is determined by the type signature carried in the
// message. Variants containing a struct are decoded into an anonymous
// struct with fields named Field0, Field1, ..., FieldN in message
// order.
//
// int8, int, uint, uintptr, complex64, complex128, interface,
// channel, and function values cannot decode any DBus type.
// Attempting to decode such values causes Unmarshal to return a
// [TypeError].
//
// DBus cannot represent cyclic or recursive types. Attempting to
// decode into such values causes Unmarshal to return a
// [TypeError].
