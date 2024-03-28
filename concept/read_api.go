package concept

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/Financial-Times/go-logger/v2"

	tidUtils "github.com/Financial-Times/transactionid-utils-go"
)

type ReadAPI interface {
	GetConceptsByIDs(ctx context.Context, ids []string) (map[string]Concept, error)
	Endpoint() string
	GTG() error
}

type internalConcordancesAPI struct {
	endpoint   string
	username   string
	password   string
	httpClient *http.Client
	batchSize  int
	log        *logger.UPPLogger
}

func NewReadAPI(client *http.Client, endpoint string, username string, password string, batchSize int, log *logger.UPPLogger) ReadAPI {
	return &internalConcordancesAPI{
		endpoint:   endpoint,
		username:   username,
		password:   password,
		httpClient: client,
		batchSize:  batchSize,
		log:        log,
	}
}

var ErrUnexpectedResponse = errors.New("concept search API returned a non-200 HTTP status code")

func (search *internalConcordancesAPI) GetConceptsByIDs(ctx context.Context, conceptIDs []string) (map[string]Concept, error) {
	tid, err := tidUtils.GetTransactionIDFromContext(ctx)
	if err != nil {
		tid = tidUtils.NewTransactionID()
		search.log.WithTransactionID(tid).
			WithError(err).
			Info("No Transaction ID provided for concept request, so a new one has been generated.")
		ctx = tidUtils.TransactionAwareContext(ctx, tid)
	}

	var conceptIDsBatch []string
	combinedResult := make(map[string]Concept)

	n := len(conceptIDs)
	for i := 0; i < n; i++ {
		conceptIDsBatch = append(conceptIDsBatch, conceptIDs[i])
		if ((i+1)%search.batchSize == 0) && (i != 0) || (i+1 == n) {
			conceptsBatch, err := search.searchConceptBatch(ctx, conceptIDsBatch)
			if err != nil {
				search.log.WithTransactionID(tid).WithError(err).Info("Failed to fetch concepts batch")
				return nil, err
			}

			for uuid, c := range conceptsBatch {
				combinedResult[uuid] = c
			}
			conceptIDsBatch = []string{}
		}
	}
	search.log.WithTransactionID(tid).Info("Concepts information fetched successfully")
	return combinedResult, nil
}

func (search *internalConcordancesAPI) searchConceptBatch(ctx context.Context, conceptIDs []string) (map[string]Concept, error) {
	tid, _ := tidUtils.GetTransactionIDFromContext(ctx)
	batchConceptsLog := search.log.WithTransactionID(tid)

	req, err := http.NewRequest(http.MethodGet, search.endpoint, nil)
	if err != nil {
		batchConceptsLog.WithError(err).Error("Error in creating the HTTP request to concept search API")
		return nil, err
	}
	req.SetBasicAuth(search.username, search.password)

	q := req.URL.Query()
	for _, id := range conceptIDs {
		q.Add("ids", id)
	}
	req.URL.RawQuery = q.Encode()

	resp, err := search.httpClient.Do(req.WithContext(ctx))
	if err != nil {
		batchConceptsLog.WithError(err).Error("Error making the HTTP request to concept search API")
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, e := io.ReadAll(resp.Body)
		if e != nil {
			err = fmt.Errorf("status %d: %w", resp.StatusCode, ErrUnexpectedResponse)
		} else {
			err = fmt.Errorf("status %d %s: %w", resp.StatusCode, string(body), ErrUnexpectedResponse)
		}
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

func (search *internalConcordancesAPI) Endpoint() string {
	return search.endpoint
}

const ftBrandUUID = "dbb0bdae-1f0c-11e4-b0cb-b2227cce2b54"

func (search *internalConcordancesAPI) GTG() error {
	tid := tidUtils.NewTransactionID()
	ctx := tidUtils.TransactionAwareContext(context.Background(), tid)
	_, err := search.searchConceptBatch(ctx, []string{ftBrandUUID})
	if err != nil {
		search.log.WithTransactionID(tid).WithError(err).Error("Concept search API is not good-to-go")
	}
	return err
}
