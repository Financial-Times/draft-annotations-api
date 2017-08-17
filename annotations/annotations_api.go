package annotations

import (
	"context"
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

	tid, err := tidutils.GetTransactionIDFromContext(ctx)
	if err != nil {
		tid = "not_found"
	}

	getAnnotationsLog = getAnnotationsLog.WithField(tidutils.TransactionIDKey, tid)

	apiReq, err := http.NewRequest("GET", apiReqURI, nil)
	if err != nil {
		getAnnotationsLog.WithError(err).Error("Error in creating the http request")
		return nil, err
	}

	apiReq.Header.Set(apiKeyHeader, api.apiKey)
	if tid != "" {
		apiReq.Header.Set(tidutils.TransactionIDHeader, tid)
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
			return fmt.Errorf("gtg returned a non-200 HTTP status [%v]", apiResp.StatusCode)
		}
		return fmt.Errorf("gtg returned a non-200 HTTP status [%v]: %v", apiResp.StatusCode, string(errMsgBody))
	}
	return nil
}

func (api *annotationsAPI) Endpoint() string {
	return api.endpointTemplate
}
