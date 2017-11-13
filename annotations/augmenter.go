package annotations

import (
	"context"
	"github.com/Financial-Times/draft-annotations-api/concept"
	modelMapper "github.com/Financial-Times/neo-model-utils-go/mapper"
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
		conceptIds = append(conceptIds, ann.ConceptId)
	}

	concepts, err := a.conceptSearchApi.SearchConcepts(ctx, conceptIds)

	if err != nil {
		log.WithField(tidUtils.TransactionIDKey, tid).
			WithError(err).Error("Error in augmenting annotation with concept data")
		return err
	}

	for _, ann := range *annotations {
		concept, found := concepts["http://www.ft.com/thing/"+ann.ConceptId]
		if found {
			ann.ApiUrl = concept.ApiUrl
			ann.PrefLabel = concept.PrefLabel
			ann.IsFTAuthor = concept.IsFTAuthor
			ann.Types = buildTypeHierarchy(concept.Type)
		} else {
			log.WithField(tidUtils.TransactionIDKey, tid).
				WithField("conceptId", ann.ConceptId).
				Warn("Information not found for augmenting concept")
		}
		ann.ConceptId = modelMapper.IDURL(ann.ConceptId)
	}

	log.WithField(tidUtils.TransactionIDKey, tid).Info("Annotations augmented with concept data")
	return nil
}

func buildTypeHierarchy(t string) []string {
	i := strings.LastIndex(t, "/")
	if i == -1 || i == len(t)-1 {
		return nil
	}
	t = t[i+1:]
	var types []string
	for t != "" {
		types = append(types, t)
		t = modelMapper.ParentType(t)
	}
	types, err := modelMapper.SortTypes(types)
	if err != nil {
		return nil
	}
	return modelMapper.TypeURIs(types)
}
