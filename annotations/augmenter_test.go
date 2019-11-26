package annotations

import (
	"context"
	"encoding/json"
	"errors"
	"io/ioutil"
	"testing"

	"github.com/Financial-Times/draft-annotations-api/concept"
	tidUtils "github.com/Financial-Times/transactionid-utils-go"
	log "github.com/sirupsen/logrus"
	logTest "github.com/sirupsen/logrus/hooks/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

var testCanonicalizedAnnotations = []Annotation{
	{
		Predicate: "http://www.ft.com/ontology/classification/isClassifiedBy",
		ConceptId: "http://www.ft.com/thing/b224ad07-c818-3ad6-94af-a4d351dbb619",
	},
	{
		Predicate: "http://www.ft.com/ontology/annotation/mentions",
		ConceptId: "http://www.ft.com/thing/1a2a1a0a-7199-38b8-8a73-e651e2172471",
	},
	{
		Predicate: "http://www.ft.com/ontology/hasContributor",
		ConceptId: "http://www.ft.com/thing/5bd49568-6d7c-3c10-a5b0-2f3fd5974a6b",
	},
	{
		Predicate: "http://www.ft.com/ontology/annotation/mentions",
		ConceptId: "http://www.ft.com/thing/1fb3faf1-bf00-3a15-8efb-1038a59653f7",
	},
	{
		Predicate: "http://www.ft.com/ontology/annotation/mentions",
		ConceptId: "http://www.ft.com/thing/7b7dafa0-d54e-4c1d-8e22-3d452792acd2",
	},
}

var testConceptIDs = []string{
	"b224ad07-c818-3ad6-94af-a4d351dbb619",
	"1a2a1a0a-7199-38b8-8a73-e651e2172471",
	"5bd49568-6d7c-3c10-a5b0-2f3fd5974a6b",
	"1fb3faf1-bf00-3a15-8efb-1038a59653f7",
	"7b7dafa0-d54e-4c1d-8e22-3d452792acd2",
}

var testConcepts = map[string]concept.Concept{
	"b224ad07-c818-3ad6-94af-a4d351dbb619": {
		ID:        "http://www.ft.com/thing/b224ad07-c818-3ad6-94af-a4d351dbb619",
		ApiUrl:    "http://api.ft.com/things/b224ad07-c818-3ad6-94af-a4d351dbb619",
		Type:      "http://www.ft.com/ontology/Subject",
		PrefLabel: "Economic Indicators",
	},
	"5bd49568-6d7c-3c10-a5b0-2f3fd5974a6b": {
		ID:         "http://www.ft.com/thing/5bd49568-6d7c-3c10-a5b0-2f3fd5974a6b",
		ApiUrl:     "http://api.ft.com/things/5bd49568-6d7c-3c10-a5b0-2f3fd5974a6b",
		Type:       "http://www.ft.com/ontology/person/Person",
		PrefLabel:  "Lisa Barrett",
		IsFTAuthor: true,
	},
	"7b7dafa0-d54e-4c1d-8e22-3d452792acd2": {
		ID:         "http://www.ft.com/thing/28f8d585-37ea-4879-ae1c-f6c0580a43b8",
		ApiUrl:     "http://api.ft.com/things/28f8d585-37ea-4879-ae1c-f6c0580a43b8",
		Type:       "http://www.ft.com/ontology/person/Person",
		PrefLabel:  "Frederick Stapleton",
		IsFTAuthor: false,
	},
}

var expectedAugmentedAnnotations = []Annotation{
	{
		Predicate:  "http://www.ft.com/ontology/classification/isClassifiedBy",
		ConceptId:  "http://www.ft.com/thing/b224ad07-c818-3ad6-94af-a4d351dbb619",
		ApiUrl:     "http://api.ft.com/things/b224ad07-c818-3ad6-94af-a4d351dbb619",
		Type:       "http://www.ft.com/ontology/Subject",
		PrefLabel:  "Economic Indicators",
		IsFTAuthor: false,
	},
	{
		Predicate:  "http://www.ft.com/ontology/hasContributor",
		ConceptId:  "http://www.ft.com/thing/5bd49568-6d7c-3c10-a5b0-2f3fd5974a6b",
		ApiUrl:     "http://api.ft.com/things/5bd49568-6d7c-3c10-a5b0-2f3fd5974a6b",
		Type:       "http://www.ft.com/ontology/person/Person",
		PrefLabel:  "Lisa Barrett",
		IsFTAuthor: true,
	},
	{
		Predicate:  "http://www.ft.com/ontology/annotation/mentions",
		ConceptId:  "http://www.ft.com/thing/28f8d585-37ea-4879-ae1c-f6c0580a43b8",
		ApiUrl:     "http://api.ft.com/things/28f8d585-37ea-4879-ae1c-f6c0580a43b8",
		Type:       "http://www.ft.com/ontology/person/Person",
		PrefLabel:  "Frederick Stapleton",
		IsFTAuthor: false,
	},
}

var testReturnSingleConceptID = []string{
	"b224ad07-c818-3ad6-94af-a4d351dbb619",
}

var testReturnSingleConcept = map[string]concept.Concept{
	"b224ad07-c818-3ad6-94af-a4d351dbb619": {
		ID:        "http://www.ft.com/thing/b224ad07-c818-3ad6-94af-a4d351dbb619",
		ApiUrl:    "http://api.ft.com/things/b224ad07-c818-3ad6-94af-a4d351dbb619",
		Type:      "http://www.ft.com/ontology/Subject",
		PrefLabel: "Economic Indicators",
	},
}

func TestAugmentAnnotations(t *testing.T) {
	matcher := mock.MatchedBy(func(l1 []string) bool {
		return assert.ElementsMatch(t, l1, testConceptIDs)
	})

	conceptRead := new(ConceptReadAPIMock)
	ctx := tidUtils.TransactionAwareContext(context.Background(), tidUtils.NewTransactionID())
	conceptRead.
		On("GetConceptsByIDs", ctx, matcher).
		Return(testConcepts, nil)
	a := NewAugmenter(conceptRead)

	annotations, err := a.AugmentAnnotations(ctx, testCanonicalizedAnnotations)

	assert.NoError(t, err)
	assert.Equal(t, len(expectedAugmentedAnnotations), len(annotations))
	assert.ElementsMatch(t, annotations, expectedAugmentedAnnotations)
	conceptRead.AssertExpectations(t)
}

func TestAugmentAnnotationsFixtures(t *testing.T) {
	tests := []struct {
		name            string
		fixtureBaseName string
	}{
		{"MultiPredicatePerConcept", "multiple-predicates-per-concept"},
		{"ShouldFilterDuplicatedAnnotations", "filter-duplicate-annotations"},
		{"ShouldFilterAnnotationsWithInvalidPredicate", "filter-invalid-predicates"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			matcher := mock.MatchedBy(func(l1 []string) bool {
				return assert.ElementsMatch(t, l1, testReturnSingleConceptID)
			})

			conceptRead := new(ConceptReadAPIMock)
			ctx := tidUtils.TransactionAwareContext(context.Background(), tidUtils.NewTransactionID())
			conceptRead.
				On("GetConceptsByIDs", ctx, matcher).
				Return(testReturnSingleConcept, nil)
			a := NewAugmenter(conceptRead)

			originalAnnotations := helperGetAnnotationsFromFixture(t, "augmenter-input-"+test.fixtureBaseName)
			annotations, err := a.AugmentAnnotations(ctx, originalAnnotations)
			assert.NoError(t, err)

			expectedAnnotations := helperGetAnnotationsFromFixture(t, "augmenter-expected-"+test.fixtureBaseName)

			assert.Equal(t, annotations, expectedAnnotations)
			conceptRead.AssertExpectations(t)
		})
	}
}

