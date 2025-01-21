package dbus

import (
	"cmp"
	"encoding/xml"
	"fmt"
	"slices"
	"strings"
)

type ObjectDescription struct {
	Name       string                  `xml:"name,attr"`
	Interfaces []*InterfaceDescription `xml:"interface"`
	Children   []*ObjectDescription    `xml:"node"`
}

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
		fmt.Fprintf(&ret, "  func %s\n", m)
	}

	signals := slices.SortedFunc(slices.Values(d.Signals), func(a, b *SignalDescription) int {
		return cmp.Compare(a.Name, b.Name)
	})
	for _, s := range signals {
		fmt.Fprintf(&ret, "  signal %s\n", s)
	}

	props := slices.SortedFunc(slices.Values(d.Properties), func(a, b *PropertyDescription) int {
		return cmp.Compare(a.Name, b.Name)
	})
	for _, s := range props {
		fmt.Fprintf(&ret, "  var %s\n", s)
	}
	ret.WriteString("}")
	return ret.String()
}

type MethodDescription struct {
	Name       string
	In         []ArgumentDescription
	Out        []ArgumentDescription
	Deprecated bool
	NoReply    bool
}

func (m MethodDescription) String() string {
	var ins []string
	for _, arg := range m.In {
		ins = append(ins, arg.String())
	}

	out := ""
	if len(m.Out) > 0 {
		var outs []string
		for _, arg := range m.Out {
			outs = append(outs, arg.String())
		}
		out = fmt.Sprintf("(%s)", strings.Join(outs, ", "))
	}
	var anns []string
	if m.Deprecated {
		anns = append(anns, "deprecated")
	}
	if m.NoReply {
		anns = append(anns, "noreply")
	}
	ann := ""
	if len(anns) > 0 {
		ann = fmt.Sprintf(" [%s]", strings.Join(anns, ", "))
	}

	return fmt.Sprintf("%s(%s) %s%s", m.Name, strings.Join(ins, ", "), out, ann)
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

type SignalDescription struct {
	Name       string
	Args       []ArgumentDescription
	Deprecated bool
}

func (s SignalDescription) String() string {
	var args []string
	for _, arg := range s.Args {
		args = append(args, arg.String())
	}
	return fmt.Sprintf("%s(%s)", s.Name, strings.Join(args, ", "))
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

type PropertyDescription struct {
	Name string
	Type Signature

	Constant bool
	Readable bool
	Writable bool

	EmitsSignal         bool
	SignalIncludesValue bool

	Deprecated bool
}

func (p PropertyDescription) String() string {
	var as []string

	if p.Deprecated {
		as = append(as, "deprecated")
	}

	switch {
	case p.Readable && !p.Writable && p.Constant:
		as = append(as, "const")
	case p.Readable && p.Writable:
		as = append(as, "readwrite")
	case p.Readable:
		as = append(as, "readonly")
	case p.Writable:
		as = append(as, "writeonly")
	}

	switch {
	case p.Readable && !p.Writable && p.Constant:
		// nothing
	case p.Constant:
		as = append(as, "const")
	case p.EmitsSignal && p.SignalIncludesValue:
		as = append(as, "signals")
	case p.EmitsSignal:
		as = append(as, "invalidates")
	default:
		as = append(as, "no-signal")
	}

	return fmt.Sprintf("%s %s [%s]", p.Name, p.Type, strings.Join(as, ", "))
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

type ArgumentDescription struct {
	Name string
	Type Signature
}

func (a ArgumentDescription) String() string {
	if a.Name != "" {
		return fmt.Sprintf("%s %s", a.Name, a.Type)
	}
	return a.Type.String()
}
