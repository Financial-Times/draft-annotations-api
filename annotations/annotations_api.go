package annotations

import (
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"

	tidutils "github.com/Financial-Times/transactionid-utils-go"
	log "github.com/sirupsen/logrus"
)

const apiKeyHeader = "X-Api-Key"
const annotationsEndpoint = "/annotations"

const syntheticContentUUID = "4f2f97ea-b8ec-11e4-b8e6-00144feab7de"

type AnnotationsAPI interface {
	Get(ctx context.Context, contentUUID string) (*http.Response, error)
	GTG() error
	Endpoint() string
}

type annotationsAPI struct {
	endpointTemplate string
	apiKey           string
	httpClient       *http.Client
}

func NewAnnotationsAPI(endpoint string, apiKey string) AnnotationsAPI {
	return &annotationsAPI{endpointTemplate: endpoint, apiKey: apiKey, httpClient: &http.Client{}}
}

func (api *annotationsAPI) Get(ctx context.Context, contentUUID string) (*http.Response, error) {
	apiReqURI := fmt.Sprintf(api.endpointTemplate, contentUUID)
	getAnnotationsLog := log.WithField("url", apiReqURI).WithField("uuid", contentUUID)

	tID, err := tidutils.GetTransactionIDFromContext(ctx)
	if err != nil {
		getAnnotationsLog.WithField(tidutils.TransactionIDKey, "TID Not found: "+err.Error())
	} else {
		getAnnotationsLog = getAnnotationsLog.WithField(tidutils.TransactionIDKey, tID)
	}

	apiReq, err := http.NewRequest("GET", apiReqURI, nil)
	if err != nil {
		getAnnotationsLog.WithError(err).Error("Error in creating the http request")
		return nil, err
	}

	apiReq.Header.Set(apiKeyHeader, api.apiKey)
	if tID != "" {
		apiReq.Header.Set(tidutils.TransactionIDHeader, tID)
	}

	getAnnotationsLog.Info("Calling UPP Public Annotations API")
	return api.httpClient.Do(apiReq)
}

func (api *annotationsAPI) GTG() error {
	apiReqURI := fmt.Sprintf(api.endpointTemplate, syntheticContentUUID)
	apiReq, err := http.NewRequest("GET", apiReqURI, nil)
	if err != nil {
		return fmt.Errorf("gtg request error: %v", err.Error())
	}

	apiReq.Header.Set(apiKeyHeader, api.apiKey)

	apiResp, err := api.httpClient.Do(apiReq)
	if err != nil {
		return fmt.Errorf("gtg call error: %v", err.Error())
	}
	defer apiResp.Body.Close()

	if apiResp.StatusCode != http.StatusOK {
		errMsgBody, err := ioutil.ReadAll(apiResp.Body)
		if err != nil {
			return errors.New("gtg returned a non-200 HTTP status")
		}
		return fmt.Errorf("gtg returned a non-200 HTTP status: %v - %v", apiResp.StatusCode, string(errMsgBody))
	}
	return nil
}

func (api *annotationsAPI) Endpoint() string {
	return api.endpointTemplate
}
