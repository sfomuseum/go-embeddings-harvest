package options

type MaxResultsOption struct {
	Option
	d int32
}

func NewMaxResultsOption(d int32) Option {

	o := &MaxResultsOption{
		d: d,
	}

	return o
}

func (o *MaxResultsOption) Type() OptionType {
	return MaxResultsOptionType
}

func (o *MaxResultsOption) MaxResults() int32 {
	return o.d
}
