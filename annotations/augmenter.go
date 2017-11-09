package annotations

import (
	"context"
	"github.com/Financial-Times/draft-annotations-api/concept"
)

type Augmenter interface {
	augmentAnnotations(ctx context.Context, depletedAnnotations []Annotation) ([]Annotation, error)
}

type annotationAugmenter struct {
	conceptSearchApi concept.SearchAPI
}

func NewAugmenter(api concept.SearchAPI) *annotationAugmenter {
	return &annotationAugmenter{api}
}

func (a *annotationAugmenter) augmentAnnotations(ctx context.Context, depletedAnnotations []Annotation) ([]Annotation, error) {
	augmentedAnnotations := []Annotation{}

	conceptIds := make([]string, len(depletedAnnotations))

	for _, ann := range depletedAnnotations {
		conceptIds = append(conceptIds, ann.ConceptId)
	}

	concepts, err := a.conceptSearchApi.SearchConcepts(ctx, conceptIds)

	if err != nil {
		return nil, err
	}

	for _, ann := range depletedAnnotations {
		concept := concepts[ann.ConceptId]
		ann.PrefLabel = concept.PrefLabel
		ann.IsFTAuthor = concept.IsFTAuthor
		ann.Types = concept.Types
		augmentedAnnotations = append(augmentedAnnotations, ann)
	}

	return augmentedAnnotations, nil
}
