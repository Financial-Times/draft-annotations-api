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

	NoAnnotationsMsg         = "No annotations found"
	UPPBadRequestMsg         = "UPP responded with a client error"
	UPPNotFoundMsg           = "UPP responded with not found"
	UPPServiceUnavailableMsg = "Service unavailable"

	pacAnnotationLifecycle       = "pac"
	v1AnnotationLifecycle        = "v1"
	nextVideoAnnotationLifecycle = "next-video"
)

type UPPError struct {
	msg     string
	status  int
	uppBody []byte
}

func (ue UPPError) Error() string {
	return ue.msg
}

func (ue UPPError) Status() int {
	return ue.status
}

func (ue UPPError) UPPBody() []byte {
	return ue.uppBody
}

type UPPAnnotationsAPI interface {
	GetAll(context.Context, string) ([]Annotation, error)
	GetAllButV2(context.Context, string) ([]Annotation, error)
	Endpoint() string
	GTG() error
}

type annotationsAPI struct {
	endpointTemplate string
	apiKey           string
	httpClient       *http.Client
}

func NewUPPAnnotationsAPI(client *http.Client, endpoint string, apiKey string) UPPAnnotationsAPI {
	return &annotationsAPI{endpointTemplate: endpoint, apiKey: apiKey, httpClient: client}
}

func (api *annotationsAPI) GetAll(ctx context.Context, contentUUID string) ([]Annotation, error) {
	return api.getAnnotations(ctx, contentUUID)
}

func (api *annotationsAPI) GetAllButV2(ctx context.Context, contentUUID string) ([]Annotation, error) {
	return api.getAnnotations(ctx, contentUUID, pacAnnotationLifecycle, v1AnnotationLifecycle, nextVideoAnnotationLifecycle)
}

func (api *annotationsAPI) getAnnotations(ctx context.Context, contentUUID string, lifecycles ...string) ([]Annotation, error) {
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
		return nil, errors.New("failed to map predicates from UPP response")
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

func (api *annotationsAPI) getUPPAnnotationsResponse(ctx context.Context, contentUUID string, lifecycles ...string) (*http.Response, error) {
	apiReqURI := fmt.Sprintf(api.endpointTemplate, contentUUID)

	if len(lifecycles) != 0 {
		baseUrl, err := url.Parse(apiReqURI)
		if err != nil {
			return nil, err
		}

		params := url.Values{}
		for _, lc := range lifecycles {
			params.Add("lifecycle", lc)
		}

		baseUrl.RawQuery = params.Encode()
		apiReqURI = baseUrl.String()
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
	getAnnotationsLog.Debug("Calling UPP Public Annotations API")

	return api.httpClient.Do(apiReq.WithContext(ctx))
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
