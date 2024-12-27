package dbus

import (
	"fmt"
	"reflect"
)

type ErrUnrepresentable struct {
	Type   string
	Reason string
}

func (e ErrUnrepresentable) Error() string {
	return fmt.Sprintf("dbus cannot represent %s: %s", e.Type, e.Reason)
}

func unrepresentable(t reflect.Type, reason string) error {
	ts := ""
	if t != nil {
		ts = t.String()
	}
	return ErrUnrepresentable{ts, reason}
}
