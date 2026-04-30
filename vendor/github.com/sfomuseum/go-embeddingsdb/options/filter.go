package options

type FilterOption struct {
	Option
	key   string
	value any
}

func NewFilterOption(key string, value any) Option {

	o := &FilterOption{
		key:   key,
		value: value,
	}

	return o
}

func (o *FilterOption) Type() OptionType {
	return FilterOptionType
}

func (o *FilterOption) Key() string {
	return o.key
}

func (o *FilterOption) Value() any {
	return o.value
}
