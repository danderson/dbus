package dbus

import (
	"fmt"
	"reflect"

	"github.com/danderson/dbus/fragments"
)

type Variant struct {
	Value any
}

var variantType = reflect.TypeFor[Variant]()

func (v Variant) MarshalDBus(st *fragments.Encoder) error {
	sig, err := SignatureOf(v.Value)
	if err != nil {
		return err
	}
	if err := st.Value(sig); err != nil {
		return err
	}
	if err := st.Value(v.Value); err != nil {
		return err
	}
	return nil
}

func (v *Variant) UnmarshalDBus(d *fragments.Decoder) error {
	var sig Signature
	if err := d.Value(&sig); err != nil {
		return fmt.Errorf("reading Variant signature: %w", err)
	}
	innerValue := sig.Value()
	if !innerValue.IsValid() {
		return fmt.Errorf("unsupported Variant type signature %q", sig)
	}
	inner := innerValue.Interface()
	if err := d.Value(inner); err != nil {
		return fmt.Errorf("reading Variant value (signature %q): %w", sig, err)
	}
	v.Value = innerValue.Elem().Interface()
	return nil
}

func (v Variant) AlignDBus() int           { return 1 }
func (v Variant) SignatureDBus() Signature { return "v" }
