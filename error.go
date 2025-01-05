package dbus

import (
	"fmt"
	"reflect"
)

type TypeError struct {
	Type   string
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

type CallError struct {
	Name   string
	Detail string
}

func (e CallError) Error() string {
	if e.Detail == "" {
		return fmt.Sprintf("call error: %s", e.Name)
	}
	return fmt.Sprintf("call error %s: %s", e.Name, e.Detail)
}
