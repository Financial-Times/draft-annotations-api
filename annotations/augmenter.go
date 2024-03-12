package annotations

import (
	"context"
	"maps"
	"reflect"
	"strings"

	"github.com/Financial-Times/draft-annotations-api/concept"
	"github.com/Financial-Times/draft-annotations-api/mapper"
	tidUtils "github.com/Financial-Times/transactionid-utils-go"
	log "github.com/sirupsen/logrus"
)

type OriginSystemIDHeaderKey string

const (
	OriginSystemIDHeader = "X-Origin-System-Id"
	PACOriginSystemID    = "http://cmdb.ft.com/systems/pac"
)

type Augmenter struct {
	conceptRead concept.ReadAPI
}

func NewAugmenter(api concept.ReadAPI) *Augmenter {
	return &Augmenter{api}
}

func (a *Augmenter) AugmentAnnotations(ctx context.Context, canonicalAnnotations []interface{}) ([]interface{}, error) {
	tid, err := tidUtils.GetTransactionIDFromContext(ctx)

	if err != nil {
		tid = tidUtils.NewTransactionID()
		log.WithField(tidUtils.TransactionIDKey, tid).
			WithError(err).
			Warn("Transaction ID error in augmenting annotations with concept data: Generated a new transaction ID")
		ctx = tidUtils.TransactionAwareContext(ctx, tid)
	}

	dedupedCanonical := dedupeCanonicalAnnotations(canonicalAnnotations)

	origin := ctx.Value(OriginSystemIDHeaderKey(OriginSystemIDHeader)).(string)
	if origin == PACOriginSystemID {
		dedupedCanonical = filterOutInvalidPredicates(dedupedCanonical)
	}

	uuids := getConceptUUIDs(dedupedCanonical)

	concepts, err := a.conceptRead.GetConceptsByIDs(ctx, uuids)

	if err != nil {
		log.WithField(tidUtils.TransactionIDKey, tid).
			WithError(err).Error("Request failed when attempting to augment annotations from UPP concept data")
		return nil, err
	}

	augmentedAnnotations := make([]interface{}, 0)
	for _, annotation := range dedupedCanonical {
		ann := make(map[string]interface{})
		maps.Copy(ann, annotation.(map[string]interface{}))
		uuid := extractUUID(ann["id"].(string))
		concept, found := concepts[uuid]
		if found {
			ann["id"] = concept.ID
			ann["apiUrl"] = concept.ApiUrl
			ann["prefLabel"] = concept.PrefLabel
			ann["isFTAuthor"] = concept.IsFTAuthor
			ann["type"] = concept.Type
			augmentedAnnotations = append(augmentedAnnotations, ann)
		} else {
			log.WithField(tidUtils.TransactionIDKey, tid).
				WithField("conceptId", ann["id"]).
				Warn("Concept data for this annotation was not found, and will be removed from the list of annotations.")
		}
	}

	log.WithField(tidUtils.TransactionIDKey, tid).Info("Annotations augmented with concept data")
	return augmentedAnnotations, nil
}

func dedupeCanonicalAnnotations(annotations []interface{}) []interface{} {
	deduped := make([]interface{}, 0)

	for i := 0; i < len(annotations); i++ {
		duplicate := false
		for j := 0; j < len(deduped); j++ {
			if reflect.DeepEqual(annotations[i], deduped[j]) {
				duplicate = true
				break
			}
		}
		if !duplicate {
			deduped = append(deduped, annotations[i])
		}
	}

	return deduped
}

func filterOutInvalidPredicates(annotations []interface{}) []interface{} {
	i := 0
	for _, item := range annotations {
		if !mapper.IsValidPACPredicate(item.(map[string]interface{})["predicate"].(string)) {
			continue
		}
		annotations[i] = item
		i++
	}

	return annotations[:i]
}

func getConceptUUIDs(canonicalAnnotations []interface{}) []string {
	conceptUUIDs := make(map[string]struct{})
	var empty struct{}
	var keys []string
	for _, ann := range canonicalAnnotations {
		conceptUUID := extractUUID(ann.(map[string]interface{})["id"].(string))
		if conceptUUID != "" {
			conceptUUIDs[conceptUUID] = empty
		}
	}
	for k := range conceptUUIDs {
		keys = append(keys, k)

	}
	return keys
}

func extractUUID(conceptID string) string {
	i := strings.LastIndex(conceptID, "/")
	if i == -1 || i == len(conceptID)-1 {
		return ""
	}
	return conceptID[i+1:]
}
