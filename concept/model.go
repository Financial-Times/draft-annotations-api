package concept

// SearchResult models the data returned from the UPP concepts API used (i.e. internal-concordances)
type SearchResult struct {
	Concepts map[string]Concept `json:"concepts"`
}

// Concept models the concept data returned from the UPP concepts API
type Concept struct {
	ID         string `json:"id"`
	ApiUrl     string `json:"apiUrl,omitempty"`
	Type       string `json:"type,omitempty"`
	PrefLabel  string `json:"prefLabel,omitempty"`
	IsFTAuthor bool   `json:"isFTAuthor,omitempty"`
}
