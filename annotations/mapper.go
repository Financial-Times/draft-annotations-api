package annotations

const PREDICATE_IS_CLASSIFIED_BY = "http://www.ft.com/ontology/classification/isClassifiedBy"
const PREDICATE_IS_PRIMARILY_CLASSIFIED_BY = "http://www.ft.com/ontology/classification/isPrimarilyClassifiedBy"
const PREDICATE_MAJOR_MENTIONS = "http://www.ft.com/ontology/annotation/majorMentions"
const PREDICATE_ABOUT = "http://www.ft.com/ontology/annotation/about"
const CONCEPT_TYPE_BRAND = "http://www.ft.com/ontology/product/Brand"
const CONCEPT_TYPE_GENRE = "http://www.ft.com/ontology/Genre"
const CONCEPT_TYPE_TOPIC = "http://www.ft.com/ontology/Topic"
const CONCEPT_TYPE_LOCATION = "http://www.ft.com/ontology/Location"
const CONCEPT_TYPE_SPECIAL_REPORT = "http://www.ft.com/ontology/SpecialReport"
const CONCEPT_TYPE_SUBJECT = "http://www.ft.com/ontology/Subject"

func ConvertPredicates(originalAnnotations []Annotation) []Annotation {
	convertedAnnotations := []Annotation{}

	for _, ann := range originalAnnotations {
		predicate := ann.Predicate

		conceptType := getLeafType(ann.Types)
		if conceptType != CONCEPT_TYPE_SPECIAL_REPORT && conceptType != CONCEPT_TYPE_SUBJECT {
			if predicate == PREDICATE_IS_CLASSIFIED_BY {
				if conceptType == CONCEPT_TYPE_TOPIC || conceptType == CONCEPT_TYPE_LOCATION {
					ann.Predicate = PREDICATE_ABOUT
				}
			} else if predicate == PREDICATE_IS_PRIMARILY_CLASSIFIED_BY {
				if conceptType == CONCEPT_TYPE_TOPIC || conceptType == CONCEPT_TYPE_LOCATION {
					ann.Predicate = PREDICATE_ABOUT
				} else if conceptType == CONCEPT_TYPE_BRAND || conceptType == CONCEPT_TYPE_GENRE {
					ann.Predicate = PREDICATE_IS_CLASSIFIED_BY
				} else {
					continue
				}
			} else if predicate == PREDICATE_MAJOR_MENTIONS {
				ann.Predicate = PREDICATE_ABOUT
			}
			convertedAnnotations = append(convertedAnnotations, ann)
		}
	}

	return convertedAnnotations
}

func getLeafType(listOfTypes []string) string {
	return listOfTypes[len(listOfTypes)-1]
}
