package dbus

import (
	"reflect"
	"testing"
)

type TestSignal struct {
	A string
	B ObjectPath
	C string
	D int16
}

type TestSignal2 struct {
	A string
	B int16
}

type TestProp struct {
	A string
	B ObjectPath
	C string
	D int16
}

type TestProp2 uint16

func init() {
	RegisterSignalType[TestSignal]("org.test", "Signal")
	RegisterSignalType[TestSignal2]("org.test", "Signal2")
	RegisterPropertyChangeType[TestProp]("org.test", "Prop")
	RegisterPropertyChangeType[TestProp2]("org.test", "Prop2")
}

func TestMatch(t *testing.T) {
	var conn *Conn

	type sigMatch struct {
		hdr  header
		body any

		want bool
	}
	type propMatch struct {
		hdr  header
		prop interfaceMember
		body any

		want bool
	}
	type testCase struct {
		name         string
		m            *Match
		filter       string
		matchSignals []sigMatch
		matchProps   []propMatch
	}

	hdr := func(sender, path, iface, name string) header {
		return header{
			Sender:    sender,
			Path:      ObjectPath(path),
			Interface: iface,
			Member:    name,
		}
	}
	sig := func(want bool, sender, path, iface, name string, sig any) sigMatch {
		return sigMatch{
			hdr:  hdr(sender, path, iface, name),
			body: sig,
			want: want,
		}
	}
	prop := func(want bool, sender, path, iface, name string, prop any) propMatch {
		return propMatch{
			hdr:  hdr(sender, path, "org.freedesktop.DBus.Properties", "PropertiesChanged"),
			prop: interfaceMember{iface, name},
			body: prop,
			want: want,
		}
	}

	tests := []testCase{
		{
			name:   "all signals",
			m:      MatchAllSignals(),
			filter: `type='signal'`,
			matchSignals: []sigMatch{
				sig(true, "test", "/test", "org.test", "Signal", &TestSignal{}),
				sig(true, "test2", "/test2", "org.test2", "Signal2", &TestSignal2{}),
			},
			matchProps: []propMatch{
				prop(false, "test", "/test", "org.test", "Prop", &TestProp{}),
				prop(false, "test2", "/test2", "org.test2", "Prop2", TestProp2(0)),
			},
		},

		{
			name:   "signal",
			m:      MatchNotification[TestSignal](),
			filter: `type='signal',interface='org.test',member='Signal'`,
			matchSignals: []sigMatch{
				sig(true, "test", "/test", "org.test", "Signal", &TestSignal{}),
				sig(false, "test2", "/test2", "org.test2", "Signal2", &TestSignal2{}),
			},
			matchProps: []propMatch{
				prop(false, "test", "/test", "org.test", "Prop", &TestProp{}),
				prop(false, "test2", "/test2", "org.test2", "Prop2", TestProp2(0)),
			},
		},

		{
			name:   "signal sender",
			m:      MatchNotification[TestSignal]().Peer(conn.Peer("test")),
			filter: `type='signal',sender='test',interface='org.test',member='Signal'`,
			matchSignals: []sigMatch{
				sig(true, "test", "/test", "org.test", "Signal", &TestSignal{}),
				sig(true, "test", "/test2", "org.test", "Signal", &TestSignal{}),
				sig(false, "test2", "/test", "org.test", "Signal", &TestSignal{}),
				sig(false, "test2", "/test2", "org.test2", "Signal2", &TestSignal2{}),
			},
			matchProps: []propMatch{
				prop(false, "test", "/test", "org.test", "Prop", &TestProp{}),
				prop(false, "test2", "/test2", "org.test2", "Prop2", TestProp2(0)),
			},
		},

		{
			name:   "signal object",
			m:      MatchNotification[TestSignal]().Object("/test"),
			filter: `type='signal',path='/test',interface='org.test',member='Signal'`,
			matchSignals: []sigMatch{
				sig(true, "test", "/test", "org.test", "Signal", &TestSignal{}),
				sig(false, "test", "/test2", "org.test", "Signal", &TestSignal{}),
				sig(true, "test2", "/test", "org.test", "Signal", &TestSignal{}),
				sig(false, "test2", "/test2", "org.test2", "Signal", &TestSignal{}),
			},
			matchProps: []propMatch{
				prop(false, "test", "/test", "org.test", "Prop", &TestProp{}),
				prop(false, "test2", "/test2", "org.test2", "Prop2", TestProp2(0)),
			},
		},

		{
			name:   "signal object prefix",
			m:      MatchNotification[TestSignal]().ObjectPrefix("/test"),
			filter: `type='signal',path_namespace='/test',interface='org.test',member='Signal'`,
			matchSignals: []sigMatch{
				sig(true, "test", "/test", "org.test", "Signal", &TestSignal{}),
				sig(true, "test", "/test/foo", "org.test", "Signal", &TestSignal{}),
				sig(true, "test", "/test/bar", "org.test", "Signal", &TestSignal{}),
				sig(false, "test", "/testf", "org.test", "Signal", &TestSignal{}),
				sig(false, "test", "/qux", "org.test", "Signal", &TestSignal{}),
				sig(true, "test2", "/test/foo", "org.test", "Signal", &TestSignal{}),
			},
			matchProps: []propMatch{
				prop(false, "test", "/test", "org.test", "Prop", &TestProp{}),
				prop(false, "test2", "/test2", "org.test2", "Prop2", TestProp2(0)),
			},
		},

		{
			name:   "signal object arg",
			m:      MatchNotification[TestSignal]().ArgStr(0, "foo").ArgStr(2, "bar"),
			filter: `type='signal',interface='org.test',member='Signal',arg0='foo',arg2='bar'`,
			matchSignals: []sigMatch{
				sig(true, "test", "/test", "org.test", "Signal", &TestSignal{
					A: "foo",
					B: "/unused",
					C: "bar",
					D: 42,
				}),
				sig(true, "test", "/test", "org.test", "Signal", &TestSignal{
					A: "foo",
					C: "bar",
				}),
				sig(false, "test", "/test", "org.test", "Signal", &TestSignal{
					A: "foo",
					C: "zot",
				}),
				sig(false, "test", "/test", "org.test", "Signal", &TestSignal{
					A: "no",
					C: "bar",
				}),
				sig(false, "test", "/test", "org.test", "Signal", &TestSignal{}),
			},
			matchProps: []propMatch{
				prop(false, "test", "/test", "org.test", "Prop", &TestProp{}),
				prop(false, "test2", "/test2", "org.test2", "Prop2", TestProp2(0)),
			},
		},

		{
			name:   "signal object arg prefix",
			m:      MatchNotification[TestSignal]().ArgPathPrefix(0, "/foo").ArgPathPrefix(1, "/bar"),
			filter: `type='signal',interface='org.test',member='Signal',arg0path='/foo',arg1path='/bar'`,
			matchSignals: []sigMatch{
				sig(true, "test", "/test", "org.test", "Signal", &TestSignal{
					A: "/foo",
					B: "/bar",
					C: "unused",
					D: 42,
				}),
				sig(true, "test", "/test", "org.test", "Signal", &TestSignal{
					A: "/foo",
					B: "/bar",
				}),
				sig(true, "test", "/test", "org.test", "Signal", &TestSignal{
					A: "/foo/bar",
					B: "/bar",
				}),
				sig(true, "test", "/test", "org.test", "Signal", &TestSignal{
					A: "/foo",
					B: "/bar/qux",
				}),
				sig(true, "test", "/test", "org.test", "Signal", &TestSignal{
					A: "/foo/bar",
					B: "/bar/qux",
				}),
				sig(false, "test", "/test", "org.test", "Signal", &TestSignal{
					A: "/foo",
					B: "/zot",
				}),
				sig(false, "test", "/test", "org.test", "Signal", &TestSignal{
					A: "no",
					B: "/bar",
				}),
				sig(false, "test", "/test", "org.test", "Signal", &TestSignal{}),
			},
			matchProps: []propMatch{
				prop(false, "test", "/test", "org.test", "Prop", &TestProp{}),
				prop(false, "test2", "/test2", "org.test2", "Prop2", TestProp2(0)),
			},
		},

		{
			name:   "signal object arg 0 namespace",
			m:      MatchNotification[TestSignal]().Arg0Namespace("foo.bar"),
			filter: `type='signal',interface='org.test',member='Signal',arg0namespace='foo.bar'`,
			matchSignals: []sigMatch{
				sig(true, "test", "/test", "org.test", "Signal", &TestSignal{
					A: "foo.bar",
					B: "/bar",
					C: "unused",
					D: 42,
				}),
				sig(true, "test", "/test", "org.test", "Signal", &TestSignal{
					A: "foo.bar",
				}),
				sig(true, "test", "/test", "org.test", "Signal", &TestSignal{
					A: "foo.bar.baz",
				}),
				sig(true, "test", "/test", "org.test", "Signal", &TestSignal{
					A: "foo.bar.qux",
				}),
				sig(false, "test", "/test", "org.test", "Signal", &TestSignal{
					A: "foo",
				}),
				sig(false, "test", "/test", "org.test", "Signal", &TestSignal{
					A: "foo.qux",
				}),
				sig(false, "test", "/test", "org.test", "Signal", &TestSignal{
					A: "zot.qux",
				}),
				sig(false, "test", "/test", "org.test", "Signal", &TestSignal{
					A: "foo.barbaz",
				}),
				sig(false, "test", "/test", "org.test", "Signal", &TestSignal{}),
			},
			matchProps: []propMatch{
				prop(false, "test", "/test", "org.test", "Prop", &TestProp{}),
				prop(false, "test2", "/test2", "org.test2", "Prop2", TestProp2(0)),
			},
		},

		{
			name:   "property",
			m:      MatchNotification[TestProp](),
			filter: `type='signal',interface='org.freedesktop.DBus.Properties',member='PropertiesChanged',arg0='org.test'`,
			matchProps: []propMatch{
				prop(true, "test", "/test", "org.test", "Prop", &TestProp{}),
				prop(true, "test2", "/test", "org.test", "Prop", &TestProp{}),
				prop(true, "test", "/test2", "org.test", "Prop", &TestProp{}),
				prop(false, "test", "/test", "org.test2", "Prop2", TestProp2(0)),
				prop(false, "test2", "/test2", "org.test2", "Prop2", TestProp2(0)),
			},
			matchSignals: []sigMatch{
				sig(false, "test", "/test", "org.test", "Signal", &TestSignal{}),
				sig(false, "test2", "/test2", "org.test2", "Signal2", &TestSignal2{}),
			},
		},

		{
			name:   "property sender",
			m:      MatchNotification[TestProp]().Peer(conn.Peer("test")),
			filter: `type='signal',sender='test',interface='org.freedesktop.DBus.Properties',member='PropertiesChanged',arg0='org.test'`,
			matchProps: []propMatch{
				prop(true, "test", "/test", "org.test", "Prop", &TestProp{}),
				prop(false, "test2", "/test", "org.test", "Prop", &TestProp{}),
				prop(true, "test", "/test2", "org.test", "Prop", &TestProp{}),
				prop(false, "test", "/test", "org.test2", "Prop2", TestProp2(0)),
				prop(false, "test2", "/test2", "org.test2", "Prop2", TestProp2(0)),
			},
			matchSignals: []sigMatch{
				sig(false, "test", "/test", "org.test", "Signal", &TestSignal{}),
				sig(false, "test2", "/test2", "org.test2", "Signal2", &TestSignal2{}),
			},
		},

		{
			name:   "property object",
			m:      MatchNotification[TestProp]().Object("/test"),
			filter: `type='signal',path='/test',interface='org.freedesktop.DBus.Properties',member='PropertiesChanged',arg0='org.test'`,
			matchProps: []propMatch{
				prop(true, "test", "/test", "org.test", "Prop", &TestProp{}),
				prop(true, "test2", "/test", "org.test", "Prop", &TestProp{}),
				prop(false, "test", "/test2", "org.test", "Prop", &TestProp{}),
				prop(false, "test", "/test", "org.test2", "Prop2", TestProp2(0)),
				prop(false, "test2", "/test2", "org.test2", "Prop2", TestProp2(0)),
			},
			matchSignals: []sigMatch{
				sig(false, "test", "/test", "org.test", "Signal", &TestSignal{}),
				sig(false, "test2", "/test2", "org.test2", "Signal2", &TestSignal2{}),
			},
		},

		{
			name:   "property object prefix",
			m:      MatchNotification[TestProp]().ObjectPrefix("/test"),
			filter: `type='signal',path_namespace='/test',interface='org.freedesktop.DBus.Properties',member='PropertiesChanged',arg0='org.test'`,
			matchProps: []propMatch{
				prop(true, "test", "/test", "org.test", "Prop", &TestProp{}),
				prop(true, "test2", "/test", "org.test", "Prop", &TestProp{}),
				prop(true, "test", "/test/foo", "org.test", "Prop", &TestProp{}),
				prop(true, "test", "/test/bar", "org.test", "Prop", &TestProp{}),
				prop(true, "test2", "/test/bar", "org.test", "Prop", &TestProp{}),
				prop(false, "test", "/test2", "org.test", "Prop", &TestProp{}),
				prop(false, "test", "/test2/bar", "org.test", "Prop", &TestProp{}),
				prop(false, "test", "/test", "org.test2", "Prop2", TestProp2(0)),
				prop(false, "test2", "/test2", "org.test2", "Prop2", TestProp2(0)),
			},
			matchSignals: []sigMatch{
				sig(false, "test", "/test", "org.test", "Signal", &TestSignal{}),
				sig(false, "test2", "/test2", "org.test2", "Signal2", &TestSignal2{}),
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got, want := tc.m.filterString(), tc.filter; got != want {
				t.Errorf("wrong filter string\n  got: %s\n want: %s", got, want)
			}
			for _, tm := range tc.matchSignals {
				if got := tc.m.matchesSignal(&tm.hdr, reflect.ValueOf(tm.body)); got != tm.want {
					t.Errorf("wrong match on sender=%q,path=%q,interface=%q,signal=%q,body=%#v: got %v, want %v", tm.hdr.Sender, tm.hdr.Path, tm.hdr.Interface, tm.hdr.Member, tm.body, got, tm.want)
				}
			}
			for _, tm := range tc.matchProps {
				if got := tc.m.matchesProperty(&tm.hdr, tm.prop, reflect.ValueOf(tm.body)); got != tm.want {
					t.Errorf("wrong match on sender=%q,path=%q,interface=%q,prop=%q,body=%#v: got %v, want %v", tm.hdr.Sender, tm.hdr.Path, tm.prop.Interface, tm.prop.Member, tm.body, got, tm.want)
				}
			}
		})
	}
}
