package annotations

import (
	"context"
	"encoding/json"
	"fmt"
	tidutils "github.com/Financial-Times/transactionid-utils-go"
	log "github.com/sirupsen/logrus"
	"io/ioutil"
	"net/http"
	"strings"
)

const batchSize = 30

type ConceptAugmenter struct {
	endpointTemplate       string
	endpointGTGTemplate    string
	conceptsGTGCredentials string
	apiKey                 string
	httpClient             *http.Client
	clusterUser            string
	clusterPassword        string
}

func NewConceptAugmenter(endpoint string, gtgEndpoint string, conceptsGTGCredentials string, apiKey string) (ConceptAugmenter, error) {
	var ca  ConceptAugmenter
	if len(conceptsGTGCredentials) == 0 {
		return ca, fmt.Errorf("credentials missing credentials,augmenter cannot make gtg requet to concepts-search-api")
	}
	credentials := strings.Split(conceptsGTGCredentials, ":")
	ca=ConceptAugmenter{endpointTemplate: endpoint,
		endpointGTGTemplate:     gtgEndpoint,
		conceptsGTGCredentials: conceptsGTGCredentials,
		apiKey:                 apiKey,
		httpClient:             &http.Client{},
		clusterUser:            credentials[0],
		clusterPassword:        credentials[1],
	}
	return ca, nil
}

func (ca ConceptAugmenter) augmentConcepts(depleted []Annotation, ctx context.Context) ([]Annotation, error) {
	augmentedAnnotations := []Annotation{}
	conceptIds := []string{}
	reqIds := []string{}
	for _, ann := range depleted {
		conceptIds = append(conceptIds, ann.ConceptId)
	}
	for i := 0; i < len(conceptIds); i += batchSize {
		end := i + batchSize
		if end > len(conceptIds) {
			end = len(conceptIds)
		}
		reqIds = conceptIds[i:end]
		concepts, err := ca.getConcepts(ctx, reqIds)
		if err != nil {
			return []Annotation{}, nil
		}
		for _, ann := range depleted {
			concept := concepts[ann.ConceptId]
			ann.PrefLabel = concept.PrefLabel
			ann.IsFTAuthor = concept.IsFTAuthor
			ann.Types = concept.Types
			augmentedAnnotations = append(augmentedAnnotations, ann)
		}
	}
	return augmentedAnnotations, nil
}

func (ca ConceptAugmenter) getConcepts(ctx context.Context, ids []string) (map[string]Concept, error) {
	conceptsMap := make(map[string]Concept)
	var concepts Concepts

	tid, err := tidutils.GetTransactionIDFromContext(ctx)
	if err != nil {
		tid = "not_found"
	}
	getConceptsLog := log.WithField(tidutils.TransactionIDKey, tid)

	req, err := http.NewRequest("GET", ca.endpointTemplate, nil)
	if err != nil {
		getConceptsLog.WithError(err).Error("Error in creating the http request")
		return nil, err
	}
	req.Header.Set(apiKeyHeader, ca.apiKey)
	req.Header.Set(apiKeyHeader, ca.apiKey)
	if tid != "" {
		req.Header.Set(tidutils.TransactionIDHeader, tid)
	}
	q := req.URL.Query()
	for _, id := range ids {
		q.Add("ids", id)
	}
	req.URL.RawQuery = q.Encode()

	resp, err := ca.httpClient.Do(req)
	if err != nil {
		getConceptsLog.WithError(err).Error("Error making the http request")
		return conceptsMap, err
	}
	defer resp.Body.Close()

	err = json.NewDecoder(resp.Body).Decode(concepts)
	if err != nil {
		getConceptsLog.WithError(err).Error("Error in unmarshaling the http respone")
		return conceptsMap, err
	}
	for _, concept := range concepts {
		conceptsMap[concept.Id] = concept
	}
	return conceptsMap, err
}

func (ca *ConceptAugmenter) IsGTG() error {
	req, err := http.NewRequest("GET", ca.endpointGTGTemplate, nil)
	req.SetBasicAuth(ca.clusterUser, ca.clusterPassword)
	if err != nil {
		return fmt.Errorf("gtg endpoint returned a non-200 status: %v", err.Error())
	}
	apiResp, err := ca.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("gtg endpoint returned a non-200 status: %v", err.Error())
	}
	defer apiResp.Body.Close()

	if apiResp.StatusCode != http.StatusOK {
		errMsgBody, err := ioutil.ReadAll(apiResp.Body)
		if err != nil {
			return fmt.Errorf("gtg returned a non-200 HTTP status: [%v]", apiResp.StatusCode)
		}
		return fmt.Errorf("gtg returned a non-200 HTTP status [%v]: %v", apiResp.StatusCode, string(errMsgBody))
	}
	return nil
}
