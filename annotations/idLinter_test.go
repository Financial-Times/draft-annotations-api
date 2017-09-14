package annotations

import (
	"context"
	"testing"

	"github.com/satori/go.uuid"
	"github.com/stretchr/testify/assert"
)

func TestIdLinter(t *testing.T) {
	l, err := NewIDLinter(`^(.+)\/\/api\.ft\.com\/things\/(.+)$`,"$1//www.ft.com/thing/$2")
	assert.NoError(t, err)

	matchingUuid := uuid.NewV4().String()
	matchingId := "http://api.ft.com/things/" + matchingUuid
	matching := Annotation{
		Predicate: isClassifiedBy,
		ConceptId: matchingId,
		ApiUrl:    matchingId,
		Types:     []string{
			"http://www.ft.com/ontology/core/Thing",
			"http://www.ft.com/ontology/concept/Concept",
			"http://www.ft.com/ontology/classification/Classification",
			"http://www.ft.com/ontology/product/Brand",
			},
		PrefLabel: "A thing with a matching ID",
	}

	nonMatchingUuid := uuid.NewV4().String()
	nonMatchingId := "http://api.ft.com/thing/" + nonMatchingUuid
	nonMatching := Annotation{
		Predicate: isClassifiedBy,
		ConceptId: nonMatchingId,
		ApiUrl:    nonMatchingId,
		Types:     []string{
			"http://www.ft.com/ontology/core/Thing",
			"http://www.ft.com/ontology/concept/Concept",
			"http://www.ft.com/ontology/Topic",
		},
		PrefLabel: "A thing with a non-matching ID",
	}

	actual, err := l.Apply(context.Background(), []Annotation{matching, nonMatching})
	assert.NoError(t, err)

	assert.Len(t, actual, 2, "linted annotations")

	assert.Equal(t, matching.Predicate, actual[0].Predicate, "linted predicate for matching annotation")
	assert.Equal(t, "http://www.ft.com/thing/" + matchingUuid, actual[0].ConceptId, "linted concept ID for matching annotation")
	assert.Equal(t, matching.ApiUrl, actual[0].ApiUrl, "linted concept ApiUrl for matching annotation")
	assert.Equal(t, matching.Types, actual[0].Types, "linted concept types for matching annotation")
	assert.Equal(t, matching.PrefLabel, actual[0].PrefLabel, "linted concept prefLabel for matching annotation")

	assert.Equal(t, nonMatching.Predicate, actual[1].Predicate, "linted predicate for non-matching annotation")
	assert.Equal(t, nonMatching.ConceptId, actual[1].ConceptId, "linted concept ID for non-matching annotation")
	assert.Equal(t, nonMatching.ApiUrl, actual[1].ApiUrl, "linted concept ApiUrl for non-matching annotation")
	assert.Equal(t, nonMatching.Types, actual[1].Types, "linted concept types for non-matching annotation")
	assert.Equal(t, nonMatching.PrefLabel, actual[1].PrefLabel, "linted concept prefLabel for non-matching annotation")
}


func TestBadRegex(t *testing.T) {
	l, err := NewIDLinter(`^(.+)\/\/api\.ft\.com\/things\/(.$`, "$1//www.ft.com/thing/$2")
	assert.Error(t, err)
	assert.Nil(t, l, "should not have created an idLinter")
}
