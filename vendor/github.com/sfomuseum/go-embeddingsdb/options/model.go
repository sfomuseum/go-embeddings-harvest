package options

import (
	"fmt"
)

type ModelOption struct {
	Option
	model string
}

func NewModelOption(p string) Option {

	o := &ModelOption{
		model: p,
	}

	return o
}

func (o *ModelOption) Type() OptionType {
	return ModelOptionType
}

func (o *ModelOption) Model() string {
	return o.model
}

func (o *ModelOption) String() string {
	return fmt.Sprintf("option:model=%s", o.model)
}
