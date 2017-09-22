package annotations

import (
	"bytes"
	"io/ioutil"
	"testing"
	"encoding/json"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func readAnnotations(t *testing.T, fileName string) []Annotation {
	body, err := ioutil.ReadFile(fileName)
	require.NoError(t, err)

	ann := []Annotation{}
	err = json.NewDecoder(bytes.NewReader(body)).Decode(&ann)
	require.NoError(t, err)

	return ann
}

func TestIsClassifiedByMappedCorrectly(t *testing.T) {
	originalAnnotations := readAnnotations(t,"test_resources/annotations_isClassifiedBy_v2.json")
	expectedAnnotations := readAnnotations(t,"test_resources/annotations_isClassifiedBy_PAC.json")

	actualAnnotations := ConvertPredicates(originalAnnotations)

	assert.Equal(t, expectedAnnotations, actualAnnotations, "annotations")
}

func TestIsPrimariTestConnectionErrorlyClassifiedByMappedCorrectly(t *testing.T) {
	originalAnnotations := readAnnotations(t,"test_resources/annotations_isPrimarilyClassifiedBy_v2.json")
	expectedAnnotations := readAnnotations(t,"test_resources/annotations_isPrimarilyClassifiedBy_PAC.json")

	actualAnnotations := ConvertPredicates(originalAnnotations)
	assert.Equal(t, expectedAnnotations, actualAnnotations, "annotations")
}

func TestIsMajorMentionsMappedCorrectly(t *testing.T) {
	originalAnnotations := readAnnotations(t,"test_resources/annotations_majorMentions_v2.json")
	expectedAnnotations := readAnnotations(t,"test_resources/annotations_majorMentions_PAC.json")

	actualAnnotations := ConvertPredicates(originalAnnotations)
	assert.Equal(t, expectedAnnotations, actualAnnotations, "annotations")
}

func TestDiscardedAndEmpty(t *testing.T) {
	originalAnnotations := readAnnotations(t,"test_resources/annotations_discard.json")

	actualAnnotations := ConvertPredicates(originalAnnotations)

	assert.Empty(t, actualAnnotations, "some annotations have not been discarded")
}

func TestDefaultPassThrough(t *testing.T) {
	originalAnnotations := readAnnotations(t,"test_resources/annotations_defaults.json")

	actualAnnotations := ConvertPredicates(originalAnnotations)

	assert.Equal(t, originalAnnotations, actualAnnotations, "annotations")
}
