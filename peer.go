package dbus

import "context"

type Peer struct {
	c    *Conn
	name string
}

func (p Peer) Ping(ctx context.Context, opts ...CallOption) error {
	return p.Conn().call(ctx, p.name, "/", "org.freedesktop.DBus.Peer", "Ping", nil, nil, opts...)
}

func (p Peer) Conn() *Conn { return p.c }

func (p Peer) Object(path ObjectPath) Object {
	return Object{
		p:    p,
		path: path,
	}
}