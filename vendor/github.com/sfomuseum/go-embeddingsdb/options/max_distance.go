package options

type MaxDistanceOption struct {
	Option
	d float32
}

func NewMaxDistanceOption(d float32) Option {

	o := &MaxDistanceOption{
		d: d,
	}

	return o
}

func (o *MaxDistanceOption) Type() OptionType {
	return MaxDistanceOptionType
}

func (o *MaxDistanceOption) MaxDistance() float32 {
	return o.d
}