func TestAugmentAnnotationsArrayShouldNotBeNull(t *testing.T) {
	matcher := mock.MatchedBy(func(l1 []string) bool {
		return assert.ElementsMatch(t, l1, testConceptIDs)
	})
	conceptRead := new(ConceptReadAPIMock)
	ctx := tidUtils.TransactionAwareContext(context.Background(), tidUtils.NewTransactionID())
	conceptRead.
		On("GetConceptsByIDs", ctx, matcher).
		Return(make(map[string]concept.Concept), nil)
	a := NewAugmenter(conceptRead)

	annotations, err := a.AugmentAnnotations(ctx, testCanonicalizedAnnotations)

	assert.NoError(t, err)
	assert.NotNil(t, annotations)
	assert.Len(t, annotations, 0)
	conceptRead.AssertExpectations(t)
}

func TestAugmentAnnotationsMissingTransactionID(t *testing.T) {
	matcher := mock.MatchedBy(func(l1 []string) bool {
		return assert.ElementsMatch(t, l1, testConceptIDs)
	})
	hook := logTest.NewGlobal()
	conceptRead := new(ConceptReadAPIMock)
	conceptRead.
		On("GetConceptsByIDs", mock.Anything, matcher).
		Return(testConcepts, nil)
	a := NewAugmenter(conceptRead)

	// nolint errcheck
	a.AugmentAnnotations(context.Background(), testCanonicalizedAnnotations)

	var tid string
	for i, e := range hook.AllEntries() {
		if i == 0 {
			assert.Equal(t, log.WarnLevel, e.Level)
			assert.Equal(t, "Transaction ID error in augmenting annotations with concept data: Generated a new transaction ID", e.Message)
			tid = e.Data[tidUtils.TransactionIDKey].(string)
			assert.NotEmpty(t, tid)
		} else {
			assert.Equal(t, tid, e.Data[tidUtils.TransactionIDKey])
		}
	}

	conceptRead.AssertExpectations(t)
}

