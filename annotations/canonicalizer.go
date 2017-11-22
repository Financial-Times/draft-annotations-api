package annotations

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strings"
)

type Canonicalizer struct {
	sorterFactory func(ann []Annotation) sort.Interface
}

type annotationSorter struct {
	ann []Annotation
}

func NewCanonicalizer(sorterFactory func(ann []Annotation) sort.Interface) *Canonicalizer {
	return &Canonicalizer{sorterFactory}
}

func (c *Canonicalizer) Canonicalize(in []Annotation) []Annotation {
	out := make([]Annotation, len(in))
	for i, ann := range in {
		out[i] = *c.deplete(ann)
	}

	sort.Sort(c.sorterFactory(out))
	return out
}

func (c *Canonicalizer) deplete(in Annotation) *Annotation {
	return &Annotation{Predicate: in.Predicate, ConceptId: in.ConceptId}
}

// Hash hashes the given payload in SHA224 + Hex
func (c *Canonicalizer) hash(ann []Annotation) string {
	out := bytes.NewBuffer([]byte{})
	canonical := c.Canonicalize(ann)
	json.NewEncoder(out).Encode(canonical)
	hash := sha256.New224()
	hash.Write(out.Bytes())

	return hex.EncodeToString(hash.Sum(nil))
}

func NewCanonicalAnnotationSorter(ann []Annotation) sort.Interface {
	return &annotationSorter{ann}
}

func (s *annotationSorter) Len() int {
	return len(s.ann)
}

func (s *annotationSorter) Less(i, j int) bool {
	compare := strings.Compare(s.ann[i].Predicate, s.ann[j].Predicate)
	if compare == 0 {
		compare = strings.Compare(s.ann[i].ConceptId, s.ann[j].ConceptId)
	}

	return compare == -1
}

func (s *annotationSorter) Swap(i, j int) {
	s.ann[i], s.ann[j] = s.ann[j], s.ann[i]
}
