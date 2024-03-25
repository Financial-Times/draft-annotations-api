package annotations

import (
	"context"
	"encoding/json"
	"maps"
	"strings"

	"github.com/Financial-Times/go-logger/v2"

	"github.com/Financial-Times/draft-annotations-api/concept"
	"github.com/Financial-Times/draft-annotations-api/mapper"
	tidUtils "github.com/Financial-Times/transactionid-utils-go"
)

type OriginSystemIDHeaderKey string

const (
	OriginSystemIDHeader = "X-Origin-System-Id"
	PACOriginSystemID    = "http://cmdb.ft.com/systems/pac"
)

type Augmenter struct {
	conceptRead concept.ReadAPI
	log         *logger.UPPLogger
}

func NewAugmenter(api concept.ReadAPI, log *logger.UPPLogger) *Augmenter {
	return &Augmenter{conceptRead: api, log: log}
}

func (a *Augmenter) AugmentAnnotations(ctx context.Context, canonicalAnnotations []interface{}) ([]interface{}, error) {
	tid, err := tidUtils.GetTransactionIDFromContext(ctx)

	if err != nil {
		tid = tidUtils.NewTransactionID()
		a.log.WithTransactionID(tid).
			WithError(err).
			Warn("Transaction ID error in augmenting annotations with concept data: Generated a new transaction ID")
		ctx = tidUtils.TransactionAwareContext(ctx, tid)
	}

	dedupedCanonical, err := dedupeCanonicalAnnotations(canonicalAnnotations)
	if err != nil {
		a.log.WithTransactionID(tid).
			WithError(err).Error("Request failed when attempting to dedupе annotations")
		return nil, err
	}

	origin := ctx.Value(OriginSystemIDHeaderKey(OriginSystemIDHeader)).(string)
	if origin == PACOriginSystemID {
		dedupedCanonical = filterOutInvalidPredicates(dedupedCanonical)
	}

	uuids := getConceptUUIDs(dedupedCanonical)

	concepts, err := a.conceptRead.GetConceptsByIDs(ctx, uuids)

	if err != nil {
		a.log.WithTransactionID(tid).
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
			a.log.WithTransactionID(tid).
				WithUUID(ann["id"].(string)).
				Warn("Concept data for this annotation was not found, and will be removed from the list of annotations.")
		}
	}

	a.log.WithTransactionID(tid).Info("Annotations augmented with concept data")
	return augmentedAnnotations, nil
}

func dedupeCanonicalAnnotations(annotations []interface{}) ([]interface{}, error) {
	var deduped []interface{}
	dedupedMap := make(map[string]bool)

	for _, ann := range annotations {
		jsonAnn, err := json.Marshal(ann)
		if err != nil {
			return nil, err
		}
		jsonAnnStr := string(jsonAnn)

		if _, exists := dedupedMap[jsonAnnStr]; !exists {
			dedupedMap[jsonAnnStr] = true
			deduped = append(deduped, ann)
		}
	}

	return deduped, nil
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
