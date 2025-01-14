package dbus

import (
	"context"
	"encoding/xml"
	"fmt"
	"maps"
)

type Object struct {
	p    Peer
	path ObjectPath
}

func (o Object) Conn() *Conn      { return o.p.Conn() }
func (o Object) Peer() Peer       { return o.p }
func (o Object) Path() ObjectPath { return o.path }

func (o Object) String() string {
	if o.path == "" {
		return fmt.Sprintf("%s:<no object>", o.Peer())
	}
	return fmt.Sprintf("%s:%s", o.Peer(), o.path)
}

func (o Object) Interface(name string) Interface {
	return Interface{
		o:    o,
		name: name,
	}
}

func (o Object) Introspect(ctx context.Context, opts ...CallOption) (*Description, error) {
	var resp string
	if err := o.Conn().call(ctx, o.p.name, o.path, "org.freedesktop.DBus.Introspectable", "Introspect", nil, &resp, opts...); err != nil {
		return nil, err
	}
	fmt.Println(resp)
	var ret Description
	if err := xml.Unmarshal([]byte(resp), &ret); err != nil {
		return nil, err
	}
	return &ret, nil
}

func (o Object) Interfaces(ctx context.Context, opts ...CallOption) ([]Interface, error) {
	var names []string
	if err := o.Interface("org.freedesktop.DBus").GetProperty(ctx, "Interfaces", &names, opts...); err != nil {
		return nil, err
	}
	ret := make([]Interface, 0, len(names))
	for _, n := range names {
		ret = append(ret, o.Interface(n))
	}
	return ret, nil
}

func (o Object) ManagedObjects(ctx context.Context, opts ...CallOption) (map[Object][]Interface, error) {
	// object path -> interface name -> map[property name]value
	var resp map[ObjectPath]map[string]map[string]Variant
	err := o.Conn().call(ctx, o.p.name, o.path, "org.freedesktop.DBus.ObjectManager", "GetManagedObjects", nil, &resp, opts...)
	if err != nil {
		return nil, err
	}
	ret := make(map[Object][]Interface, len(resp))
	for path, ifs := range resp {
		// TODO: validate that path is a subpath of the current object
		child := o.Peer().Object(path)
		ifaces := make([]Interface, 0, len(ifs))
		for ifname := range maps.Keys(ifs) {
			ifaces = append(ifaces, child.Interface(ifname))
		}
		ret[o.Peer().Object(path)] = ifaces
	}
	return ret, nil
}
