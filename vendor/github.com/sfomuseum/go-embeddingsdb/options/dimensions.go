package options

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
