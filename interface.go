package dbus

import (
	"context"
	"errors"
	"fmt"
	"reflect"
)

// Interface is a set of methods, properties and signals offered by an
// [Object].
type Interface struct {
	o    Object
	name string
}

// Conn returns the DBus connection associated with the interface.
func (f Interface) Conn() *Conn { return f.o.Conn() }

// Peer returns the Peer that is offering the interface.
func (f Interface) Peer() Peer { return f.o.Peer() }

// Object returns the Object that implements the interface.
func (f Interface) Object() Object { return f.o }

// Name returns the name of the interface.
func (f Interface) Name() string { return f.name }

func (f Interface) String() string {
	if f.name == "" {
		return fmt.Sprintf("%s:<no interface>", f.Object())
	}
	return fmt.Sprintf("%s:%s", f.Object(), f.name)
}

// Call calls method on the interface with the given request body, and
// writes the response into response.
//
// This is a low-level calling API. It is the caller's responsibility
// to match the body and response types to the signature of the method
// being invoked. Body may be nil for methods that accept no
// parameters. Response may be nil for methods that return no values.
func (f Interface) Call(ctx context.Context, method string, body any, response any) error {
	return f.Conn().call(ctx, f.Peer().Name(), f.Object().Path(), f.Name(), method, body, response, false)
}

// OneWay calls method on the interface with the given request body,
// and tells the peer not to send a reply.
//
// OneWay returns after the method call is successfully sent. Since
// the response is suppressed at the bus level, there is no way to
// know whether the call was delivered to anyone, or acted upon.
//
// This is a low-level calling API. It is the caller's responsibility
// to match the body to the signature of the method being
// invoked. Body may be nil for methods that accept no parameters.
func (f Interface) OneWay(ctx context.Context, method string, body any) error {
	return f.Conn().call(ctx, f.Peer().Name(), f.Object().Path(), f.Name(), method, body, nil, true)
}

// GetProperty reads the value of the given property into val.
//
// It is the caller's responsibility to match the value's type to the
// type offered by the interface. val may also be of type *any to
// retrieve a property without knowing its type.
func (f Interface) GetProperty(ctx context.Context, name string, val any) error {
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
	err := f.Object().Interface(ifaceProps).Call(ctx, "Get", req, &resp)
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

// SetProperty sets the given property to value.
//
// It is the caller's responsibility to match the value's type to the
// type offered by the interface.
func (f Interface) SetProperty(ctx context.Context, name string, value any) error {
	req := struct {
		InterfaceName string
		PropertyName  string
		Value         Variant
	}{f.name, name, Variant{value}}
	return f.Object().Interface(ifaceProps).Call(ctx, "Set", req, nil)
}

// GetAllProperties returns all the properties exported by the
// interface.
func (f Interface) GetAllProperties(ctx context.Context) (map[string]any, error) {
	var resp map[string]Variant
	err := f.Object().Interface(ifaceProps).Call(ctx, "GetAll", f.name, &resp)
	if err != nil {
		return nil, err
	}

	ret := make(map[string]any, len(resp))
	for k, v := range resp {
		ret[k] = v.Value
	}
	return ret, nil
}
