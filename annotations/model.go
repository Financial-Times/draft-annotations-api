package annotations

import "net/http"

type Annotations struct {
	Annotations []Annotation `json:"annotations"`
}

type Annotation struct {
	Predicate  string `json:"predicate"`
	ConceptId  string `json:"id"`
	ApiUrl     string `json:"apiUrl,omitempty"`
	Type       string `json:"type,omitempty"`
	PrefLabel  string `json:"prefLabel,omitempty"`
	IsFTAuthor bool   `json:"isFTAuthor,omitempty"`
}

func userAgent(req *http.Request) {
	req.Header.Set("User-Agent", "PAC draft-annotations-api")
}
