package annotations

import (
	"context"
	"testing"

	"github.com/satori/go.uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

const (
	mentions = "http://www.ft.com/ontology/mentions"
)

func assertedBrandAnnotation() Annotation {
	conceptUuid := uuid.NewV4().String()
	conceptId := "http://www.ft.com/thing/" + conceptUuid
	return Annotation{
		Predicate: isClassifiedBy,
		ConceptId: conceptId,
		ApiUrl:    conceptId,
		Types: []string{
			"http://www.ft.com/ontology/core/Thing",
			"http://www.ft.com/ontology/concept/Concept",
			"http://www.ft.com/ontology/classification/Classification",
			"http://www.ft.com/ontology/product/Brand",
		},
		PrefLabel: "A brand",
	}
}

func mentionsAnnotation() Annotation {
	conceptUuid := uuid.NewV4().String()
	conceptId := "http://www.ft.com/thing/" + conceptUuid
	return Annotation{
		Predicate: mentions,
		ConceptId: conceptId,
		ApiUrl:    conceptId,
		Types: []string{
			"http://www.ft.com/ontology/core/Thing",
			"http://www.ft.com/ontology/concept/Concept",
			"http://www.ft.com/ontology/person/Person",
		},
		PrefLabel: "John Doe",
	}
}

func impliedBrandAnnotation() Annotation {
	conceptUuid := uuid.NewV4().String()
	conceptId := "http://www.ft.com/thing/" + conceptUuid
	return Annotation{
		Predicate: implicitlyClassifiedBy,
		ConceptId: conceptId,
		ApiUrl:    conceptId,
		Types: []string{
			"http://www.ft.com/ontology/core/Thing",
			"http://www.ft.com/ontology/concept/Concept",
			"http://www.ft.com/ontology/classification/Classification",
			"http://www.ft.com/ontology/product/Brand",
		},
		PrefLabel: "An implied brand",
	}
}

func TestRemoveRule(t *testing.T) {
	rule := NewRemoveRule([]string{implicitlyClassifiedBy})

	ann1 := assertedBrandAnnotation()
	ann2 := impliedBrandAnnotation()

	actual, err := rule.Apply(context.Background(), []Annotation{ann1, ann2})
	assert.NoError(t, err)

	assert.Len(t, actual, 1)
	assert.Equal(t, ann1, actual[0], "remaining annotation")
}

type mockBrandsResolver struct {
	mock.Mock
}

func (m *mockBrandsResolver) Refresh(brandUuids []string) {
	m.Called(brandUuids)
}

func (m *mockBrandsResolver) GetBrands(brand string) []string {
	args := m.Called(brand)

	return args.Get(0).([]string)
}

func TestImplicitBrandsRuleWithAncestors(t *testing.T) {
	ann1 := assertedBrandAnnotation()

	parentBrandUuid := uuid.NewV4().String()
	parentBrandId := "http://www.ft.com/thing/" + parentBrandUuid

	grandparentBrandUuid := uuid.NewV4().String()
	grandparentBrandId := "http://www.ft.com/thing/" + grandparentBrandUuid

	brandsResolver := &mockBrandsResolver{}
	brandsResolver.On("GetBrands", ann1.ConceptId).Return([]string{ann1.ConceptId, parentBrandId, grandparentBrandId}, nil)
	rule := NewImplicitBrandsRule([]string{isClassifiedBy}, implicitlyClassifiedBy, []string{}, brandsResolver)

	ann2 := mentionsAnnotation()

	actual, err := rule.Apply(context.Background(), []Annotation{ann1, ann2})
	assert.NoError(t, err)

	assert.Len(t, actual, 4, "annotations")

	for _, ann := range actual {
		switch ann.ConceptId {
		case ann1.ConceptId:
			assert.Equal(t, isClassifiedBy, ann.Predicate, "predicate for originating brand")
		case parentBrandId:
			fallthrough
		case grandparentBrandId:
			assert.Equal(t, implicitlyClassifiedBy, ann.Predicate, "predicate for implied brand")
		case ann2.ConceptId:
			assert.Equal(t, mentions, ann.Predicate,"predicate for other annotation")
		default:
			assert.Fail(t, "unexpected concept in annotation", ann.ConceptId)
		}
	}

	brandsResolver.AssertExpectations(t)
}

func TestImplicitBrandsRuleNoAncestors(t *testing.T) {
	ann1 := assertedBrandAnnotation()

	brandsResolver := &mockBrandsResolver{}
	brandsResolver.On("GetBrands", ann1.ConceptId).Return([]string{ann1.ConceptId}, nil)
	rule := NewImplicitBrandsRule([]string{isClassifiedBy}, implicitlyClassifiedBy, []string{}, brandsResolver)

	ann2 := mentionsAnnotation()

	actual, err := rule.Apply(context.Background(), []Annotation{ann1, ann2})
	assert.NoError(t, err)

	assert.Len(t, actual, 2, "annotations")

	for _, ann := range actual {
		switch ann.ConceptId {
		case ann1.ConceptId:
			assert.Equal(t, isClassifiedBy, ann.Predicate, "predicate for originating brand")
		case ann2.ConceptId:
			assert.Equal(t, mentions, ann.Predicate,"predicate for other annotation")
		default:
			assert.Fail(t, "unexpected concept in annotation", ann.ConceptId)
		}
	}

	brandsResolver.AssertExpectations(t)
}

func TestImplicitBrandsRuleWithExclusion(t *testing.T) {
	ann1 := assertedBrandAnnotation()

	parentBrandUuid := uuid.NewV4().String()
	parentBrandId := "http://www.ft.com/thing/" + parentBrandUuid

	brandsResolver := &mockBrandsResolver{}
	brandsResolver.On("GetBrands", ann1.ConceptId).Return([]string{ann1.ConceptId, parentBrandId}, nil)

	ann2 := mentionsAnnotation()
	ann3 := assertedBrandAnnotation()

	rule := NewImplicitBrandsRule([]string{isClassifiedBy}, implicitlyClassifiedBy, []string{ann3.ConceptId}, brandsResolver)

	actual, err := rule.Apply(context.Background(), []Annotation{ann1, ann2, ann3})
	assert.NoError(t, err)

	assert.Len(t, actual, 4, "annotations")

	for _, ann := range actual {
		switch ann.ConceptId {
		case ann1.ConceptId:
			fallthrough
		case ann3.ConceptId:
			assert.Equal(t, isClassifiedBy, ann.Predicate, "predicate for originating brand")
		case parentBrandId:
			assert.Equal(t, implicitlyClassifiedBy, ann.Predicate, "predicate for implied brand")
		case ann2.ConceptId:
			assert.Equal(t, mentions, ann.Predicate,"predicate for other annotation")
		default:
			assert.Fail(t, "unexpected concept in annotation", ann.ConceptId)
		}
	}

	brandsResolver.AssertExpectations(t)
}
