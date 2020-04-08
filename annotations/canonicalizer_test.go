package annotations

import (
	"fmt"
	"sort"
	"strings"
	"testing"

	uuid "github.com/satori/go.uuid"
	"github.com/stretchr/testify/assert"
)

const (
	about    = "http://www.ft.com/ontology/annotation/about"
	mentions = "http://www.ft.com/ontology/annotation/mentions"

	testType = "http://www.ft.com/ontology/TestType"
)

func TestCanonicalAnnotationSorterOrderByPredicate(t *testing.T) {
	conceptUuid := []string{
		uuid.NewV4().String(),
		uuid.NewV4().String(),
	}

	// not in order
	annotations := []Annotation{
		{
			Predicate: mentions,
			ConceptId: conceptUuid[0],
		},
		{
			Predicate: about,
			ConceptId: conceptUuid[1],
		},
	}

	sorter := NewCanonicalAnnotationSorter(annotations)
	sort.Sort(sorter)

	assert.Equal(t, about, annotations[0].Predicate, "first annotation predicate")
	assert.Equal(t, conceptUuid[1], annotations[0].ConceptId, "first annotation concept id")
	assert.Equal(t, mentions, annotations[1].Predicate, "second annotation predicate")
	assert.Equal(t, conceptUuid[0], annotations[1].ConceptId, "second annotation concept id")

	// already in order
	annotations = []Annotation{
		{
			Predicate: about,
			ConceptId: conceptUuid[0],
		},
		{
			Predicate: mentions,
			ConceptId: conceptUuid[1],
		},
	}

	sorter = NewCanonicalAnnotationSorter(annotations)
	sort.Sort(sorter)

	assert.Equal(t, about, annotations[0].Predicate, "first annotation predicate")
	assert.Equal(t, conceptUuid[0], annotations[0].ConceptId, "first annotation concept id")
	assert.Equal(t, mentions, annotations[1].Predicate, "second annotation predicate")
	assert.Equal(t, conceptUuid[1], annotations[1].ConceptId, "second annotation concept id")
}

func TestCanonicalAnnotationSorterEqualPredicateOrderByUUID(t *testing.T) {
	conceptUuid := []string{
		uuid.NewV4().String(),
		uuid.NewV4().String(),
	}

	if strings.Compare(conceptUuid[0], conceptUuid[1]) == -1 {
		// swap so that [0] is definitely later lexicographically than [1]
		conceptUuid[0], conceptUuid[1] = conceptUuid[1], conceptUuid[0]
	}

	annotations := []Annotation{
		{
			Predicate: about,
			ConceptId: conceptUuid[0],
		},
		{
			Predicate: about,
			ConceptId: conceptUuid[1],
		},
	}

	sorter := NewCanonicalAnnotationSorter(annotations)
	sort.Sort(sorter)

	assert.Equal(t, about, annotations[0].Predicate, "first annotation predicate")
	assert.Equal(t, conceptUuid[1], annotations[0].ConceptId, "first annotation concept id")
	assert.Equal(t, about, annotations[1].Predicate, "second annotation predicate")
	assert.Equal(t, conceptUuid[0], annotations[1].ConceptId, "second annotation concept id")

	// swap round so that they are in order
	conceptUuid[0], conceptUuid[1] = conceptUuid[1], conceptUuid[0]

	annotations = []Annotation{
		{
			Predicate: about,
			ConceptId: conceptUuid[0],
		},
		{
			Predicate: about,
			ConceptId: conceptUuid[1],
		},
	}

	sorter = NewCanonicalAnnotationSorter(annotations)
	sort.Sort(sorter)

	assert.Equal(t, about, annotations[0].Predicate, "first annotation predicate")
	assert.Equal(t, conceptUuid[0], annotations[0].ConceptId, "first annotation concept id")
	assert.Equal(t, about, annotations[1].Predicate, "second annotation predicate")
	assert.Equal(t, conceptUuid[1], annotations[1].ConceptId, "second annotation concept id")
}

