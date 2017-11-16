package annotations

import (
	"context"
	"errors"
	"testing"

	"github.com/Financial-Times/draft-annotations-api/concept"
	tidUtils "github.com/Financial-Times/transactionid-utils-go"
	log "github.com/sirupsen/logrus"
	logTest "github.com/sirupsen/logrus/hooks/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func buildTestAnnotations() []*Annotation {
	return []*Annotation{
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
	}
}

var testConceptIDs = []string{
	"b224ad07-c818-3ad6-94af-a4d351dbb619",
	"1a2a1a0a-7199-38b8-8a73-e651e2172471",
	"5bd49568-6d7c-3c10-a5b0-2f3fd5974a6b",
	"1fb3faf1-bf00-3a15-8efb-1038a59653f7",
}

var testConcepts = map[string]concept.Concept{
	"http://www.ft.com/thing/b224ad07-c818-3ad6-94af-a4d351dbb619": {
		Id:        "http://www.ft.com/thing/b224ad07-c818-3ad6-94af-a4d351dbb619",
		ApiUrl:    "http://api.ft.com/things/b224ad07-c818-3ad6-94af-a4d351dbb619",
		Type:      "http://www.ft.com/ontology/Subject",
		PrefLabel: "Economic Indicators",
	},
	"http://www.ft.com/thing/5bd49568-6d7c-3c10-a5b0-2f3fd5974a6b": {
		Id:         "http://www.ft.com/thing/5bd49568-6d7c-3c10-a5b0-2f3fd5974a6b",
		ApiUrl:     "http://api.ft.com/things/5bd49568-6d7c-3c10-a5b0-2f3fd5974a6b",
		Type:       "http://www.ft.com/ontology/person/Person",
		PrefLabel:  "Lisa Barrett",
		IsFTAuthor: true,
	},
}

var expectedAugmentedAnnotations = []*Annotation{
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
}

func TestAugmentAnnotations(t *testing.T) {
	conceptsSearchAPI := new(ConceptSearchAPIMock)
	ctx := tidUtils.TransactionAwareContext(context.Background(), tidUtils.NewTransactionID())
	conceptsSearchAPI.
		On("SearchConcepts", ctx, testConceptIDs).
		Return(testConcepts, nil)
	a := NewAugmenter(conceptsSearchAPI)

	annotations := buildTestAnnotations()
	annotations, err := a.AugmentAnnotations(ctx, annotations)

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
	a.AugmentAnnotations(context.Background(), annotations)

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

func TestAugmentAnnotationsConceptSearchError(t *testing.T) {
	conceptsSearchAPI := new(ConceptSearchAPIMock)
	ctx := tidUtils.TransactionAwareContext(context.Background(), tidUtils.NewTransactionID())
	conceptsSearchAPI.
		On("SearchConcepts", ctx, testConceptIDs).
		Return(map[string]concept.Concept{}, errors.New("one minute to midnight"))
	a := NewAugmenter(conceptsSearchAPI)

	annotations := buildTestAnnotations()
	annotations, err := a.AugmentAnnotations(ctx, annotations)

	assert.Error(t, err)

	conceptsSearchAPI.AssertExpectations(t)
}

func TestAugmentAnnotationsWithInvalidConceptID(t *testing.T) {
	conceptsSearchAPI := new(ConceptSearchAPIMock)
	ctx := tidUtils.TransactionAwareContext(context.Background(), tidUtils.NewTransactionID())
	conceptsSearchAPI.
		On("SearchConcepts", ctx, testConceptIDs).
		Return(testConcepts, nil)
	a := NewAugmenter(conceptsSearchAPI)

	annotations := buildTestAnnotations()
	annotations = append(annotations, &Annotation{ConceptId: "xyz"})
	annotations, err := a.AugmentAnnotations(ctx, annotations)

	assert.NoError(t, err)
	assert.Equal(t, len(expectedAugmentedAnnotations), len(annotations))
	assert.Equal(t, expectedAugmentedAnnotations, annotations)

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