func TestAugmentAnnotationsConceptSearchError(t *testing.T) {
	matcher := mock.MatchedBy(func(l1 []string) bool {
		return assert.ElementsMatch(t, l1, testConceptIDs)
	})
	conceptRead := new(ConceptReadAPIMock)
	ctx := tidUtils.TransactionAwareContext(context.Background(), tidUtils.NewTransactionID())
	conceptRead.
		On("GetConceptsByIDs", ctx, matcher).
		Return(map[string]concept.Concept{}, errors.New("one minute to midnight"))
	a := NewAugmenter(conceptRead)

	_, err := a.AugmentAnnotations(ctx, testCanonicalizedAnnotations)

	assert.Error(t, err)

	conceptRead.AssertExpectations(t)
}

func TestAugmentAnnotationsWithInvalidConceptID(t *testing.T) {
	matcher := mock.MatchedBy(func(l1 []string) bool {
		return assert.ElementsMatch(t, l1, testConceptIDs)
	})
	conceptRead := new(ConceptReadAPIMock)
	ctx := tidUtils.TransactionAwareContext(context.Background(), tidUtils.NewTransactionID())
	conceptRead.
		On("GetConceptsByIDs", ctx, matcher).
		Return(testConcepts, nil)
	a := NewAugmenter(conceptRead)

	testCanonicalizedAnnotations = append(testCanonicalizedAnnotations, Annotation{ConceptId: "xyz"})
	annotations, err := a.AugmentAnnotations(ctx, testCanonicalizedAnnotations)

	assert.NoError(t, err)
	assert.Equal(t, len(expectedAugmentedAnnotations), len(annotations))
	for _, expected := range expectedAugmentedAnnotations {
		assert.Contains(t, annotations, expected)
	}

	conceptRead.AssertExpectations(t)
}

type ConceptReadAPIMock struct {
	mock.Mock
}

func (m *ConceptReadAPIMock) GetConceptsByIDs(ctx context.Context, ids []string) (map[string]concept.Concept, error) {
	args := m.Called(ctx, ids)
	return args.Get(0).(map[string]concept.Concept), args.Error(1)
}

func (m *ConceptReadAPIMock) GTG() error {
	args := m.Called()
	return args.Error(0)
}

func (m *ConceptReadAPIMock) Endpoint() string {
	args := m.Called()
	return args.String(0)
}

func helperGetAnnotationsFromFixture(t *testing.T, fixtureName string) []Annotation {
	j, err := ioutil.ReadFile("testdata/" + fixtureName + ".json")
	if err != nil {
		t.Fatal(err)
	}
	var a []Annotation
	err = json.Unmarshal(j, &a)
	if err != nil {
		t.Fatal(err)
	}

	return a
}