func TestCanonicalizer(t *testing.T) {
	conceptUuid := []string{
		uuid.NewV4().String(),
		uuid.NewV4().String(),
	}

	apiUrl := make([]string, len(conceptUuid))
	for i, id := range conceptUuid {
		apiUrl[i] = fmt.Sprintf("http://www.ft.com/thing/%s", id)
	}

	prefLabel := []string{
		"Some concept",
		"Some other concept",
	}

	annotations := []Annotation{
		{
			mentions,
			conceptUuid[0],
			apiUrl[0],
			testType,
			prefLabel[0],
			false,
		},
		{
			about,
			conceptUuid[1],
			apiUrl[1],
			testType,
			prefLabel[1],
			false,
		},
	}

	c14n := NewCanonicalizer(NewCanonicalAnnotationSorter)
	actual := c14n.Canonicalize(annotations)

	assert.Equal(t, about, actual[0].Predicate, "first c14n annotation predicate")
	assert.Equal(t, conceptUuid[1], actual[0].ConceptId, "first c14n annotation concept id")
	assert.Empty(t, actual[0].ApiUrl, "first c14n annotation apiUrl")
	assert.Empty(t, actual[0].Type, "first c14n annotation type")
	assert.Empty(t, actual[0].PrefLabel, "first c14n annotation prefLabel")

	assert.Equal(t, mentions, actual[1].Predicate, "second c14n annotation predicate")
	assert.Equal(t, conceptUuid[0], actual[1].ConceptId, "second c14n annotation concept id")
	assert.Empty(t, actual[1].ApiUrl, "second c14n annotation apiUrl")
	assert.Empty(t, actual[1].Type, "second c14n annotation type")
	assert.Empty(t, actual[1].PrefLabel, "second c14n annotation prefLabel")

	// but the original annotation structs must not have been altered
	assert.Equal(t, mentions, annotations[0].Predicate, "first annotation predicate")
	assert.Equal(t, conceptUuid[0], annotations[0].ConceptId, "first annotation concept id")
	assert.Equal(t, apiUrl[0], annotations[0].ApiUrl, "first annotation apiUrl")
	assert.Equal(t, testType, annotations[0].Type, "first annotation type")
	assert.Equal(t, prefLabel[0], annotations[0].PrefLabel, "first annotation prefLabel")

	assert.Equal(t, about, annotations[1].Predicate, "second annotation predicate")
	assert.Equal(t, conceptUuid[1], annotations[1].ConceptId, "second annotation concept id")
	assert.Equal(t, apiUrl[1], annotations[1].ApiUrl, "second annotation apiUrl")
	assert.Equal(t, testType, annotations[1].Type, "second annotation type")
	assert.Equal(t, prefLabel[1], annotations[1].PrefLabel, "second annotation prefLabel")
}

func TestCanonicalizerHash(t *testing.T) {
	conceptUuid := []string{
		uuid.NewV4().String(),
		uuid.NewV4().String(),
	}

	apiUrl := make([]string, len(conceptUuid))
	for i, id := range conceptUuid {
		apiUrl[i] = fmt.Sprintf("http://www.ft.com/thing/%s", id)
	}

	prefLabel := []string{
		"Some concept",
		"Some other concept",
	}

	annotations1 := []Annotation{
		{
			mentions,
			conceptUuid[0],
			apiUrl[0],
			testType,
			prefLabel[0],
			false,
		},
		{
			about,
			conceptUuid[1],
			apiUrl[1],
			testType,
			prefLabel[1],
			false,
		},
	}

	annotations2 := []Annotation{
		{
			Predicate: about,
			ConceptId: conceptUuid[1],
			Type:      testType,
			PrefLabel: "bar",
		},
		{
			Predicate: mentions,
			ConceptId: conceptUuid[0],
			ApiUrl:    apiUrl[0],
			PrefLabel: "foo",
		},
	}

	c14n := NewCanonicalizer(NewCanonicalAnnotationSorter)
	h1 := c14n.hash(annotations1)
	h2 := c14n.hash(annotations2)
	assert.Equal(t, h1, h2, "canonical hash values")
}
