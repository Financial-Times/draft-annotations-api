package annotations

import (
	"context"
	"strings"

	"github.com/Financial-Times/draft-annotations-api/concept"
	tidUtils "github.com/Financial-Times/transactionid-utils-go"
	log "github.com/sirupsen/logrus"
)

type Augmenter interface {
	AugmentAnnotations(ctx context.Context, depletedAnnotations []Annotation) ([]Annotation, error)
}

type annotationAugmenter struct {
	conceptSearchApi concept.SearchAPI
}

func NewAugmenter(api concept.SearchAPI) *annotationAugmenter {
	return &annotationAugmenter{api}
}

func (a *annotationAugmenter) AugmentAnnotations(ctx context.Context, canonicalAnnotations []Annotation) ([]Annotation, error) {
	tid, err := tidUtils.GetTransactionIDFromContext(ctx)

	if err != nil {
		tid = tidUtils.NewTransactionID()
		log.WithField(tidUtils.TransactionIDKey, tid).
			WithError(err).
			Warn("Transaction ID error in augmenting annotations with concept data: Generated a new transaction ID")
		ctx = tidUtils.TransactionAwareContext(ctx, tid)
	}

	uuids, mappedAnnotations := toMap(canonicalAnnotations)

	concepts, err := a.conceptSearchApi.SearchConcepts(ctx, uuids)

	if err != nil {
		log.WithField(tidUtils.TransactionIDKey, tid).
			WithError(err).Error("Request failed when attempting to augment annotations from UPP concept data")
		return nil, err
	}

	var augmentedAnnotations []Annotation
	for uuid, ann := range mappedAnnotations {
		concept, found := concepts[uuid]
		if found {
			ann.ConceptId = concept.ID
			ann.ApiUrl = concept.ApiUrl
			ann.PrefLabel = concept.PrefLabel
			ann.IsFTAuthor = concept.IsFTAuthor
			ann.Type = concept.Type
			augmentedAnnotations = append(augmentedAnnotations, ann)
		} else {
			log.WithField(tidUtils.TransactionIDKey, tid).
				WithField("conceptId", ann.ConceptId).
				Warn("Concept data for this annotation was not found, and will be removed from the list of annotations.")
		}
	}

	log.WithField(tidUtils.TransactionIDKey, tid).Info("Annotations augmented with concept data")
	return augmentedAnnotations, nil
}

func toMap(canonicalAnnotations []Annotation) ([]string, map[string]Annotation) {
	mappedConcepts := make(map[string]Annotation)
	var keys []string
	for _, ann := range canonicalAnnotations {
		conceptUUID := extractUUID(ann.ConceptId)
		if conceptUUID != "" {
			keys = append(keys, conceptUUID)
			mappedConcepts[conceptUUID] = ann
		}
	}
	return keys, mappedConcepts
}

func extractUUID(conceptID string) string {
	i := strings.LastIndex(conceptID, "/")
	if i == -1 || i == len(conceptID)-1 {
		return ""
	}
	return conceptID[i+1:]
}
