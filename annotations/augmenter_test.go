package annotations

import (
	"context"
	"errors"
	"github.com/Financial-Times/draft-annotations-api/concept"
	tidUtils "github.com/Financial-Times/transactionid-utils-go"
	log "github.com/sirupsen/logrus"
	logTest "github.com/sirupsen/logrus/hooks/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"testing"
)

func buildTestAnnotations() []*Annotation {
	return []*Annotation{
		{
			Predicate: "http://www.ft.com/ontology/classification/isClassifiedBy",
			ConceptId: "http://api.ft.com/things/b224ad07-c818-3ad6-94af-a4d351dbb619",
		},
		{
			Predicate: "http://www.ft.com/ontology/annotation/mentions",
			ConceptId: "http://api.ft.com/things/1a2a1a0a-7199-38b8-8a73-e651e2172471",
		},
		{
			Predicate: "http://www.ft.com/ontology/hasContributor",
			ConceptId: "http://api.ft.com/things/5bd49568-6d7c-3c10-a5b0-2f3fd5974a6b",
		},
	}
}

var testConceptIDs = []string{
	"b224ad07-c818-3ad6-94af-a4d351dbb619",
	"1a2a1a0a-7199-38b8-8a73-e651e2172471",
	"5bd49568-6d7c-3c10-a5b0-2f3fd5974a6b",
}

var testConcepts = map[string]concept.Concept{
	"b224ad07-c818-3ad6-94af-a4d351dbb619": {
		Id:     "b224ad07-c818-3ad6-94af-a4d351dbb619",
		ApiUrl: "http://api.ft.com/things/b224ad07-c818-3ad6-94af-a4d351dbb619",
		Types: []string{
			"http://www.ft.com/ontology/core/Thing",
			"http://www.ft.com/ontology/concept/Concept",
			"http://www.ft.com/ontology/classification/Classification",
			"http://www.ft.com/ontology/Subject",
		},
		PrefLabel: "Economic Indicators",
	},
	"1a2a1a0a-7199-38b8-8a73-e651e2172471": {
		Id:     "1a2a1a0a-7199-38b8-8a73-e651e2172471",
		ApiUrl: "http://api.ft.com/things/1a2a1a0a-7199-38b8-8a73-e651e2172471",
		Types: []string{
			"http://www.ft.com/ontology/core/Thing",
			"http://www.ft.com/ontology/concept/Concept",
			"http://www.ft.com/ontology/Location",
		},
		PrefLabel: "United Kingdom",
	},
	"5bd49568-6d7c-3c10-a5b0-2f3fd5974a6b": {
		Id:     "5bd49568-6d7c-3c10-a5b0-2f3fd5974a6b",
		ApiUrl: "http://api.ft.com/things/5bd49568-6d7c-3c10-a5b0-2f3fd5974a6b",
		Types: []string{
			"http://www.ft.com/ontology/core/Thing",
			"http://www.ft.com/ontology/concept/Concept",
			"http://www.ft.com/ontology/person/Person",
		},
		PrefLabel:  "Lisa Barrett",
		IsFTAuthor: true,
	},
}

var expectedAugmentedAnnotations = []*Annotation{
	{
		Predicate: "http://www.ft.com/ontology/classification/isClassifiedBy",
		ConceptId: "http://api.ft.com/things/b224ad07-c818-3ad6-94af-a4d351dbb619",
		ApiUrl:    "http://api.ft.com/things/b224ad07-c818-3ad6-94af-a4d351dbb619",
		Types: []string{
			"http://www.ft.com/ontology/core/Thing",
			"http://www.ft.com/ontology/concept/Concept",
			"http://www.ft.com/ontology/classification/Classification",
			"http://www.ft.com/ontology/Subject",
		},
		PrefLabel:  "Economic Indicators",
		IsFTAuthor: false,
	},
	{
		Predicate: "http://www.ft.com/ontology/annotation/mentions",
		ConceptId: "http://api.ft.com/things/1a2a1a0a-7199-38b8-8a73-e651e2172471",
		ApiUrl:    "http://api.ft.com/things/1a2a1a0a-7199-38b8-8a73-e651e2172471",
		Types: []string{
			"http://www.ft.com/ontology/core/Thing",
			"http://www.ft.com/ontology/concept/Concept",
			"http://www.ft.com/ontology/Location",
		},
		PrefLabel:  "United Kingdom",
		IsFTAuthor: false,
	},
	{
		Predicate: "http://www.ft.com/ontology/hasContributor",
		ConceptId: "http://api.ft.com/things/5bd49568-6d7c-3c10-a5b0-2f3fd5974a6b",
		ApiUrl:    "http://api.ft.com/things/5bd49568-6d7c-3c10-a5b0-2f3fd5974a6b",
		Types: []string{
			"http://www.ft.com/ontology/core/Thing",
			"http://www.ft.com/ontology/concept/Concept",
			"http://www.ft.com/ontology/person/Person",
		},
		PrefLabel:  "Lisa Barrett",
		IsFTAuthor: true,
	},
}

