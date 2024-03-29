package mapper

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestConvertPredicates(t *testing.T) {
	tests := []struct {
		name            string
		fixtureBaseName string
	}{
		{"IsClassifiedByMappedCorrectly", "annotations_isClassifiedBy"},
		{"IsPrimariTestConnectionErrorlyClassifiedByMappedCorrectly", "annotations_isPrimarilyClassifiedBy"},
		{"IsMajorMentionsMappedCorrectly", "annotations_majorMentions"},
		{"DefaultPassThrough", "annotations_defaults"},
		{"ImplicitAnnotationsAreFiltered", "annotations_implicit"},
		{"InvalidAnnotationsAreFiltered", "annotations_invalid"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			originalBody, err := os.ReadFile("testdata/" + test.fixtureBaseName + "_v2.json")
			if err != nil {
				t.Fatal(err)
			}
			expectedBody, err := os.ReadFile("testdata/" + test.fixtureBaseName + "_PAC.json")
			if err != nil {
				t.Fatal(err)
			}

			actualBody, _ := ConvertPredicates(originalBody)
			assert.JSONEq(t, string(expectedBody), string(actualBody), "they do not match")
		})
	}
}

func TestDiscardedAndEmpty(t *testing.T) {
	originalBody, err := os.ReadFile("testdata/annotations_discard.json")
	if err != nil {
		t.Fatal(err)
	}
	actualBody, _ := ConvertPredicates(originalBody)

	assert.True(t, actualBody == nil, "some annotations have not been discarded")
}
