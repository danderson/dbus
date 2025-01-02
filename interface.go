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

func (f Interface) Call(ctx context.Context, method string, body any, response any) error {
	req := Request{
		Destination: f.Peer().name,
		Path:        f.Object().path,
		Interface:   f.name,
		Method:      method,
		Body:        body,
	}
	return f.Conn().Call(ctx, req, response)
}

func (f Interface) GetProp(ctx context.Context, name string) (any, error) {
	req := Request{
		Destination: f.Peer().name,
		Path:        f.Object().path,
		Interface:   "org.freedesktop.DBus.Properties",
		Method:      "Get",
		Body: struct {
			InterfaceName string
			PropertyName  string
		}{f.name, name},
	}
	var resp Variant
	if err := f.Conn().Call(ctx, req, &resp); err != nil {
		return "", err
	}
	return resp.Value, nil
}

func (f Interface) SetProp(ctx context.Context, name string, value any) error {
	req := Request{
		Destination: f.Peer().name,
		Path:        f.Object().path,
		Interface:   "org.freedesktop.DBus.Properties",
		Method:      "Get",
		Body: struct {
			InterfaceName string
			PropertyName  string
			Value         Variant
		}{f.name, name, Variant{value}},
	}
	if err := f.Conn().Call(ctx, req, nil); err != nil {
		return err
	}
	return nil
}

func (f Interface) GetAll(ctx context.Context) (map[string]Variant, error) {
	req := Request{
		Destination: f.Peer().name,
		Path:        f.Object().path,
		Interface:   "org.freedesktop.DBus.Properties",
		Method:      "Get",
		Body:        f.name,
	}
	var resp map[string]Variant
	if err := f.Conn().Call(ctx, req, &resp); err != nil {
		return nil, err
	}
	return resp, nil
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
