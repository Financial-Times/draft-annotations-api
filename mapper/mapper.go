package mapper

import (
	"encoding/json"
	"errors"
	"strings"

	log "github.com/sirupsen/logrus"
)

const PREDICATE_IS_CLASSIFIED_BY = "http://www.ft.com/ontology/classification/isClassifiedBy"
const PREDICATE_IS_PRIMARILY_CLASSIFIED_BY = "http://www.ft.com/ontology/classification/isPrimarilyClassifiedBy"
const PREDICATE_MAJOR_MENTIONS = "http://www.ft.com/ontology/annotation/majorMentions"
const PREDICATE_ABOUT = "http://www.ft.com/ontology/annotation/about"
const PredicateImplicitlyAbout = "http://www.ft.com/ontology/implicitlyAbout"
const PredicateImplicitlyClassifiedBy = "http://www.ft.com/ontology/implicitlyClassifiedBy"
const CONCEPT_TYPE_BRAND = "http://www.ft.com/ontology/product/Brand"
const CONCEPT_TYPE_GENRE = "http://www.ft.com/ontology/Genre"
const CONCEPT_TYPE_TOPIC = "http://www.ft.com/ontology/Topic"
const CONCEPT_TYPE_LOCATION = "http://www.ft.com/ontology/Location"
const CONCEPT_TYPE_SPECIAL_REPORT = "http://www.ft.com/ontology/SpecialReport"
const CONCEPT_TYPE_SUBJECT = "http://www.ft.com/ontology/Subject"

func ConvertPredicates(body []byte) ([]byte, error) {
	originalAnnotations := make([]map[string]interface{}, 0)
	convertedAnnotations := make([]map[string]interface{}, 0)
	err := json.Unmarshal(body, &originalAnnotations)
	if err != nil {
		return []byte{}, errors.New("could not unmarshal json body")
	}

	for i := 0; i < len(originalAnnotations); i++ {
		annoMap := originalAnnotations[i]
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

		if conceptType == CONCEPT_TYPE_SPECIAL_REPORT || conceptType == CONCEPT_TYPE_SUBJECT {
			continue
		}

		if predicate == PREDICATE_IS_CLASSIFIED_BY {
			if conceptType == CONCEPT_TYPE_TOPIC || conceptType == CONCEPT_TYPE_LOCATION {
				originalAnnotations[i]["predicate"] = PREDICATE_ABOUT
			}
		} else if predicate == PREDICATE_IS_PRIMARILY_CLASSIFIED_BY {
			if conceptType == CONCEPT_TYPE_TOPIC || conceptType == CONCEPT_TYPE_LOCATION {
				originalAnnotations[i]["predicate"] = PREDICATE_ABOUT
			} else if conceptType == CONCEPT_TYPE_BRAND || conceptType == CONCEPT_TYPE_GENRE {
				originalAnnotations[i]["predicate"] = PREDICATE_IS_CLASSIFIED_BY
			} else {
				continue
			}
		} else if predicate == PREDICATE_MAJOR_MENTIONS {
			originalAnnotations[i]["predicate"] = PREDICATE_ABOUT
		} else if predicate == PredicateImplicitlyAbout || predicate == PredicateImplicitlyClassifiedBy {
			continue
		}
		convertedAnnotations = append(convertedAnnotations, originalAnnotations[i])
	}

	if len(convertedAnnotations) == 0 {
		return nil, nil
	} else {
		return json.Marshal(convertedAnnotations)
	}
}

func toStringArray(val interface{}) ([]string, error) {
	arrVal, ok := val.([]interface{})
	if !ok {
		log.Info("val is not an array")
		return nil, errors.New("Unexpected types property")
	}
	result := make([]string, 0)
	for _, v := range arrVal {
		s, ok := v.(string)
		if !ok {
			return nil, errors.New("Unexpected types property")
		}
		result = append(result, s)
	}
	return result, nil
}

func getLeafType(listOfTypes []string) string {
	return listOfTypes[len(listOfTypes)-1]
}

func TransformConceptID(id string) string {
	i := strings.LastIndex(id, "/")
	if i == -1 || i == len(id)-1 {
		return ""
	}
	return "http://www.ft.com/thing/" + id[i+1:]
}
