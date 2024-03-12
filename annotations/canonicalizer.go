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
	sorterFactory func(ann []interface{}) sort.Interface
}

type annotationSorter struct {
	ann []interface{}
}

func NewCanonicalizer(sorterFactory func(ann []interface{}) sort.Interface) *Canonicalizer {
	return &Canonicalizer{sorterFactory}
}

func (c *Canonicalizer) Canonicalize(in []interface{}) []interface{} {
	out := make([]interface{}, len(in))
	for i, ann := range in {
		out[i] = *c.deplete(ann.(map[string]interface{}))
	}

	sort.Sort(c.sorterFactory(out))
	return out
}

func (c *Canonicalizer) deplete(in map[string]interface{}) *map[string]interface{} {
	return &map[string]interface{}{"predicate": in["predicate"], "id": in["id"]}
}

// Hash hashes the given payload in SHA224 + Hex
func (c *Canonicalizer) hash(ann []interface{}) string {
	out := bytes.NewBuffer([]byte{})
	canonical := c.Canonicalize(ann)
	json.NewEncoder(out).Encode(canonical)
	hash := sha256.New224()
	hash.Write(out.Bytes())

	return hex.EncodeToString(hash.Sum(nil))
}

func NewCanonicalAnnotationSorter(ann []interface{}) sort.Interface {
	return &annotationSorter{ann}
}

func (s *annotationSorter) Len() int {
	return len(s.ann)
}

func (s *annotationSorter) Less(i, j int) bool {
	predicateI := s.ann[i].(map[string]interface{})["predicate"]
	if predicateI == nil {
		predicateI = ""
	}
	predicateJ := s.ann[j].(map[string]interface{})["predicate"]
	if predicateJ == nil {
		predicateJ = ""
	}
	conceptIDI := s.ann[i].(map[string]interface{})["id"]
	if conceptIDI == nil {
		conceptIDI = ""
	}
	conceptIDJ := s.ann[j].(map[string]interface{})["id"]
	if conceptIDJ == nil {
		conceptIDJ = ""
	}

	compare := strings.Compare(predicateI.(string), predicateJ.(string))
	if compare == 0 {
		compare = strings.Compare(conceptIDI.(string), conceptIDJ.(string))
	}

	return compare == -1
}

func (s *annotationSorter) Swap(i, j int) {
	s.ann[i], s.ann[j] = s.ann[j], s.ann[i]
}
