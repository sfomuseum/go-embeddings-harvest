package options

type OptionType uint8

const (
	UndefinedOptionType OptionType = iota
	DimensionsOptionType
	FilterOptionType
	ProviderOptionType
	SimilarProviderOptionType
	MaxDistanceOptionType
	MaxResultsOptionType
	ModelOptionType
)

type Option interface {
	Type() OptionType
	String() string
}
