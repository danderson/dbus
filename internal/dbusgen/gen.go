package dbusgen

import (
	"bytes"
	"cmp"
	"errors"
	"fmt"
	"go/format"
	"reflect"
	"slices"
	"strings"
	"unicode"

	"github.com/danderson/dbus"
)

type generator struct {
	out   bytes.Buffer
	iface *dbus.InterfaceDescription
	inits bytes.Buffer
}

func Interface(iface *dbus.InterfaceDescription) (string, error) {
	if iface == nil {
		return "", errors.New("no interface provided")
	}
	g := generator{iface: iface}
	if err := g.Interface(iface); err != nil {
		return "", err
	}

	ret, err := format.Source(g.out.Bytes())
	if err != nil {
		return g.out.String(), err
	}

	return string(ret), nil
}

func (g *generator) s(s string) {
	g.out.WriteString(s)
}

func (g *generator) f(msg string, args ...any) {
	fmt.Fprintf(&g.out, msg, args...)
}

func (g *generator) init(msg string, args ...any) {
	fmt.Fprintf(&g.inits, msg, args...)
}

func (g *generator) Interface(iface *dbus.InterfaceDescription) error {
	g.f(`
type %[1]s struct { iface dbus.Interface }

// New returns an interface to TODO
func new(conn *dbus.Conn) %[1]s {
  obj := conn.Peer("TODO").Object("TODO")
  return Interface(obj)
}

// Interface returns a %[1]s on the given object.
func Interface(obj dbus.Object) %[1]s {
  return %[1]s{
    iface: obj.Interface(%[2]q),
  }
}
`, publicIdentifier(g.iface.Name), iface.Name)

	slices.SortFunc(iface.Methods, func(a, b *dbus.MethodDescription) int {
		return cmp.Compare(a.Name, b.Name)
	})
	slices.SortFunc(iface.Signals, func(a, b *dbus.SignalDescription) int {
		return cmp.Compare(a.Name, b.Name)
	})
	slices.SortFunc(iface.Properties, func(a, b *dbus.PropertyDescription) int {
		return cmp.Compare(a.Name, b.Name)
	})

	for _, m := range iface.Methods {
		g.Method(m)
	}
	for _, p := range iface.Properties {
		g.Property(p)
	}
	for _, s := range iface.Signals {
		g.Signal(s)
	}
	if inits := g.inits.String(); len(inits) > 0 {
		g.f(`func init() {
%s
}`, strings.TrimSpace(inits))
	}
	return nil
}

func (g *generator) Method(m *dbus.MethodDescription) {
	mname := publicIdentifier(m.Name)
	ai := argsIn{mname, m.In}
	ao := argsOut{mname, m.Out}

	ai.writeStruct(g)
	ao.writeStruct(g)

	g.f("func (iface %s) %s(", publicIdentifier(g.iface.Name), mname)
	ai.writeArgs(g)
	g.s(") (")
	ao.writeArgs(g)
	g.s(") {\n")
	reqVar := ai.writeMkReq(g)
	respVar := ao.writeMkRet(g)
	if ao.noRet() {
		g.f("err := iface.iface.Call(ctx, %q, %s, %s)\n", m.Name, reqVar, respVar)
	} else {
		g.f("err = iface.iface.Call(ctx, %q, %s, %s)\n", m.Name, reqVar, respVar)
	}
	ao.writeRet(g)
	g.s("}\n\n")
}

func (g *generator) Signal(s *dbus.SignalDescription) {
	sname := publicIdentifier(s.Name)
	g.f(`
// %[1]s implements the signal %[2]s.%[3]s.
type %[1]s %[4]s

`, sname, g.iface.Name, s.Name, asStruct(s.Args).Type())
	g.init("dbus.RegisterSignalType[%s](%q, %q)\n", publicIdentifier(s.Name), g.iface.Name, s.Name)
}

func (g *generator) Property(prop *dbus.PropertyDescription) {
	if prop.Constant || prop.Readable {
		g.f(`
// %[2]s returns the value of the property %[4]q.
func (iface %[1]s) %[2]s(ctx context.Context) (%[3]s, error) {
  var ret %[3]s
  err := iface.iface.GetProperty(ctx, %[4]q, &ret)
  return ret, err
}

`, publicIdentifier(g.iface.Name), publicIdentifier(prop.Name), prop.Type.Type(), prop.Name)
	}

	if prop.Writable {
		g.f(`
// %[2]s sets the value of property %[4]q to val.
func (iface %[1]s) Set%[2]s(ctx context.Context, val %[3]s) error {
  return iface.iface.SetProperty(ctx, %[4]q, val)
}

`, publicIdentifier(g.iface.Name), publicIdentifier(prop.Name), prop.Type.Type(), prop.Name)
	}

	if !prop.EmitsSignal {
		return
	}

	if prop.SignalIncludesValue {
		g.f(`
// %[1]sChanged signals that the value of property %[3]q has changed.
type %[1]sChanged %[2]s
`, publicIdentifier(prop.Name), prop.Type.Type(), prop.Name)
	} else {
		g.f(`
// %[1]sChanged signals that the value of property %[2]q has changed.
type %[1]sChanged struct{}
`, publicIdentifier(prop.Name), prop.Name)
	}
	g.init("dbus.RegisterPropertyChangeType[%sChanged](%q, %q)\n", publicIdentifier(prop.Name), g.iface.Name, prop.Name)
}

