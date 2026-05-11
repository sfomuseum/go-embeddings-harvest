package options

import (
	"testing"
)

func TestModelOptions(t *testing.T) {

	p := "apple/mobileclip_s1"
	o := NewModelOption(p)

	if o.Type() != ModelOptionType {
		t.Fatalf("Invalid type, %v", o.Type())
	}

	switch o.(type) {
	case *ModelOption:
		// pass
	default:
		t.Fatalf("Invalid type")
	}

	v := o.(*ModelOption).Model()

	if v != p {
		t.Fatalf("Invalid model, %s. Expected %s", v, p)
	}
}
