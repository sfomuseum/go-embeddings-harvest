package options

import (
	"fmt"
)

type ProviderOption struct {
	Option
	provider string
}

func NewProviderOption(p string) Option {

	o := &ProviderOption{
		provider: p,
	}

	return o
}

func (o *ProviderOption) Type() OptionType {
	return ProviderOptionType
}

func (o *ProviderOption) Provider() string {
	return o.provider
}

func (o *ProviderOption) String() string {
	return fmt.Sprintf("option:provider=%s", o.provider)
}

type SimilarProviderOption struct {
	Option
	provider string
}

func NewSimilarProviderOption(p string) Option {

	o := &SimilarProviderOption{
		provider: p,
	}

	return o
}

func (o *SimilarProviderOption) Type() OptionType {
	return SimilarProviderOptionType
}

func (o *SimilarProviderOption) SimilarProvider() string {
	return o.provider
}

func (o *SimilarProviderOption) String() string {
	return fmt.Sprintf("option:similar_provider=%s", o.provider)
}
