package dbus

import (
	"cmp"
	"encoding/xml"
	"fmt"
	"slices"
	"strings"
)

// ObjectDescription describes a DBus object's exported interfaces and
// child objects.
//
// Interface and child descriptions are provided by the DBus peer
// hosting the object, and may not accurately reflect the actual
// exposed API or object structure.
type ObjectDescription struct {
	// Interfaces maps an interface name to a description of its API.
	Interfaces map[string]*InterfaceDescription
	// Children is the relative paths to child objects under this
	// object. The relative paths may contain multiple path
	// components.
	Children []string
}

func (o *ObjectDescription) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	var raw struct {
		Interfaces []*InterfaceDescription `xml:"interface"`
		Children   []struct {
			Name string `xml:"name,attr"`
		} `xml:"node"`
	}
	if err := d.DecodeElement(&raw, &start); err != nil {
		return err
	}
	o.Interfaces = make(map[string]*InterfaceDescription, len(raw.Interfaces))
	for _, iface := range raw.Interfaces {
		o.Interfaces[iface.Name] = iface
	}
	o.Children = make([]string, 0, len(raw.Children))
	for _, v := range raw.Children {
		o.Children = append(o.Children, v.Name)
	}
	return nil
}

// InterfaceDescription describes a DBus interface.
//
// Interface descriptions are provided by the DBus peer offering the
// interface, and may not accurately reflect the actual exposed API.
type InterfaceDescription struct {
	Name       string                 `xml:"name,attr"`
	Methods    []*MethodDescription   `xml:"method"`
	Signals    []*SignalDescription   `xml:"signal"`
	Properties []*PropertyDescription `xml:"property"`
}

func (d InterfaceDescription) String() string {
	var ret strings.Builder
	fmt.Fprintf(&ret, "interface %s {\n", d.Name)

	methods := slices.SortedFunc(slices.Values(d.Methods), func(a, b *MethodDescription) int {
		return cmp.Compare(a.Name, b.Name)
	})
	for _, m := range methods {
		fmt.Fprintf(&ret, "  %s\n", m)
	}

	signals := slices.SortedFunc(slices.Values(d.Signals), func(a, b *SignalDescription) int {
		return cmp.Compare(a.Name, b.Name)
	})
	for _, s := range signals {
		fmt.Fprintf(&ret, "  %s\n", s)
	}

	props := slices.SortedFunc(slices.Values(d.Properties), func(a, b *PropertyDescription) int {
		return cmp.Compare(a.Name, b.Name)
	})
	for _, s := range props {
		fmt.Fprintf(&ret, "  %s\n", s)
	}
	ret.WriteString("}")
	return ret.String()
}

// MethodDescription describes a DBus method.
//
// Method descriptions are provided by the DBus peer offering the
// method, and may not accurately reflect the actual exposed API.
type MethodDescription struct {
	Name string
	In   []ArgumentDescription
	Out  []ArgumentDescription
	// Deprecated, if true, indicates that the method should be
	// avoided in new code.
	Deprecated bool
	// If true, NoReply indicates that the caller is expected to use
	// Interface.OneWay to invoke this method, not Interface.Call.
	NoReply bool
}

func (m MethodDescription) String() string {
	var ret strings.Builder
	ret.WriteString("func ")
	ret.WriteString(m.Name)
	ret.WriteByte('(')
	for i, arg := range m.In {
		if i > 0 {
			ret.WriteString(", ")
		}
		ret.WriteString(arg.String())
	}
	ret.WriteByte(')')

	if len(m.Out) > 0 {
		ret.WriteString(" (")
		for i, arg := range m.Out {
			if i > 0 {
				ret.WriteString(", ")
			}
			ret.WriteString(arg.String())
		}
		ret.WriteByte(')')
	}
	switch {
	case m.Deprecated && m.NoReply:
		ret.WriteString(" [deprecated,noreply]")
	case m.Deprecated:
		ret.WriteString(" [deprecated]")
	case m.NoReply:
		ret.WriteString(" [noreply]")
	}
	return ret.String()
}

func (m *MethodDescription) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	var raw struct {
		Name string `xml:"name,attr"`
		Args []struct {
			Name      string `xml:"name,attr"`
			Type      string `xml:"type,attr"`
			Direction string `xml:"direction,attr"`
		} `xml:"arg"`
		Meta []struct {
			Name  string `xml:"name,attr"`
			Value string `xml:"value,attr"`
		} `xml:"annotation"`
	}
	if err := d.DecodeElement(&raw, &start); err != nil {
		return err
	}
	m.Name = raw.Name
	m.In, m.Out = nil, nil
	m.Deprecated, m.NoReply = false, false
	for _, arg := range raw.Args {
		sig, err := ParseSignature(arg.Type)
		if err != nil {
			return fmt.Errorf("invalid signature %q for arg %s: %w", arg.Type, arg.Name, err)
		}
		ad := ArgumentDescription{
			Name: arg.Name,
			Type: sig,
		}
		if arg.Direction == "in" {
			m.In = append(m.In, ad)
		} else {
			m.Out = append(m.Out, ad)
		}
	}
	for _, attr := range raw.Meta {
		switch attr.Name {
		case "org.freedesktop.DBus.Deprecated":
			m.Deprecated = attr.Value == "true"
		case "org.freedesktop.DBus.Method.NoReply":
			m.NoReply = attr.Value == "true"
		}
	}

	return nil
}

// SignalDescription describes a DBus signal.
//
// Signal descriptions are provided by the DBus peer emitting the
// signal, and may not accurately reflect the received signal.
type SignalDescription struct {
	Name string
	Args []ArgumentDescription
	// Deprecated, if true, indicates that the signal should be
	// avoided in new code.
	Deprecated bool
}

