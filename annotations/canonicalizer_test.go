package annotations

import (
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

const (
	about    = "http://www.ft.com/ontology/annotation/about"
	mentions = "http://www.ft.com/ontology/annotation/mentions"

	testType = "http://www.ft.com/ontology/TestType"
)

func TestCanonicalAnnotationSorterOrderByPredicate(t *testing.T) {
	conceptUuid := []string{
		uuid.New().String(),
		uuid.New().String(),
	}

	// not in order
	annotations := []interface{}{
		map[string]interface{}{
			"predicate": mentions,
			"id":        conceptUuid[0],
		},
		map[string]interface{}{
			"predicate": about,
			"id":        conceptUuid[1],
		},
	}

	sorter := NewCanonicalAnnotationSorter(annotations)
	sort.Sort(sorter)

	assert.Equal(t, about, annotations[0].(map[string]interface{})["predicate"], "first annotation predicate")
	assert.Equal(t, conceptUuid[1], annotations[0].(map[string]interface{})["id"], "first annotation concept id")
	assert.Equal(t, mentions, annotations[1].(map[string]interface{})["predicate"], "second annotation predicate")
	assert.Equal(t, conceptUuid[0], annotations[1].(map[string]interface{})["id"], "second annotation concept id")

	// already in order
	annotations = []interface{}{
		map[string]interface{}{
			"predicate": about,
			"id":        conceptUuid[0],
		},
		map[string]interface{}{
			"predicate": mentions,
			"id":        conceptUuid[1],
		},
	}

	sorter = NewCanonicalAnnotationSorter(annotations)
	sort.Sort(sorter)

	assert.Equal(t, about, annotations[0].(map[string]interface{})["predicate"], "first annotation predicate")
	assert.Equal(t, conceptUuid[0], annotations[0].(map[string]interface{})["id"], "first annotation concept id")
	assert.Equal(t, mentions, annotations[1].(map[string]interface{})["predicate"], "second annotation predicate")
	assert.Equal(t, conceptUuid[1], annotations[1].(map[string]interface{})["id"], "second annotation concept id")
}

func TestCanonicalAnnotationSorterEqualPredicateOrderByUUID(t *testing.T) {
	conceptUuid := []string{
		uuid.New().String(),
		uuid.New().String(),
	}

	if strings.Compare(conceptUuid[0], conceptUuid[1]) == -1 {
		// swap so that [0] is definitely later lexicographically than [1]
		conceptUuid[0], conceptUuid[1] = conceptUuid[1], conceptUuid[0]
	}

	annotations := []interface{}{
		map[string]interface{}{
			"predicate": about,
			"id":        conceptUuid[0],
		},
		map[string]interface{}{
			"predicate": about,
			"id":        conceptUuid[1],
		},
	}

	sorter := NewCanonicalAnnotationSorter(annotations)
	sort.Sort(sorter)

	assert.Equal(t, about, annotations[0].(map[string]interface{})["predicate"], "first annotation predicate")
	assert.Equal(t, conceptUuid[1], annotations[0].(map[string]interface{})["id"], "first annotation concept id")
	assert.Equal(t, about, annotations[1].(map[string]interface{})["predicate"], "second annotation predicate")
	assert.Equal(t, conceptUuid[0], annotations[1].(map[string]interface{})["id"], "second annotation concept id")

	// swap round so that they are in order
	conceptUuid[0], conceptUuid[1] = conceptUuid[1], conceptUuid[0]

	annotations = []interface{}{
		map[string]interface{}{
			"predicate": about,
			"id":        conceptUuid[0],
		},
		map[string]interface{}{
			"predicate": about,
			"id":        conceptUuid[1],
		},
	}

	sorter = NewCanonicalAnnotationSorter(annotations)
	sort.Sort(sorter)

	assert.Equal(t, about, annotations[0].(map[string]interface{})["predicate"], "first annotation predicate")
	assert.Equal(t, conceptUuid[0], annotations[0].(map[string]interface{})["id"], "first annotation concept id")
	assert.Equal(t, about, annotations[1].(map[string]interface{})["predicate"], "second annotation predicate")
	assert.Equal(t, conceptUuid[1], annotations[1].(map[string]interface{})["id"], "second annotation concept id")
}

func TestCanonicalizer(t *testing.T) {
	conceptUuid := []string{
		uuid.New().String(),
		uuid.New().String(),
	}

	apiUrl := make([]string, len(conceptUuid))
	for i, id := range conceptUuid {
		apiUrl[i] = fmt.Sprintf("http://www.ft.com/thing/%s", id)
	}

	prefLabel := []string{
		"Some concept",
		"Some other concept",
	}

	annotations := []interface{}{
		map[string]interface{}{
			"predicate":  mentions,
			"id":         conceptUuid[0],
			"apiUrl":     apiUrl[0],
			"type":       testType,
			"prefLabel":  prefLabel[0],
			"isFTAuthor": false,
		},
		map[string]interface{}{
			"predicate":  about,
			"id":         conceptUuid[1],
			"apiUrl":     apiUrl[1],
			"type":       testType,
			"prefLabel":  prefLabel[1],
			"isFTAuthor": false,
		},
	}

	c14n := NewCanonicalizer(NewCanonicalAnnotationSorter)
	actual := c14n.Canonicalize(annotations)

	assert.Equal(t, about, actual[0].(map[string]interface{})["predicate"], "first c14n annotation predicate")
	assert.Equal(t, conceptUuid[1], actual[0].(map[string]interface{})["id"], "first c14n annotation concept id")
	assert.Empty(t, actual[0].(map[string]interface{})["apiUrl"], "first c14n annotation apiUrl")
	assert.Empty(t, actual[0].(map[string]interface{})["type"], "first c14n annotation type")
	assert.Empty(t, actual[0].(map[string]interface{})["prefLabel"], "first c14n annotation prefLabel")

	assert.Equal(t, mentions, actual[1].(map[string]interface{})["predicate"], "second c14n annotation predicate")
	assert.Equal(t, conceptUuid[0], actual[1].(map[string]interface{})["id"], "second c14n annotation concept id")
	assert.Empty(t, actual[1].(map[string]interface{})["apiUrl"], "second c14n annotation apiUrl")
	assert.Empty(t, actual[1].(map[string]interface{})["type"], "second c14n annotation type")
	assert.Empty(t, actual[1].(map[string]interface{})["prefLabel"], "second c14n annotation prefLabel")

	// but the original annotation structs must not have been altered
	assert.Equal(t, mentions, annotations[0].(map[string]interface{})["predicate"], "first annotation predicate")
	assert.Equal(t, conceptUuid[0], annotations[0].(map[string]interface{})["id"], "first annotation concept id")
	assert.Equal(t, apiUrl[0], annotations[0].(map[string]interface{})["apiUrl"], "first annotation apiUrl")
	assert.Equal(t, testType, annotations[0].(map[string]interface{})["type"], "first annotation type")
	assert.Equal(t, prefLabel[0], annotations[0].(map[string]interface{})["prefLabel"], "first annotation prefLabel")

	assert.Equal(t, about, annotations[1].(map[string]interface{})["predicate"], "second annotation predicate")
	assert.Equal(t, conceptUuid[1], annotations[1].(map[string]interface{})["id"], "second annotation concept id")
	assert.Equal(t, apiUrl[1], annotations[1].(map[string]interface{})["apiUrl"], "second annotation apiUrl")
	assert.Equal(t, testType, annotations[1].(map[string]interface{})["type"], "second annotation type")
	assert.Equal(t, prefLabel[1], annotations[1].(map[string]interface{})["prefLabel"], "second annotation prefLabel")
}

func TestCanonicalizerHash(t *testing.T) {
	conceptUuid := []string{
		uuid.New().String(),
		uuid.New().String(),
	}

	apiUrl := make([]string, len(conceptUuid))
	for i, id := range conceptUuid {
		apiUrl[i] = fmt.Sprintf("http://www.ft.com/thing/%s", id)
	}

	prefLabel := []string{
		"Some concept",
		"Some other concept",
	}

	annotations1 := []interface{}{
		map[string]interface{}{
			"predicate":  mentions,
			"id":         conceptUuid[0],
			"apiUrl":     apiUrl[0],
			"type":       testType,
			"prefLabel":  prefLabel[0],
			"isFTAuthor": false,
		},
		map[string]interface{}{
			"predicate":  about,
			"id":         conceptUuid[1],
			"apiUrl":     apiUrl[1],
			"type":       testType,
			"prefLabel":  prefLabel[1],
			"isFTAuthor": false,
		},
	}

	annotations2 := []interface{}{
		map[string]interface{}{
			"predicate": about,
			"id":        conceptUuid[1],
			"type":      testType,
			"prefLabel": "bar",
		},
		map[string]interface{}{
			"predicate": mentions,
			"id":        conceptUuid[0],
			"type":      apiUrl[0],
			"prefLabel": "foo",
		},
	}

	c14n := NewCanonicalizer(NewCanonicalAnnotationSorter)
	h1 := c14n.hash(annotations1)
	h2 := c14n.hash(annotations2)
	assert.Equal(t, h1, h2, "canonical hash values")
}