func argName(n int, arg dbus.ArgumentDescription) string {
	name := arg.Name
	if name == "" {
		name = fmt.Sprintf("arg%d", n)
	}
	name = identifier(name)
	switch name {
	case "type":
		name = "typ"
	}
	return name
}

func arg(n int, arg dbus.ArgumentDescription) string {
	return fmt.Sprintf("%s %s", argName(n, arg), arg.Type.Type())
}

func identifier(s string) string {
	if i := strings.LastIndexByte(s, '.'); i >= 0 {
		s = s[i+1:]
	}
	fs := strings.Split(s, "_")
	for i := range fs {
		if i == 0 {
			fst := true
			fs[i] = strings.Map(func(r rune) rune {
				if fst {
					fst = false
					return unicode.ToLower(r)
				}
				return r
			}, fs[i])
		} else {
			switch fs[i] {
			case "id":
				fs[i] = "ID"
			case "fd":
				fs[i] = "FD"
			default:
				fs[i] = strings.Title(fs[i])
			}
		}
	}
	return strings.Join(fs, "")
}

func publicIdentifier(s string) string {
	return strings.Title(identifier(s))
}

func asStruct(args []dbus.ArgumentDescription) dbus.Signature {
	fs := make([]reflect.StructField, len(args))
	for i, a := range args {
		fs[i] = reflect.StructField{
			Name: publicIdentifier(argName(i, a)),
			Type: a.Type.Type(),
		}
	}
	st := reflect.StructOf(fs)
	ret, err := dbus.SignatureOf(reflect.New(st).Elem().Interface())
	if err != nil {
		panic(err)
	}
	return ret
}

type argsIn struct {
	methodName string
	args       []dbus.ArgumentDescription
}

func (a argsIn) useStruct() bool {
	return len(a.args) > 3
}

func (a argsIn) writeStruct(g *generator) {
	if !a.useStruct() {
		return
	}
	g.f("type %sRequest %s\n", a.methodName, asStruct(a.args).Type())
}

func (a argsIn) writeArgs(g *generator) {
	if a.useStruct() {
		g.f("ctx context.Context, req %sRequest", a.methodName)
	} else {
		g.s("ctx context.Context")
		for i, a := range a.args {
			g.f(", %s %s", argName(i, a), a.Type.Type())
		}
	}
}

func (a argsIn) writeMkReq(g *generator) (varName string) {
	if len(a.args) == 0 {
		return "nil"
	}
	if len(a.args) == 1 {
		return argName(0, a.args[0])
	}
	if a.useStruct() {
		return "req"
	}

	st := asStruct(a.args)
	g.f("req := %s{\n", st.Type())
	for i, a := range a.args {
		g.f("%s: %s,\n", publicIdentifier(argName(i, a)), argName(i, a))
	}
	g.s("}\n")
	return "req"
}

type argsOut struct {
	methodName string
	args       []dbus.ArgumentDescription
}

func (a argsOut) noRet() bool {
	return len(a.args) == 0
}

func (a argsOut) useStruct() bool {
	return len(a.args) > 2
}

func (a argsOut) useSliceStruct() bool {
	if len(a.args) != 1 {
		return false
	}
	t := a.args[0].Type.Type()
	if t.Kind() == reflect.Slice && t.Elem().Kind() == reflect.Struct {
		return true
	}
	return false
}

func (a argsOut) writeStruct(g *generator) {
	if a.useStruct() {
		g.f("type %sResponse %s\n", a.methodName, asStruct(a.args).Type())
	} else if a.useStruct() {
		g.f("type %sVal %s\n", a.methodName, a.args[0].Type.Type().Elem())
	}
}

func (a argsOut) writeArgs(g *generator) {
	if a.noRet() {
		g.f("error")
	} else if a.useStruct() {
		g.f("resp %sResponse, err error", a.methodName)
	} else if a.useSliceStruct() {
		g.f("resp []%sVal, err error", a.methodName)
	} else {
		for i, a := range a.args {
			if i > 0 {
				g.s(",")
			}
			g.f("%s %s", argName(i, a), a.Type.Type())
		}
		g.s(", err error")
	}
}

func (a argsOut) writeMkRet(g *generator) (varName string) {
	if len(a.args) == 0 {
		return "nil"
	}
	if len(a.args) == 1 {
		return "&" + argName(0, a.args[0])
	}
	if a.useStruct() {
		g.f("var resp %sResponse\n", a.methodName)
		return "&resp"
	}
	if a.useSliceStruct() {
		g.f("var resp []%sVal\n", a.methodName)
		return "&resp"
	}
	g.f("var resp %s\n", asStruct(a.args).Type())
	return "&resp"
}

func (a argsOut) writeRet(g *generator) {
	if len(a.args) == 0 {
		g.s("return err\n")
	} else if len(a.args) == 1 {
		g.f("return %s, err", argName(0, a.args[0]))
	} else if a.useStruct() || a.useSliceStruct() {
		g.s("return resp, err\n")
	} else {
		g.s("return ")
		for i, a := range a.args {
			if i > 0 {
				g.s(",")
			}
			g.f("resp.%s", publicIdentifier(argName(i, a)))
		}
		g.s(", err\n")
	}
}