func (s SignalDescription) String() string {
	var ret strings.Builder
	ret.WriteString("signal ")
	ret.WriteString(s.Name)
	ret.WriteByte('(')
	for i, arg := range s.Args {
		if i > 0 {
			ret.WriteString(", ")
		}
		ret.WriteString(arg.String())
	}
	ret.WriteByte(')')
	if s.Deprecated {
		ret.WriteString(" [deprecated]")
	}
	return ret.String()
}

func (s *SignalDescription) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	var raw struct {
		Name       string `xml:"name,attr"`
		Attributes []struct {
			Name string `xml:"name,attr"`
			Type string `xml:"type,attr"`
		} `xml:"arg"`
		Meta []struct {
			Name  string `xml:"name,attr"`
			Value string `xml:"value,attr"`
		} `xml:"annotation"`
	}
	if err := d.DecodeElement(&raw, &start); err != nil {
		return err
	}
	s.Name = raw.Name
	s.Args = nil
	s.Deprecated = false
	for _, attr := range raw.Attributes {
		sig, err := ParseSignature(attr.Type)
		if err != nil {
			return fmt.Errorf("invalid signature %q for signal arg %s: %w", attr.Type, attr.Name, err)
		}
		s.Args = append(s.Args, ArgumentDescription{
			Name: attr.Name,
			Type: sig,
		})
	}
	for _, attr := range raw.Meta {
		if attr.Name == "org.freedesktop.DBus.Deprecated" && attr.Value == "true" {
			s.Deprecated = true
		}
	}
	return nil
}

// PropertyDescription describes a DBus property.
//
// Property descriptions are provied by the DBus peer offering the
// property, and may not accurately reflect the actual property.
type PropertyDescription struct {
	Name string
	Type Signature

	// If true, Constant indicates that the property's value never
	// changes, and thus can safely be cached locally.
	Constant bool
	// Readable is whether the property value can be read using
	// Interface.GetProperty.
	Readable bool
	// Writable is whether the property value can be set using
	// Interface.SetProperty
	Writable bool

	// EmitsSignal is whether the property emits a PropertiesChanged
	// signal when updated.
	EmitsSignal bool
	// SignalIncludesValue is whether the PropertiesChanged signal
	// emitted when this property changes includes the new value. If
	// false, the signal merely reports that the property's value has
	// been invalidated, and the recipient must use
	// Interface.GetProperty to retrieve the updated value.
	SignalIncludesValue bool

	// Deprecated, if true, indicates that the property should be
	// avoided in new code.
	Deprecated bool
}

func (p PropertyDescription) String() string {
	var ret strings.Builder
	fmt.Fprintf(&ret, "property %s %s [", p.Name, p.Type.Type())

	switch {
	case p.Readable && !p.Writable && p.Constant:
		ret.WriteString("const")
	case p.Readable && p.Writable:
		ret.WriteString("readwrite")
	case p.Readable:
		ret.WriteString("readonly")
	case p.Writable:
		ret.WriteString("writeonly")
	}
	if p.Deprecated {
		ret.WriteString(",deprecated")
	}

	if p.EmitsSignal && p.SignalIncludesValue {
		ret.WriteString(",signals")
	} else if p.EmitsSignal {
		ret.WriteString(",invalidates")
	}
	ret.WriteByte(']')
	return ret.String()
}

func (p *PropertyDescription) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	var raw struct {
		Name   string `xml:"name,attr"`
		Type   string `xml:"type,attr"`
		Access string `xml:"access,attr"`
		Meta   []struct {
			Name  string `xml:"name,attr"`
			Value string `xml:"value,attr"`
		} `xml:"annotation"`
	}
	if err := d.DecodeElement(&raw, &start); err != nil {
		return err
	}
	p.Name = raw.Name
	sig, err := ParseSignature(raw.Type)
	if err != nil {
		return fmt.Errorf("invalid signature %q for property %s: %w", raw.Type, raw.Name, err)
	}
	p.Type = sig
	p.Constant, p.EmitsSignal, p.SignalIncludesValue = false, true, true
	switch raw.Access {
	case "read":
		p.Readable, p.Writable = true, false
	case "write":
		p.Readable, p.Writable = false, true
	case "readwrite":
		p.Readable, p.Writable = true, true
	default:
		return fmt.Errorf("unknown property access value %q", raw.Access)
	}
	for _, attr := range raw.Meta {
		switch attr.Name {
		case "org.freedesktop.DBus.Deprecated":
			p.Deprecated = attr.Value == "true"
		case "org.freedesktop.DBus.Property.EmitsChangedSignal":
			switch attr.Value {
			case "false":
				p.EmitsSignal = false
				p.SignalIncludesValue = false
			case "invalidates":
				p.SignalIncludesValue = false
			case "const":
				p.Constant = true
				p.EmitsSignal = false
				p.SignalIncludesValue = false
			}
		}
	}
	return nil
}

// ArgumentDescription describes a DBus method's input or output, or a
// signal's argument.
type ArgumentDescription struct {
	Name string // optional
	Type Signature
}

func (a ArgumentDescription) String() string {
	if a.Name != "" {
		// Older DBus interfaces used arg-name style naming, which
		// looks weird to people used to C and Go-style languages. The
		// modern recommendation is to use underscores, and since
		// argument names aren't load-bearing for correctness, fix
		// them up here for readability.
		n := strings.Replace(a.Name, "-", "_", -1)
		return fmt.Sprintf("%s %s", n, a.Type.Type())
	}
	return a.Type.Type().String()
}
