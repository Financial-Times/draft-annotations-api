package annotations

type Annotation struct {
	Predicate  string `json:"predicate"`
	ConceptId  string `json:"id"`
	ApiUrl     string `json:"apiUrl,omitempty"`
	Type       string `json:"type,omitempty"`
	PrefLabel  string `json:"prefLabel,omitempty"`
	IsFTAuthor bool   `json:"isFTAuthor,omitempty"`
}
