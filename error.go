package dbus

import (
	"fmt"
	"reflect"
)

type TypeError struct {
	Type   string
	Reason string
}

func (e TypeError) Error() string {
	return fmt.Sprintf("dbus cannot represent %s: %s", e.Type, e.Reason)
}

func typeErr(t reflect.Type, reason string) error {
	ts := ""
	if t != nil {
		ts = t.String()
	}
	return TypeError{ts, reason}
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
