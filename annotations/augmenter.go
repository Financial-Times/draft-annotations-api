package annotations

import (
	"context"
	"errors"
	"github.com/Financial-Times/draft-annotations-api/concept"
	tidUtils "github.com/Financial-Times/transactionid-utils-go"
	log "github.com/sirupsen/logrus"
	"strings"
)

type Augmenter interface {
	AugmentAnnotations(ctx context.Context, depletedAnnotations *[]*Annotation) error
}

type annotationAugmenter struct {
	conceptSearchApi concept.SearchAPI
}

func NewAugmenter(api concept.SearchAPI) *annotationAugmenter {
	return &annotationAugmenter{api}
}

func (a *annotationAugmenter) AugmentAnnotations(ctx context.Context, annotations *[]*Annotation) error {
	tid, err := tidUtils.GetTransactionIDFromContext(ctx)

	if err != nil {
		tid = tidUtils.NewTransactionID()
		log.WithField(tidUtils.TransactionIDKey, tid).
			WithError(err).
			Warn("Transaction ID error in augmenting annotations with concept data: Generated a new transaction ID")
		ctx = tidUtils.TransactionAwareContext(ctx, tid)
	}

	var conceptIds []string

	for _, ann := range *annotations {
		conceptUUID, err := extractUUID(ann.ConceptId)
		if err != nil {
			log.WithField(tidUtils.TransactionIDKey, tid).
				WithField("conceptID", ann.ConceptId).
				WithError(err).Warn("Error in augmenting annotation with concept data")
		} else {
			conceptIds = append(conceptIds, conceptUUID)
		}
	}

	concepts, err := a.conceptSearchApi.SearchConcepts(ctx, conceptIds)

	if err != nil {
		log.WithField(tidUtils.TransactionIDKey, tid).
			WithError(err).Error("Error in augmenting annotation with concept data")
		return err
	}

	for _, ann := range *annotations {
		conceptUUID, _ := extractUUID(ann.ConceptId)
		concept, found := concepts[conceptUUID]
		if found {
			ann.ApiUrl = concept.ApiUrl
			ann.PrefLabel = concept.PrefLabel
			ann.IsFTAuthor = concept.IsFTAuthor
			ann.Types = concept.Types
		}
	}

	log.WithField(tidUtils.TransactionIDKey, tid).Info("Annotations augmented with concept data")
	return nil
}

func extractUUID(conceptURI string) (string, error) {
	i := strings.LastIndex(conceptURI, "/")
	if i == -1 || i == len(conceptURI)-1 {
		return "", errors.New("impossible to extract UUID from concept URI")
	}
	return conceptURI[i+1:], nil
}
