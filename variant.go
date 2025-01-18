package dbus

import (
	"context"
	"fmt"
	"reflect"

	"github.com/danderson/dbus/fragments"
)

// A Variant is a value of any valid DBus type.
//
// Variant corresponds to the DBus "variant" basic type, which is used
// in APIs where a value's type is only known at runtime.
type Variant struct {
	Value any
}

var variantType = reflect.TypeFor[Variant]()

func (v Variant) MarshalDBus(ctx context.Context, e *fragments.Encoder) error {
	sig, err := SignatureOf(v.Value)
	if err != nil {
		return err
	}
	if err := e.Value(ctx, sig); err != nil {
		return err
	}
	if err := e.Value(ctx, v.Value); err != nil {
		return err
	}
	return nil
}

func (v *Variant) UnmarshalDBus(ctx context.Context, d *fragments.Decoder) error {
	var sig Signature
	if err := d.Value(ctx, &sig); err != nil {
		return fmt.Errorf("reading Variant signature: %w", err)
	}
	innerType := sig.Type()
	if innerType == nil {
		return fmt.Errorf("unsupported Variant type signature %q", sig)
	}
	inner := reflect.New(innerType)
	if err := d.Value(ctx, inner.Interface()); err != nil {
		return fmt.Errorf("reading Variant value (signature %q): %w", sig, err)
	}
	v.Value = inner.Elem().Interface()
	return nil
}

func (v Variant) IsDBusStruct() bool { return false }

var variantSignature = mkSignature(variantType, "v")

func (v Variant) SignatureDBus() Signature { return variantSignature }
