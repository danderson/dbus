package dbus

import (
	"context"
	"errors"
	"fmt"
	"reflect"
)

type Interface struct {
	o    Object
	name string
}

func (f Interface) Conn() *Conn    { return f.o.Conn() }
func (f Interface) Peer() Peer     { return f.o.Peer() }
func (f Interface) Object() Object { return f.o }
func (f Interface) Name() string   { return f.name }

func (f Interface) String() string {
	if f.name == "" {
		return fmt.Sprintf("%s:<no interface>", f.Object())
	}
	return fmt.Sprintf("%s:%s", f.Object(), f.name)
}

func (f Interface) Call(ctx context.Context, method string, body any, response any, opts ...CallOption) error {
	return f.Conn().call(ctx, f.Peer().name, f.Object().path, f.name, method, body, response, opts...)
}

func (f Interface) GetProperty(ctx context.Context, name string, val any, opts ...CallOption) error {
	want := reflect.ValueOf(val)
	if !want.IsValid() {
		return errors.New("cannot read property into nil interface")
	}
	if want.Kind() != reflect.Pointer {
		return errors.New("cannot read property into non-pointer")
	}
	if want.IsNil() {
		return errors.New("cannot read property into nil pointer")
	}

	var resp Variant
	req := struct {
		InterfaceName string
		PropertyName  string
	}{f.name, name}
	err := f.Object().Interface("org.freedesktop.DBus.Properties").Call(ctx, "Get", req, &resp, opts...)
	if err != nil {
		return err
	}

	got := reflect.ValueOf(resp.Value)
	if !got.Type().AssignableTo(want.Type().Elem()) {
		return fmt.Errorf("property type %s is not assignable to %s", got.Type(), want.Type())
	}
	want.Elem().Set(got)

	return nil
}

func (f Interface) SetProperty(ctx context.Context, name string, value any, opts ...CallOption) error {
	req := struct {
		InterfaceName string
		PropertyName  string
		Value         Variant
	}{f.name, name, Variant{value}}
	return f.Object().Interface("org.freedesktop.DBus.Properties").Call(ctx, "Set", req, nil, opts...)
}

func (f Interface) GetAllProperties(ctx context.Context, opts ...CallOption) (map[string]any, error) {
	var resp map[string]Variant
	err := f.Object().Interface("org.freedesktop.DBus.Properties").Call(ctx, "GetAll", f.name, &resp, opts...)
	if err != nil {
		return nil, err
	}

	ret := make(map[string]any, len(resp))
	for k, v := range resp {
		ret[k] = v.Value
	}
	return ret, nil
}
