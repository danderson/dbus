package dbusgen

import (
	"bytes"
	"fmt"
	"go/format"
	"reflect"
	"strings"
	"unicode"

	"github.com/danderson/dbus"
)

type generator struct {
	out       bytes.Buffer
	ifaceName string
}

func Interface(iface *dbus.InterfaceDescription) (string, error) {
	g := generator{}
	g.ifaceName = publicIdentifier(iface.Name)
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

func (g *generator) Interface(iface *dbus.InterfaceDescription) error {
	g.f("type %s struct { iface dbus.Interface }\n", g.ifaceName)
	g.f("func New%s(obj dbus.Object) %s { return %s{obj.Interface(%q)} }\n\n", g.ifaceName, g.ifaceName, g.ifaceName, iface.Name)

	for _, m := range iface.Methods {
		if err := g.Method(m); err != nil {
			return err
		}
	}
	return nil
}

func (g *generator) Method(m *dbus.MethodDescription) error {
	mname := publicIdentifier(m.Name)
	ai := argsIn{mname, m.In}
	ao := argsOut{mname, m.Out}

	ai.writeStruct(g)
	ao.writeStruct(g)

	g.f("func (iface %s) %s(", g.ifaceName, mname)
	ai.writeArgs(g)
	g.s(") (")
	ao.writeArgs(g)
	g.s(") {\n")
	reqVar := ai.writeMkReq(g)
	respVar := ao.writeMkRet(g)
	g.f("err := iface.iface.Call(ctx, %q, %s, %s)\n", m.Name, reqVar, respVar)
	ao.writeRet(g)
	g.s("}\n\n")
	return nil
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
	if len(a.args) == 0 {
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
