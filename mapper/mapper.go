package mapper

import (
	"encoding/json"
	"errors"
	log "github.com/sirupsen/logrus"
)

const PREDICATE_IS_CLASSIFIED_BY = "http://www.ft.com/ontology/classification/isClassifiedBy"
const PREDICATE_IS_PRIMARILY_CLASSIFIED_BY = "http://www.ft.com/ontology/classification/isPrimarilyClassifiedBy"
const PREDICATE_MAJOR_MENTIONS = "http://www.ft.com/ontology/annotation/majorMentions"
const PREDICATE_ABOUT = "http://www.ft.com/ontology/annotation/about"
const CONCEPT_TYPE_TOPIC = "http://www.ft.com/ontology/Topic"
const CONCEPT_TYPE_LOCATION = "http://www.ft.com/ontology/Location"
const CONCEPT_TYPE_SPECIAL_REPORT = "http://www.ft.com/ontology/SpecialReport"
const CONCEPT_TYPE_SUBJECT = "http://www.ft.com/ontology/Subject"

func ConvertPredicates(body []byte) ([]byte, error) {
	originalAnnotations := make([]map[string]interface{}, 0)
	convertedAnnotations := make([]map[string]interface{}, 0)
	err := json.Unmarshal(body, &originalAnnotations)
	if err != nil {
		log.Fatal("Could not unmarshall json body", err)
	}

	for i := 0; i < len(originalAnnotations); i++ {
		annoMap := originalAnnotations[i]
		pred, ok := annoMap["predicate"]
		if !ok {
			log.Info("no predicate")
			continue
		}
		predicate := pred.(string)
		someTypes, ok := annoMap["types"]
		if !ok {
			log.Info("no types")
			continue
		}

		stringTypes, _ := toStringArray(someTypes)
		conceptType := getLeafType(stringTypes)
		if conceptType != CONCEPT_TYPE_SPECIAL_REPORT && conceptType != CONCEPT_TYPE_SUBJECT {
			if predicate == PREDICATE_IS_CLASSIFIED_BY && (conceptType == CONCEPT_TYPE_TOPIC || conceptType == CONCEPT_TYPE_LOCATION) {
				originalAnnotations[i]["predicate"] = PREDICATE_ABOUT
			} else if predicate == PREDICATE_IS_PRIMARILY_CLASSIFIED_BY && (conceptType == CONCEPT_TYPE_TOPIC || conceptType == CONCEPT_TYPE_LOCATION) {
				originalAnnotations[i]["predicate"] = PREDICATE_ABOUT
			} else if predicate == PREDICATE_MAJOR_MENTIONS {
				originalAnnotations[i]["predicate"] = PREDICATE_ABOUT
			}
			convertedAnnotations = append(convertedAnnotations, originalAnnotations[i])
		}
		// otherwise discarded
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
