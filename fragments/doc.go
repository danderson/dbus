// package fragments provides low-level encoding and decoding helpers
// to construct and parse DBus message.
//
// The provided encoder and decoder are low level tools, and do not
// ensure that all outputs are valid DBus messages.
//
// You should not need to use this package at all, unless you are
// writing your own [github.com/danderson/dbus.Marshaler] or
// [github.com/danderson/dbus.Unmarshaler], in which case your code
// will be handed an [Encoder] or [Decoder] and expected to produce
// correct DBus wire data with it.
package fragments
