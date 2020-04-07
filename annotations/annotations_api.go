package annotations

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"

	"github.com/Financial-Times/draft-annotations-api/mapper"
	"github.com/pkg/errors"

	tidUtils "github.com/Financial-Times/transactionid-utils-go"
	log "github.com/sirupsen/logrus"
)

const (
	apiKeyHeader        = "X-Api-Key"
	annotationsEndpoint = "/annotations"

	syntheticContentUUID = "4f2f97ea-b8ec-11e4-b8e6-00144feab7de"

	// NoAnnotationsMsg is error message used when there are no processed annotations.
	NoAnnotationsMsg = "No annotations found"
	// UPPBadRequestMsg is error message used when UPP annotations endpoint returns bad request.
	UPPBadRequestMsg = "UPP responded with a client error"
	// UPPNotFoundMsg is error message used when UPP annotations endpoint returns not found.
	UPPNotFoundMsg = "UPP responded with not found"
	// UPPServiceUnavailableMsg is error message used when UPP annotations endpoint returns http error code
	// different from bad request and not found
	UPPServiceUnavailableMsg = "Service unavailable"

	pacAnnotationLifecycle       = "pac"
	v1AnnotationLifecycle        = "v1"
	nextVideoAnnotationLifecycle = "next-video"
)

// UPPError encapsulate error information for errors originating from calls to UPP annotations endpoint.
type UPPError struct {
	msg     string
	status  int
	uppBody []byte
}

// Error returns the error message.
func (ue UPPError) Error() string {
	return ue.msg
}

// Status returns the http status code returned by the call to the UPP annotations endpoint.
func (ue UPPError) Status() int {
	return ue.status
}

// UPPBody returns the http response body returned by the call to the UPP annotations endpoint.
func (ue UPPError) UPPBody() []byte {
	return ue.uppBody
}

// UPPAnnotationsAPI retrieves published annotations from UPP.
type UPPAnnotationsAPI struct {
	endpointTemplate string
	apiKey           string
	httpClient       *http.Client
}

// NewUPPAnnotationsAPI initializes UPPAnnotationsAPI by given http client,
// the url of the UPP public endpoint for getting published annotations and UPP API key.
func NewUPPAnnotationsAPI(client *http.Client, endpoint string, apiKey string) *UPPAnnotationsAPI {
	return &UPPAnnotationsAPI{endpointTemplate: endpoint, apiKey: apiKey, httpClient: client}
}

// GetAll retrieves the list of published annotations for given contentUUID.
// The returned list contains the annotations returned by UPP without filtering.
func (api *UPPAnnotationsAPI) GetAll(ctx context.Context, contentUUID string) ([]Annotation, error) {
	return api.getAnnotations(ctx, contentUUID)
}

// GetAllButV2 retrieves the list of published annotations for given contentUUID but filtering v2 annotations.
func (api *UPPAnnotationsAPI) GetAllButV2(ctx context.Context, contentUUID string) ([]Annotation, error) {
	return api.getAnnotations(ctx, contentUUID, pacAnnotationLifecycle, v1AnnotationLifecycle, nextVideoAnnotationLifecycle)
}

func (api *UPPAnnotationsAPI) getAnnotations(ctx context.Context, contentUUID string, lifecycles ...string) ([]Annotation, error) {
	uppResponse, err := api.getUPPAnnotationsResponse(ctx, contentUUID, lifecycles...)
	if err != nil {
		return nil, err
	}

	defer uppResponse.Body.Close()
	respBody, err := ioutil.ReadAll(uppResponse.Body)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read UPP response body")
	}

	if uppResponse.StatusCode != http.StatusOK {
		if uppResponse.StatusCode == http.StatusBadRequest {
			return nil, UPPError{msg: UPPBadRequestMsg, status: http.StatusBadRequest, uppBody: respBody}
		}
		if uppResponse.StatusCode == http.StatusNotFound {
			return nil, UPPError{msg: UPPNotFoundMsg, status: http.StatusNotFound, uppBody: respBody}
		}

		return nil, UPPError{msg: UPPServiceUnavailableMsg, status: http.StatusServiceUnavailable, uppBody: nil}
	}

	convertedBody, err := mapper.ConvertPredicates(respBody)
	if err != nil {
		return nil, errors.Wrap(err, "failed to map predicates from UPP response")
	}

	if convertedBody == nil {
		return nil, UPPError{msg: NoAnnotationsMsg, status: http.StatusNotFound, uppBody: nil}
	}

	rawAnnotations := []Annotation{}
	err = json.Unmarshal(convertedBody, &rawAnnotations)
	if err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal UPP annotations")
	}

	return rawAnnotations, nil
}

func (api *UPPAnnotationsAPI) getUPPAnnotationsResponse(ctx context.Context, contentUUID string, lifecycles ...string) (*http.Response, error) {
	apiReqURI := fmt.Sprintf(api.endpointTemplate, contentUUID)

	if len(lifecycles) != 0 {
		baseURL, err := url.Parse(apiReqURI)
		if err != nil {
			return nil, err
		}

		params := url.Values{}
		for _, lc := range lifecycles {
			params.Add("lifecycle", lc)
		}

		baseURL.RawQuery = params.Encode()
		apiReqURI = baseURL.String()
	}

	getAnnotationsLog := log.WithField("url", apiReqURI).WithField("uuid", contentUUID)

	tid, err := tidUtils.GetTransactionIDFromContext(ctx)
	if err != nil {
		tid = "not_found"
	}

	getAnnotationsLog = getAnnotationsLog.WithField(tidUtils.TransactionIDKey, tid)

	apiReq, err := http.NewRequest("GET", apiReqURI, nil)
	if err != nil {
		getAnnotationsLog.WithError(err).Error("Error in creating the http request")
		return nil, err
	}

	apiReq.Header.Set(apiKeyHeader, api.apiKey)
	getAnnotationsLog.Info("Calling UPP Public Annotations API")

	return api.httpClient.Do(apiReq.WithContext(ctx))
}

// GTG is making call the UPP annotations endpoint for predefined synthetic content UUID and check that response is returned
func (api *UPPAnnotationsAPI) GTG() error {
	apiReqURI := fmt.Sprintf(api.endpointTemplate, syntheticContentUUID)
	apiReq, err := http.NewRequest("GET", apiReqURI, nil)
	if err != nil {
		return fmt.Errorf("GTG: %w", err)
	}

	apiReq.Header.Set(apiKeyHeader, api.apiKey)

	apiResp, err := api.httpClient.Do(apiReq)
	if err != nil {
		return fmt.Errorf("GTG: %w", err)
	}
	defer apiResp.Body.Close()

	if apiResp.StatusCode != http.StatusOK {
		errMsgBody, err := ioutil.ReadAll(apiResp.Body)
		if err != nil {
			return fmt.Errorf("status %d: %w", apiResp.StatusCode, ErrGTGNotOK)
		}
		return fmt.Errorf("status %d %s: %w", apiResp.StatusCode, string(errMsgBody), ErrGTGNotOK)
	}
	return nil
}

// Endpoint retrieves the template for UPP annotations endpoint
func (api *UPPAnnotationsAPI) Endpoint() string {
	return api.endpointTemplate
}
