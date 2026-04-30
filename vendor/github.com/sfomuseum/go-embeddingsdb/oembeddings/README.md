## OEmbeddings

_Note: "OEmbeddings" should still be considered work in progress and subject to review and suggestions._

OEmbeddings defines a model for the _least_ amount of metadata to be associated with a vector embedding record in order to allow a preview of the content used to create the embeddings and to display provenance for that content with links back to the subject depicted in the content on a provider's website.

As the name suggests it is modeled in spirit after the [OEmbed specification](https://oembed.com/) which descibes itself as "a format for allowing an embedded representation of a URL on third party sites.". The `Oembeddings` structure (propeties) MAY be present in the free-form "attributes" dictionary of a `Record` instance but is not required.

```
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
```

### JSON Schema

There is a [JSON Schema document](oembeddings/oembeddings.json) for validating an "attributes" dictionary to ensure that it contains the required fields for an `OEmbeddings` data structure.

### WebAssembly

There is also an `oembeddings_validate` WebAssembly (WASM) binary for use with JavaScript. For example:

```
const input = document.querySelector("#input");
const feedback = document.querySelector("#feedback");

const oe = input.value;

oembeddings_validate(oe).then((rsp) => {
	feedback.innerText = "Document validates as OEmbeddings";
}).catch((err) => {
	console.error("Validation failed");
	feedback.innerText = "Validation failed: " + err;
});
```

The WASM binary needs to be built manually using the `make wasmjs` Makefile target. See the [oembeddings/www](oembeddings/www) folder for details.

