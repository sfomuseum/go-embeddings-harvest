package options

import (
	"fmt"
)

type DimensionsOption struct {
	Option
	d int
}

func NewDimensionsOption(d int) Option {

	o := &DimensionsOption{
		d: d,
	}

	return o
}

func (o *DimensionsOption) Type() OptionType {
	return DimensionsOptionType
}

func (o *DimensionsOption) Dimensions() int {
	return o.d
}

func (o *DimensionsOption) String() string {
	return fmt.Sprintf("option:dimensions=%d", o.d)
}