func TestAugmentAnnotations(t *testing.T) {
	conceptsSearchAPI := new(ConceptSearchAPIMock)
	ctx := tidUtils.TransactionAwareContext(context.Background(), tidUtils.NewTransactionID())
	conceptsSearchAPI.
		On("SearchConcepts", ctx, testConceptIDs).
		Return(testConcepts, nil)
	a := NewAugmenter(conceptsSearchAPI)

	annotations := buildTestAnnotations()
	err := a.AugmentAnnotations(ctx, &annotations)

	assert.NoError(t, err)
	assert.Equal(t, len(expectedAugmentedAnnotations), len(annotations))
	assert.Equal(t, expectedAugmentedAnnotations, annotations)

	conceptsSearchAPI.AssertExpectations(t)
}

func TestAugmentAnnotationsMissingTransactionID(t *testing.T) {
	hook := logTest.NewGlobal()
	conceptsSearchAPI := new(ConceptSearchAPIMock)
	conceptsSearchAPI.
		On("SearchConcepts", mock.Anything, testConceptIDs).
		Return(testConcepts, nil)
	a := NewAugmenter(conceptsSearchAPI)

	annotations := buildTestAnnotations()
	a.AugmentAnnotations(context.Background(), &annotations)

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

	conceptsSearchAPI.AssertExpectations(t)
}

func TestAugmentAnnotationsInvalidConceptID(t *testing.T) {
	conceptsSearchAPI := new(ConceptSearchAPIMock)
	conceptsSearchAPI.
		On("SearchConcepts", mock.Anything, []string{"b224ad07-c818-3ad6-94af-a4d351dbb619", "invalid-id"}).
		Return(testConcepts, nil)
	a := NewAugmenter(conceptsSearchAPI)

	tid := tidUtils.NewTransactionID()
	ctx := tidUtils.TransactionAwareContext(context.Background(), tid)

	annotations := buildTestAnnotations()
	annotations[1].ConceptId = "invalid-id"
	annotations[2].ConceptId = "http://api.ft.com/things/invalid-id"

	err := a.AugmentAnnotations(ctx, &annotations)

	assert.NoError(t, err)
	assert.Len(t, annotations, 3)

	for i, ann := range annotations {
		if i == 0 {
			assert.Equal(t, expectedAugmentedAnnotations[i], ann)
		} else {
			assert.Empty(t, ann.PrefLabel)
			assert.Empty(t, ann.Types)
			assert.Empty(t, ann.ApiUrl)
			assert.Empty(t, ann.IsFTAuthor)
		}
	}

	conceptsSearchAPI.AssertExpectations(t)
}

func TestAugmentAnnotationsConceptSearchError(t *testing.T) {
	conceptsSearchAPI := new(ConceptSearchAPIMock)
	ctx := tidUtils.TransactionAwareContext(context.Background(), tidUtils.NewTransactionID())
	conceptsSearchAPI.
		On("SearchConcepts", ctx, testConceptIDs).
		Return(map[string]concept.Concept{}, errors.New("one minute to midnight"))
	a := NewAugmenter(conceptsSearchAPI)

	annotations := buildTestAnnotations()
	err := a.AugmentAnnotations(ctx, &annotations)

	assert.Error(t, err)

	conceptsSearchAPI.AssertExpectations(t)
}

type ConceptSearchAPIMock struct {
	mock.Mock
}

func (m *ConceptSearchAPIMock) SearchConcepts(ctx context.Context, ids []string) (map[string]concept.Concept, error) {
	args := m.Called(ctx, ids)
	return args.Get(0).(map[string]concept.Concept), args.Error(1)
}

func (m *ConceptSearchAPIMock) GTG() error {
	args := m.Called()
	return args.Error(0)
}

func (m *ConceptSearchAPIMock) Endpoint() string {
	args := m.Called()
	return args.String(0)
}
