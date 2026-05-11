package options

import (
	"fmt"
)

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

func (o *MaxResultsOption) String() string {
	return fmt.Sprintf("option:max_results=%d", o.d)
}
