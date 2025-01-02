package dbus

import (
	"context"
	"fmt"
)

type Interface struct {
	o    Object
	name string
}

func (f Interface) Conn() *Conn    { return f.o.Conn() }
func (f Interface) Peer() Peer     { return f.o.Peer() }
func (f Interface) Object() Object { return f.o }
func (f Interface) Name() string   { return f.name }

func (f Interface) Call(ctx context.Context, method string, body any, response any, opts ...CallOption) error {
	return f.Conn().call(ctx, f.Peer().name, f.Object().path, f.name, method, body, response, opts...)
}

func (f Interface) GetProp(ctx context.Context, name string, opts ...CallOption) (any, error) {
	var resp Variant
	req := struct {
		InterfaceName string
		PropertyName  string
	}{f.name, name}
	err := f.Object().Interface("org.freedesktop.DBus.Properties").Call(ctx, "Get", req, &resp, opts...)
	if err != nil {
		return nil, err
	}
	return resp.Value, nil
}

func (f Interface) SetProp(ctx context.Context, name string, value any, opts ...CallOption) error {
	req := struct {
		InterfaceName string
		PropertyName  string
		Value         Variant
	}{f.name, name, Variant{value}}
	return f.Object().Interface("org.freedesktop.DBus.Properties").Call(ctx, "Set", req, nil, opts...)
}

func (f Interface) GetAll(ctx context.Context, opts ...CallOption) (map[string]any, error) {
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

func GetProperty[T any](ctx context.Context, iface Interface, name string) (T, error) {
	v, err := iface.GetProp(ctx, name)
	if err != nil {
		var zero T
		return zero, err
	}
	ret, ok := v.(T)
	if !ok {
		var zero T
		return zero, fmt.Errorf("property %q has type %T, not %T", name, v, zero)
	}

	return ret, nil
}

func Call[Resp any, Req any](ctx context.Context, iface Interface, name string, body Req) (Resp, error) {
	var resp Resp
	if err := iface.Call(ctx, name, body, &resp); err != nil {
		var zero Resp
		return zero, err
	}
	return resp, nil
}
