package dbus

import "context"

type Object struct {
	p    Peer
	path ObjectPath
}

func (o Object) Conn() *Conn { return o.p.Conn() }
func (o Object) Peer() Peer  { return o.p }

func (o Object) Interface(name string) Interface {
	return Interface{
		o:    o,
		name: name,
	}
}

func (o Object) Introspect(ctx context.Context) (string, error) {
	req := Request{
		Destination: o.p.name,
		Path:        o.path,
		Interface:   "org.freedesktop.DBus.Introspectable",
		Method:      "Introspect",
	}
	var resp string
	if err := o.p.c.Call(ctx, req, &resp); err != nil {
		return "", err
	}
	return resp, nil
}

func (o Object) Interfaces(ctx context.Context) ([]Interface, error) {
	names, err := GetProperty[[]string](ctx, o.Interface("org.freedesktop.DBus"), "Interfaces")
	if err != nil {
		return nil, err
	}
	ret := make([]Interface, 0, len(names))
	for _, n := range names {
		ret = append(ret, o.Interface(n))
	}
	return ret, nil
}
