package concept

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	tidUtils "github.com/Financial-Times/transactionid-utils-go"
	log "github.com/sirupsen/logrus"
)

type SearchAPI interface {
	SearchConcepts(ctx context.Context, ids []string) (map[string]Concept, error)
	Endpoint() string
	GTG() error
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

func (search *conceptSearchAPI) SearchConcepts(ctx context.Context, conceptIDs []string) (map[string]Concept, error) {
	combinedResult := make(map[string]Concept)
	tid, err := tidUtils.GetTransactionIDFromContext(ctx)

	if err != nil {
		tid = tidUtils.NewTransactionID()
		log.WithField(tidUtils.TransactionIDKey, tid).
			WithError(err).
			Warn("Transaction ID error for requests of concepts to concept search API: Generated a new transaction ID")
		ctx = tidUtils.TransactionAwareContext(ctx, tid)
	}

	var conceptIDsBatch []string
	n := len(conceptIDs)
	for i := 0; i < n; i++ {
		conceptIDsBatch = append(conceptIDsBatch, conceptIDs[i])
		if ((i+1)%search.batchSize == 0) && (i != 0) || (i+1 == n) {
			conceptsBatch, err := search.searchConceptBatch(ctx, conceptIDsBatch)
			if err != nil {
				log.WithField(tidUtils.TransactionIDKey, tid).Info("Concepts information fetched successfully")
				return nil, err
			}
			for _, c := range conceptsBatch {
				combinedResult[c.Id] = c
			}
			conceptIDsBatch = []string{}
		}
	}
	log.WithField(tidUtils.TransactionIDKey, tid).Info("Concepts information fetched successfully")
	return combinedResult, nil
}

const apiKeyHeader = "X-Api-Key"

func (search *conceptSearchAPI) searchConceptBatch(ctx context.Context, conceptIDs []string) ([]Concept, error) {
	tid, _ := tidUtils.GetTransactionIDFromContext(ctx)
	batchConceptsLog := log.WithField(tidUtils.TransactionIDKey, tid)

	req, err := http.NewRequest("GET", search.endpoint, nil)
	if err != nil {
		batchConceptsLog.WithError(err).Error("Error in creating the HTTP request to concept search API")
		return nil, err
	}
	req.Header.Set(apiKeyHeader, search.apiKey)
	req.Header.Set(tidUtils.TransactionIDHeader, tid)

	q := req.URL.Query()
	for _, id := range conceptIDs {
		q.Add("ids", id)
	}
	req.URL.RawQuery = q.Encode()

	resp, err := search.httpClient.Do(req)
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

	var result SearchResult
	err = json.NewDecoder(resp.Body).Decode(&result)
	if err != nil {
		batchConceptsLog.WithError(err).Error("Error in unmarshalling the HTTP response from concept search API")
		return nil, err
	}

	return result.Concepts, nil
}

func (search *conceptSearchAPI) Endpoint() string {
	return search.endpoint
}

const ftBrandUUID = "dbb0bdae-1f0c-11e4-b0cb-b2227cce2b54"

func (search *conceptSearchAPI) GTG() error {
	tid := tidUtils.NewTransactionID()
	ctx := tidUtils.TransactionAwareContext(context.Background(), tid)
	_, err := search.searchConceptBatch(ctx, []string{ftBrandUUID})
	if err != nil {
		log.WithError(err).WithField(tidUtils.TransactionIDKey, tid).Error("Concept search API is not good-to-go")
	}
	return err
}
