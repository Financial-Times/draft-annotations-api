package concept

type SearchResult struct {
	Concepts []Concept `"json:concepts"`
}

type Concept struct {
	Id         string `json:"id"`
	ApiUrl     string `json:"apiUrl,omitempty"`
	Type       string `json:"type,omitempty"`
	PrefLabel  string `json:"prefLabel,omitempty"`
	IsFTAuthor bool   `json:"isFTAuthor,omitempty"`
}
