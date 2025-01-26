package dbus

import (
	"fmt"
	"reflect"
)

// TypeError is the error returned when a type cannot be represented
// in the DBus wire format.
type TypeError struct {
	// Type is the name of the type that caused the error.
	Type string
	// Reason is an explanation of why the type isn't representable by
	// DBus.
	Reason error
}

func (e TypeError) Error() string {
	return fmt.Sprintf("dbus cannot represent %s: %s", e.Type, e.Reason)
}

func (e TypeError) Unwrap() error {
	return e.Reason
}

func typeErr(t reflect.Type, reason string, args ...any) error {
	ts := ""
	if t != nil {
		ts = t.String()
	}
	return TypeError{ts, fmt.Errorf(reason, args...)}
}

// CallError is the error returned from failed DBus method calls.
type CallError struct {
	// Name is the error name provided by the remote peer.
	Name string
	// Detail is the human-readable explanation of what went wrong.
	Detail string
}

func (e CallError) Error() string {
	if e.Detail == "" {
		return fmt.Sprintf("call error %s", e.Name)
	}
	return fmt.Sprintf("call error %s: %s", e.Name, e.Detail)
}
