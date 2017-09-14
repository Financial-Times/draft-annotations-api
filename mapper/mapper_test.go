package mapper

import (
	"github.com/stretchr/testify/assert"
	"io/ioutil"
	"testing"
)

func TestIsClassifiedByMappedCorrectly(t *testing.T) {

	originalBody, err := ioutil.ReadFile("test_resources/annotations_isClassifiedBy_v2.json")
	if err != nil {
		panic(err)
	}
	expectedBody, err := ioutil.ReadFile("test_resources/annotations_isClassifiedBy_PAC.json")

	if err != nil {
		panic(err)
	}
	actualBody, _ := ConvertPredicates(originalBody)

	assert.JSONEq(t, string(expectedBody), string(actualBody), "they do not match")
}

func TestIsPrimariTestConnectionErrorlyClassifiedByMappedCorrectly(t *testing.T) {

	originalBody, err := ioutil.ReadFile("test_resources/annotations_isPrimarilyClassifiedBy_v2.json")
	if err != nil {
		panic(err)
	}
	expectedBody, err := ioutil.ReadFile("test_resources/annotations_isPrimarilyClassifiedBy_PAC.json")

	if err != nil {
		panic(err)
	}
	actualBody, _ := ConvertPredicates(originalBody)
	assert.JSONEq(t, string(expectedBody), string(actualBody), "they do not match")
}

func TestIsMajorMentionsMappedCorrectly(t *testing.T) {

	originalBody, err := ioutil.ReadFile("test_resources/annotations_majorMentions_v2.json")
	if err != nil {
		panic(err)
	}
	expectedBody, err := ioutil.ReadFile("test_resources/annotations_majorMentions_PAC.json")

	if err != nil {
		panic(err)
	}
	actualBody, _ := ConvertPredicates(originalBody)
	assert.JSONEq(t, string(expectedBody), string(actualBody), "they do not match")
}

func TestDiscardedAndEmpty(t *testing.T) {

	originalBody, err := ioutil.ReadFile("test_resources/annotations_discard.json")
	if err != nil {
		panic(err)
	}
	if err != nil {
		panic(err)
	}

	actualBody, _ := ConvertPredicates(originalBody)

	assert.True(t, actualBody == nil, "some annotations have not been discarded")
}

func TestDefaultPassThrough(t *testing.T) {

	originalBody, err := ioutil.ReadFile("test_resources/annotations_defaults.json")
	if err != nil {
		panic(err)
	}

	if err != nil {
		panic(err)
	}
	actualBody, _ := ConvertPredicates(originalBody)

	assert.JSONEq(t, string(originalBody), string(actualBody), "json not equal")

}
