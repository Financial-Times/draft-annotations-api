package mapper

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	log "github.com/sirupsen/logrus"
)

const (
	PredicateIsClassifiedBy          = "http://www.ft.com/ontology/classification/isClassifiedBy"
	PredicateIsPrimarilyClassifiedBy = "http://www.ft.com/ontology/classification/isPrimarilyClassifiedBy"
	PredicateMentions                = "http://www.ft.com/ontology/annotation/mentions"
	PredicateMajorMentions           = "http://www.ft.com/ontology/annotation/majorMentions"
	PredicateAbout                   = "http://www.ft.com/ontology/annotation/about"
	PredicateImplicitlyAbout         = "http://www.ft.com/ontology/implicitlyAbout"
	PredicateImplicitlyClassifiedBy  = "http://www.ft.com/ontology/implicitlyClassifiedBy"
	PredicateHasAuthor               = "http://www.ft.com/ontology/annotation/hasAuthor"
	PredicateHasBrand                = "http://www.ft.com/ontology/hasBrand"
	PredicateHasContributor          = "http://www.ft.com/ontology/hasContributor"
	PredicateHasDisplayTag           = "http://www.ft.com/ontology/hasDisplayTag"

	ConceptTypeBrand         = "http://www.ft.com/ontology/product/Brand"
	ConceptTypeGenre         = "http://www.ft.com/ontology/Genre"
	ConceptTypeTopic         = "http://www.ft.com/ontology/Topic"
	ConceptTypeLocation      = "http://www.ft.com/ontology/Location"
	ConceptTypeSpecialReport = "http://www.ft.com/ontology/SpecialReport"
	ConceptTypeSubject       = "http://www.ft.com/ontology/Subject"
)

func ConvertPredicates(body []byte) ([]byte, error) {
	originalAnnotations := make([]map[string]interface{}, 0)
	convertedAnnotations := make([]map[string]interface{}, 0)
	err := json.Unmarshal(body, &originalAnnotations)
	if err != nil {
		return nil, fmt.Errorf("could not unmarshal json body:%w", err)
	}

	for _, annoMap := range originalAnnotations {

		pred, ok := annoMap["predicate"]
		if !ok {
			log.Info("no predicate supplied for incoming annotation")
			continue
		}
		predicate := pred.(string)
		someTypes, ok := annoMap["types"]
		if !ok {
			log.Info("no types supplied for incoming annotation")
			continue
		}

		annoMap["id"] = TransformConceptID(annoMap["id"].(string))

		stringTypes, _ := toStringArray(someTypes)
		conceptType := getLeafType(stringTypes)

		annoMap["type"] = conceptType
		delete(annoMap, "types")

		if conceptType == ConceptTypeSpecialReport || conceptType == ConceptTypeSubject {
			continue
		}

		switch predicate {
		case PredicateIsClassifiedBy:
			if conceptType == ConceptTypeTopic || conceptType == ConceptTypeLocation {
				annoMap["predicate"] = PredicateAbout
			}
		case PredicateIsPrimarilyClassifiedBy:
			switch conceptType {
			case ConceptTypeTopic, ConceptTypeLocation:
				annoMap["predicate"] = PredicateAbout
			case ConceptTypeBrand, ConceptTypeGenre:
				annoMap["predicate"] = PredicateIsClassifiedBy
			default:
				continue
			}
		case PredicateMajorMentions:
			annoMap["predicate"] = PredicateAbout
		case PredicateImplicitlyAbout, PredicateImplicitlyClassifiedBy:
			continue
		default:
			if !IsValidFTPinkPredicate(predicate) {
				log.Infof("Invalid PAC predicated not mapped: %s", predicate)
				continue
			}
		}

		convertedAnnotations = append(convertedAnnotations, annoMap)
	}

	if len(convertedAnnotations) == 0 {
		return nil, nil
	}

	return json.Marshal(convertedAnnotations)
}

func toStringArray(val interface{}) ([]string, error) {
	arrVal, ok := val.([]interface{})
	if !ok {
		log.Info("val is not an array")
		return nil, errors.New("unexpected types property")
	}
	result := make([]string, 0)
	for _, v := range arrVal {
		s, ok := v.(string)
		if !ok {
			return nil, errors.New("unexpected types property")
		}
		result = append(result, s)
	}
	return result, nil
}

func getLeafType(listOfTypes []string) string {
	return listOfTypes[len(listOfTypes)-1]
}

func IsValidFTPinkPredicate(pr string) bool {
	var predicates = [...]string{
		PredicateAbout,
		PredicateHasAuthor,
		PredicateHasBrand,
		PredicateHasContributor,
		PredicateHasDisplayTag,
		PredicateIsClassifiedBy,
		PredicateMentions,
	}
	for _, item := range predicates {
		if pr == item {
			return true
		}
	}

	return false
}

func TransformConceptID(id string) string {
	i := strings.LastIndex(id, "/")
	if i == -1 || i == len(id)-1 {
		return ""
	}
	return "http://www.ft.com/thing/" + id[i+1:]
}
