package annotations

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/Financial-Times/go-logger/v2"

	"github.com/Financial-Times/draft-annotations-api/mapper"
	"github.com/pkg/errors"

	tidUtils "github.com/Financial-Times/transactionid-utils-go"
)

const (
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
	manualAnnotationLifecycle    = "manual"
)

// UPPError encapsulates error information for errors originating from calls to UPP annotations endpoint.
type UPPError struct {
	msg     string
	status  int
	uppBody []byte
}

// NewUPPError initializes UPPError
func NewUPPError(msg string, status int, uppBody []byte) UPPError {
	return UPPError{msg: msg, status: status, uppBody: uppBody}
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
	username         string
	password         string
	httpClient       *http.Client
	log              *logger.UPPLogger
}

// NewUPPAnnotationsAPI initializes UPPAnnotationsAPI by given http client,
// the url of the UPP public endpoint for getting published annotations and UPP API key.
func NewUPPAnnotationsAPI(client *http.Client, endpoint string, username string, password string, log *logger.UPPLogger) *UPPAnnotationsAPI {
	return &UPPAnnotationsAPI{endpointTemplate: endpoint, username: username, password: password, httpClient: client, log: log}
}

// GetAll retrieves the list of published annotations for given contentUUID.
// The returned list contains the annotations returned by UPP without filtering.
func (api *UPPAnnotationsAPI) GetAll(ctx context.Context, contentUUID string) ([]interface{}, error) {
	return api.getAnnotations(ctx, contentUUID)
}

// GetAllButV2 retrieves the list of published annotations for given contentUUID but filtering v2 annotations.
func (api *UPPAnnotationsAPI) GetAllButV2(ctx context.Context, contentUUID string) ([]interface{}, error) {
	return api.getAnnotations(ctx, contentUUID, pacAnnotationLifecycle, v1AnnotationLifecycle, nextVideoAnnotationLifecycle, manualAnnotationLifecycle)
}

func (api *UPPAnnotationsAPI) getAnnotations(ctx context.Context, contentUUID string, lifecycles ...string) ([]interface{}, error) {
	uppResponse, err := api.getUPPAnnotationsResponse(ctx, contentUUID, lifecycles...)
	if err != nil {
		return nil, err
	}

	defer uppResponse.Body.Close()
	respBody, err := io.ReadAll(uppResponse.Body)
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

	origin := ctx.Value(OriginSystemIDHeaderKey(OriginSystemIDHeader)).(string)
	var convertedBody []byte
	if origin == PACOriginSystemID {
		convertedBody, err = mapper.ConvertPredicates(respBody)
		if err != nil {
			return nil, errors.Wrap(err, "failed to map predicates from UPP response")
		}
	} else {
		convertedBody = respBody
	}

	if convertedBody == nil {
		return nil, UPPError{msg: NoAnnotationsMsg, status: http.StatusNotFound, uppBody: nil}
	}

	var rawAnnotations []interface{}
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

		//by default publications are not returned from public-annotations-api,
		//so we need to add this parameter to the query
		params.Add("showPublication", "true")

		baseURL.RawQuery = params.Encode()
		apiReqURI = baseURL.String()
	}

	tid, err := tidUtils.GetTransactionIDFromContext(ctx)
	if err != nil {
		tid = "not_found"
	}

	logEntry := api.log.WithTransactionID(tid).WithField("url", apiReqURI).WithUUID(contentUUID)

	apiReq, err := http.NewRequest("GET", apiReqURI, nil)
	if err != nil {
		logEntry.WithError(err).Error("Error in creating the http request")
		return nil, err
	}

	apiReq.SetBasicAuth(api.username, api.password)
	logEntry.Info("Calling UPP Public Annotations API")

	return api.httpClient.Do(apiReq.WithContext(ctx))
}

// GTG is making call the UPP annotations endpoint for predefined synthetic content UUID and check that response is returned
func (api *UPPAnnotationsAPI) GTG() error {
	apiReqURI := fmt.Sprintf(api.endpointTemplate, syntheticContentUUID)
	apiReq, err := http.NewRequest("GET", apiReqURI, nil)
	if err != nil {
		return fmt.Errorf("GTG: %w", err)
	}

	apiReq.SetBasicAuth(api.username, api.password)
	apiResp, err := api.httpClient.Do(apiReq)
	if err != nil {
		return fmt.Errorf("GTG: %w", err)
	}
	defer apiResp.Body.Close()

	if apiResp.StatusCode != http.StatusOK {
		errMsgBody, err := io.ReadAll(apiResp.Body)
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
