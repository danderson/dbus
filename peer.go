package dbus

import "context"

type Peer struct {
	c    *Conn
	name string
}

func (p Peer) Ping(ctx context.Context) error {
	req := Request{
		Destination: p.name,
		Path:        "/",
		Interface:   "org.freedesktop.DBus.Peer",
		Method:      "Ping",
	}
	if err := p.c.Call(ctx, req, nil); err != nil {
		return err
	}
	return nil
}

func (p Peer) Conn() *Conn { return p.c }

func (p Peer) Object(path ObjectPath) Object {
	return Object{
		p:    p,
		path: path,
	}
}
