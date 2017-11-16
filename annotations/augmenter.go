package annotations

import (
	"context"

	"github.com/Financial-Times/draft-annotations-api/concept"
	tidUtils "github.com/Financial-Times/transactionid-utils-go"
	log "github.com/sirupsen/logrus"
)

type Augmenter interface {
	AugmentAnnotations(ctx context.Context, depletedAnnotations []*Annotation) ([]*Annotation, error)
}

type annotationAugmenter struct {
	conceptSearchApi concept.SearchAPI
}

func NewAugmenter(api concept.SearchAPI) *annotationAugmenter {
	return &annotationAugmenter{api}
}

func (a *annotationAugmenter) AugmentAnnotations(ctx context.Context, depletedAnnotations []*Annotation) ([]*Annotation, error) {
	tid, err := tidUtils.GetTransactionIDFromContext(ctx)

	if err != nil {
		tid = tidUtils.NewTransactionID()
		log.WithField(tidUtils.TransactionIDKey, tid).
			WithError(err).
			Warn("Transaction ID error in augmenting annotations with concept data: Generated a new transaction ID")
		ctx = tidUtils.TransactionAwareContext(ctx, tid)
	}

	var conceptIds []string

	for _, ann := range depletedAnnotations {
		conceptIds = append(conceptIds, ann.ConceptId)
	}

	concepts, err := a.conceptSearchApi.SearchConcepts(ctx, conceptIds)

	if err != nil {
		log.WithField(tidUtils.TransactionIDKey, tid).
			WithError(err).Error("Error in augmenting annotation with concept data")
		return nil, err
	}

	var augmentedAnnotations []*Annotation
	for _, ann := range depletedAnnotations {
		ann.ConceptId = "http://www.ft.com/thing/" + ann.ConceptId
		c, found := concepts[ann.ConceptId]
		if found {
			ann.ApiUrl = c.ApiUrl
			ann.PrefLabel = c.PrefLabel
			ann.IsFTAuthor = c.IsFTAuthor
			ann.Type = c.Type
			augmentedAnnotations = append(augmentedAnnotations, ann)
		} else {
			log.WithField(tidUtils.TransactionIDKey, tid).
				WithField("conceptId", ann.ConceptId).
				Error("Information not found for augmenting concept")
		}
	}

	log.WithField(tidUtils.TransactionIDKey, tid).Info("Annotations augmented with concept data")
	return augmentedAnnotations, nil
}
