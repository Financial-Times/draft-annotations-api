package concept

import (
	"context"
	"encoding/json"
	"fmt"
	tidUtils "github.com/Financial-Times/transactionid-utils-go"
	log "github.com/sirupsen/logrus"
	"net/http"
)

type SearchAPI interface {
	SearchConcepts(ctx context.Context, ids []string) (map[string]Concept, error)
}

type conceptSearchAPI struct {
	endpoint   string
	apiKey     string
	httpClient *http.Client
	batchSize  int
}

func NewSearchAPI(endpoint string, apiKey string, batchSize int) SearchAPI {
	return &conceptSearchAPI{
		endpoint:   endpoint,
		apiKey:     apiKey,
		httpClient: &http.Client{},
		batchSize:  batchSize,
	}
}

func (s *conceptSearchAPI) SearchConcepts(ctx context.Context, conceptIDs []string) (map[string]Concept, error) {
	combinedResult := make(map[string]Concept)

	conceptIDsBatch := []string{}
	for i := 0; i < len(conceptIDs); i++ {
		conceptIDsBatch = append(conceptIDsBatch, conceptIDs[i])
		if ((i+1)%s.batchSize == 0) && (i != 0) || (i+1 == len(conceptIDs)) {
			conceptsBatch, err := s.searchConceptBatch(ctx, conceptIDsBatch)
			if err != nil {
				return nil, err
			}
			for _, c := range conceptsBatch {
				combinedResult[c.Id] = c
			}
			conceptIDsBatch = []string{}
		}
	}

	return combinedResult, nil
}

const apiKeyHeader = "X-Api-Key"

func (s *conceptSearchAPI) searchConceptBatch(ctx context.Context, conceptIDs []string) ([]Concept, error) {
	tid, err := tidUtils.GetTransactionIDFromContext(ctx)
	if err != nil {
		log.WithError(err).Warn("transaction ID not found for batch request of concepts to concept search API")
		tid = tidUtils.NewTransactionID()
	}
	batchConceptsLog := log.WithField(tidUtils.TransactionIDKey, tid)

	req, err := http.NewRequest("GET", s.endpoint, nil)
	if err != nil {
		batchConceptsLog.WithError(err).Error("Error in creating the HTTP request to concept search API")
		return nil, err
	}
	req.Header.Set(apiKeyHeader, s.apiKey)
	req.Header.Set(tidUtils.TransactionIDHeader, tid)

	q := req.URL.Query()
	for _, id := range conceptIDs {
		q.Add("ids", id)
	}
	req.URL.RawQuery = q.Encode()

	resp, err := s.httpClient.Do(req)
	if err != nil {
		batchConceptsLog.WithError(err).Error("Error making the HTTP request to concept search API")
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		err = fmt.Errorf("concept search API returned a non-200 HTTP status code: %v", resp.StatusCode)
		batchConceptsLog.WithError(err).Error("Error received from concept search API")
		return nil, err
	}

	var concepts []Concept
	err = json.NewDecoder(resp.Body).Decode(&concepts)
	if err != nil {
		batchConceptsLog.WithError(err).Error("Error in unmarshalling the HTTP response from concept search API")
		return nil, err
	}

	return concepts, nil
}
