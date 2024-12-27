// package fragments provides low-level encoding and decoding helpers
// to construct and parse DBus message.
//
// The provided encoder and decoder are very low level, and do not
// encode any DBus semantics. It is the caller's responsibility to
// produce valid DBus messages using these tools.
//
// You should not need to use this package at all, unless you are
// writing your own dbus.Marshaler/dbus.Unmarshaler implementations,
// in which case your code will be handed a
// [fragment.Encoder]/[fragment.Decoder] and expected to produce
// correct DBus fragments with it.
package fragments
