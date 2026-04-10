package oembeddings

import (
	"fmt"
)

// OEmbeddings defines a model for the _least_ amount of metadata to be associated with a vector embedding record
// in order to allow a preview of the content used to create the embeddings and to display provenance for that content
// with links back to the subject depicted in the content on a provider's website. As the name suggests it is modeled
// in spirit after the OEmbed specification which descibes itself as "a format for allowing an embedded representation
// of a URL on third party sites.". The `Oembeddings` structure (propeties) MAY be present in the free-form "attributes"
// dictionary of a [Record] instance. To determine if it is you can use the [Validate] method included with this package.
type OEmbeddings struct {
	// The type of material used to create the vector embeddings. Expected to be "image" or "text".
	Type string `json:"type"`
	// The preview content for the vector embeddings. If `Type` is "text" then this is expected to be a string. If `Type` is "image" this is expected to be a string confirming to the JSON Schema "uri" type.
	Preview string `json:"preview"`
	// A web page (or resource) for the depiction used to create the vector embeddings.
	DepictionURL string `json:"depiction_url,omitempty"`
	// A web page (or resource) for the subject of the depiction used to create the vector embeddings.
	SubjectURL string `json:"subject_url"`
	// The title of the subject of the depiction.
	SubjectTitle string `json:"subject_title"`
	// The creditline or attribution for the subject of the depiction.
	SubjectCreditline string `json:"subject_creditline"`
	// The name of the provider (holder) of the subject being depicted.
	ProviderName string `json:"provider_name"`
	// The primary web page for the provider (holder) of the subject being depicted.
	ProviderURL string `json:"provider_url"`
}

func (o *OEmbeddings) AsMap() map[string]string {

	o_map := map[string]string{
		"type":               o.Type,
		"preview":            o.Preview,
		"subject_url":        o.SubjectURL,
		"subject_title":      o.SubjectTitle,
		"subject_creditline": o.SubjectCreditline,
		"provider_name":      o.ProviderName,
		"provider_url":       o.ProviderURL,
	}

	if o.DepictionURL != "" {
		o_map["depiction_url"] = o.DepictionURL
	}

	return o_map
}

func FromAttributes(attrs map[string]string) (*OEmbeddings, error) {

	valid, err := ValidateWithAttributes(attrs)

	if err != nil {
		return nil, err
	}

	if !valid {
		return nil, fmt.Errorf("Attributes does not conform to the OEmbeddings structure")
	}

	oe := &OEmbeddings{
		Type:              attrs["type"],
		Preview:           attrs["preview"],
		SubjectURL:        attrs["subject_url"],
		SubjectTitle:      attrs["subject_title"],
		SubjectCreditline: attrs["subject_creditline"],
		ProviderName:      attrs["provider_name"],
		ProviderURL:       attrs["provider_url"],
	}

	v, exists := attrs["depiction_url"]

	if exists {
		oe.DepictionURL = v
	}

	return oe, nil
}
