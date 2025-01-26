package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"iter"
	"maps"
	"os"
	"regexp"
	"slices"
	"strings"

	"github.com/creachadair/mds/heapq"
	"github.com/danderson/dbus"
)

type indenter struct {
	prefix     string
	indentNext bool
}

func (i *indenter) v(v any) {
	fmt.Fprintf(i, "%v\n", v)
}

func (i *indenter) s(msg string) {
	io.WriteString(i, msg+"\n")
}

func (i *indenter) f(msg string, args ...any) {
	fmt.Fprintf(i, msg+"\n", args...)
}

func (i *indenter) Write(bs []byte) (int, error) {
	ret := 0
	for len(bs) > 0 {
		if i.indentNext {
			i.indentNext = false
			_, err := io.WriteString(os.Stdout, i.prefix)
			if err != nil {
				return ret, err
			}
		}

		var wr []byte
		idx := bytes.IndexByte(bs, '\n')
		if idx >= 0 {
			i.indentNext = true
			wr, bs = bs[:idx+1], bs[idx+1:]
		} else {
			bs = nil
		}

		n, err := os.Stdout.Write(wr)
		ret += n
		if err != nil {
			return ret, err
		}
	}
	return ret, nil
}

func (i *indenter) indent(n int) {
	i.prefix = strings.Repeat("  ", n)
}

func listPeers(ctx context.Context, conn *dbus.Conn, peerFilter string) iter.Seq2[dbus.Peer, error] {
	if peerFilter == "" {
		// Unique bus connections fail to handle introspection
		// gracefully more often than not.
		peerFilter = `^[^:].*`
	}
	return func(yield func(dbus.Peer, error) bool) {
		f, err := regexp.Compile(peerFilter)
		if err != nil {
			yield(dbus.Peer{}, err)
			return
		}
		peers, err := conn.Peers(ctx)
		if err != nil {
			yield(dbus.Peer{}, err)
			return
		}
		for _, p := range peers {
			if !f.MatchString(p.Name()) {
				continue
			}
			if !yield(p, nil) {
				return
			}
		}
	}
}

type objectInterface struct {
	dbus.Interface
	Description *dbus.InterfaceDescription
}

func listInterfaces(ctx context.Context, peer dbus.Peer, objectFilter, interfaceFilter string) iter.Seq2[objectInterface, error] {
	return func(yield func(objectInterface, error) bool) {
		om, err := regexp.Compile(objectFilter)
		if err != nil {
			yield(objectInterface{}, err)
			return
		}
		im, err := regexp.Compile(interfaceFilter)
		if err != nil {
			yield(objectInterface{}, err)
			return
		}

		objs := heapq.New(dbus.Object.Compare)
		objs.Add(peer.Object("/"))
		for !objs.IsEmpty() {
			obj, _ := objs.Pop()
			desc, err := obj.Introspect(ctx)
			if err != nil {
				if !yield(objectInterface{}, err) {
					return
				}
				continue
			}
			for _, child := range desc.Children {
				objs.Add(obj.Child(child))
			}
			if !om.MatchString(string(obj.Path())) {
				continue
			}
			ks := slices.Sorted(maps.Keys(desc.Interfaces))
			for _, k := range ks {
				if !im.MatchString(k) {
					continue
				}
				iface := obj.Interface(k)
				if !yield(objectInterface{iface, desc.Interfaces[k]}, nil) {
					return
				}
			}
		}
	}
}

func growTo(s []string, n int) []string {
	for len(s) < n {
		s = append(s, "")
	}
	return s
}
