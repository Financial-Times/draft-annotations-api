package annotations

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"

	tidutils "github.com/Financial-Times/transactionid-utils-go"
	log "github.com/sirupsen/logrus"
)

const (
	isClassifiedBy = "http://www.ft.com/ontology/classification/isClassifiedBy"
	implicitlyClassifiedBy = "http://www.ft.com/ontology/implicitlyClassifiedBy"
)

type AnnotationsService interface {
	Read(ctx context.Context, uuid string) ([]Annotation, error)
	Write(ctx context.Context, uuid string, draftAnnotations []Annotation, imply bool) ([]Annotation, error)
}

type ConceptChecker interface {
	IsConcept(id string) bool
}

type Annotation struct {
	Predicate string   `json:"predicate"`
	ConceptId string   `json:"id"`
	ApiUrl    string   `json:"apiUrl,omitempty"`
	Types     []string `json:"types,omitempty"`
	PrefLabel string   `json:"prefLabel,omitempty"`
}

type annotationsService struct {
	uppAnnotations AnnotationsAPI
	reasoner       *Reasoner
}

type UPPAnnotationsApiError struct {
	StatusCode int
	msg        string
	Body       io.Reader
}

func (e *UPPAnnotationsApiError) Error() string {
	return e.msg
}

func NewAnnotationsService(uppAnnotations AnnotationsAPI, brandsResolver BrandsResolverService, idLinter *IDLinter, genres ConceptChecker) AnnotationsService {
	removeImplicitBrands := NewRemoveRule([]string{implicitlyClassifiedBy})
	addImplicitBrands := NewImplicitBrandsRule([]string{isClassifiedBy}, implicitlyClassifiedBy, genres, brandsResolver)
	reasoner := NewReasoner([]Rule{idLinter, removeImplicitBrands, addImplicitBrands})

	return &annotationsService{uppAnnotations: uppAnnotations, reasoner: reasoner}
}

func (s *annotationsService) Read(ctx context.Context, uuid string) ([]Annotation, error) {
	tid, err := tidutils.GetTransactionIDFromContext(ctx)
	if err != nil {
		tid = "not_found"
	}

	reqLog := log.WithField(tidutils.TransactionIDKey, tid)

	resp, err := s.uppAnnotations.Get(ctx, uuid)
	if err != nil {
		reqLog.WithError(err).Error("error calling UPP annotations API")
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		buffer := bytes.Buffer{}
		io.Copy(&buffer, resp.Body)
		return nil, &UPPAnnotationsApiError{resp.StatusCode, resp.Status, &buffer}
	}

	draftAnnotations := []Annotation{}
	if err = json.NewDecoder(resp.Body).Decode(&draftAnnotations); err != nil {
		reqLog.WithError(err).Error("unable to parse response from UPP annotations API")
		return nil, err
	}

	draftAnnotations = ConvertPredicates(draftAnnotations)
	if len(draftAnnotations) == 0 {
		reqLog.WithField("uuid", uuid).Error("Error in mapping UPP annotations")
		msg := &bytes.Buffer{}
		json.NewEncoder(msg).Encode(map[string]interface{}{"message":"No annotations can be found"})
		return nil, &UPPAnnotationsApiError{http.StatusNotFound, "No annotations can be found", bytes.NewReader(msg.Bytes())}
	}

	return draftAnnotations, nil
}

func (s *annotationsService) Write(ctx context.Context, uuid string, draftAnnotations []Annotation, imply bool) ([]Annotation, error) {
	if imply {
		var err error
		draftAnnotations, err = s.reasoner.Process(ctx, draftAnnotations)
		if err != nil {
			return nil, err
		}
	}

	// TODO save annotations

	return draftAnnotations, nil
}

func (s *annotationsService) isImplicitAnnotation(ann Annotation) bool {
	return ann.Predicate == implicitlyClassifiedBy
}
