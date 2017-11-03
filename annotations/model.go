package annotations

type Annotation struct {
	Predicate  string   `json:"predicate"`
	ConceptId  string   `json:"id"`
	ApiUrl     string   `json:"apiUrl,omitempty"`
	Types      []string `json:"types,omitempty"`
	PrefLabel  string   `json:"prefLabel,omitempty"`
	IsFTAuthor bool    `json:"isFTAuthor,omitempty"`
}

type Concepts []Concept

type Concept struct {
	Id         string   `json:"id"`
	ApiUrl     string   `json:"apiUrl,omitempty"`
	Types      []string `json:"types,omitempty"`
	PrefLabel  string   `json:"prefLabel,omitempty"`
	IsFTAuthor bool    `json:"isFTAuthor,omitempty"`
}
