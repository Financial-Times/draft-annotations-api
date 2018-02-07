package annotations

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	tidUtils "github.com/Financial-Times/transactionid-utils-go"
	"github.com/satori/go.uuid"
	"github.com/stretchr/testify/assert"
)

const testAPIKey = "testAPIKey"

func TestHappyAnnotationsAPIGTG(t *testing.T) {
	annotationsServerMock := newAnnotationsAPIGTGServerMock(t, http.StatusOK, "I am happy!")
	defer annotationsServerMock.Close()

	annotationsAPI := NewUPPAnnotationsAPI(annotationsServerMock.URL+"/content/%v/annotations", testAPIKey)
	err := annotationsAPI.GTG()
	assert.NoError(t, err)
}

func TestUnhappyAnnotationsAPIGTG(t *testing.T) {
	annotationsServerMock := newAnnotationsAPIGTGServerMock(t, http.StatusServiceUnavailable, "I am not happy!")
	defer annotationsServerMock.Close()

	annotationsAPI := NewUPPAnnotationsAPI(annotationsServerMock.URL+"/content/%v/annotations", testAPIKey)
	err := annotationsAPI.GTG()
	assert.EqualError(t, err, "gtg returned a non-200 HTTP status [503]: I am not happy!")
}

func TestAnnotationsAPIGTGWrongAPIKey(t *testing.T) {
	annotationsServerMock := newAnnotationsAPIGTGServerMock(t, http.StatusServiceUnavailable, "I not am happy!")
	defer annotationsServerMock.Close()

	annotationsAPI := NewUPPAnnotationsAPI(annotationsServerMock.URL+"/content/%v/annotations", "a-non-existing-key")
	err := annotationsAPI.GTG()
	assert.EqualError(t, err, "gtg returned a non-200 HTTP status [401]: unauthorized")
}

func TestAnnotationsAPIGTGInvalidURL(t *testing.T) {
	annotationsAPI := NewUPPAnnotationsAPI(":#", testAPIKey)
	err := annotationsAPI.GTG()
	assert.EqualError(t, err, "gtg request error: parse :: missing protocol scheme")
}

func TestAnnotationsAPIGTGConnectionError(t *testing.T) {
	annotationsServerMock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	annotationsServerMock.Close()

	annotationsAPI := NewUPPAnnotationsAPI(annotationsServerMock.URL+"/content/%v/annotations", testAPIKey)
	err := annotationsAPI.GTG()
	assert.Error(t, err)
}

func TestHappyAnnotationsAPI(t *testing.T) {
	uuid := uuid.NewV4().String()
	tid := "tid_all-good"
	ctx := tidUtils.TransactionAwareContext(context.TODO(), tid)

	annotationsServerMock := newAnnotationsAPIServerMock(t, tid, uuid, http.StatusOK, "I am happy!")
	defer annotationsServerMock.Close()

	annotationsAPI := NewUPPAnnotationsAPI(annotationsServerMock.URL+"/content/%v/annotations", testAPIKey)
	resp, err := annotationsAPI.Get(ctx, uuid)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestUnhappyAnnotationsAPI(t *testing.T) {
	uuid := uuid.NewV4().String()
	tid := "tid_all-good?"
	ctx := tidUtils.TransactionAwareContext(context.TODO(), tid)

	annotationsServerMock := newAnnotationsAPIServerMock(t, tid, uuid, http.StatusServiceUnavailable, "I am definitely not happy!")
	defer annotationsServerMock.Close()

	annotationsAPI := NewUPPAnnotationsAPI(annotationsServerMock.URL+"/content/%v/annotations", testAPIKey)
	resp, err := annotationsAPI.Get(ctx, uuid)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)
}

func TestNoTIDAnnotationsAPI(t *testing.T) {
	uuid := uuid.NewV4().String()
	annotationsServerMock := newAnnotationsAPIServerMock(t, "not_found", uuid, http.StatusServiceUnavailable, "I am definitely not happy!")
	defer annotationsServerMock.Close()

	annotationsAPI := NewUPPAnnotationsAPI(annotationsServerMock.URL+"/content/%v/annotations", testAPIKey)
	resp, err := annotationsAPI.Get(context.TODO(), uuid)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)
}

func TestRequestFailsAnnotationsAPI(t *testing.T) {
	annotationsAPI := NewUPPAnnotationsAPI(":#", testAPIKey)
	resp, err := annotationsAPI.Get(context.TODO(), "")

	assert.Error(t, err)
	assert.Nil(t, resp)
}

func TestResponseFailsAnnotationsAPI(t *testing.T) {
	annotationsAPI := NewUPPAnnotationsAPI("#:", testAPIKey)
	resp, err := annotationsAPI.Get(context.TODO(), "")

	assert.Error(t, err)
	assert.Nil(t, resp)
}

func newAnnotationsAPIServerMock(t *testing.T, tid string, uuid string, status int, body string) *httptest.Server {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/content/"+uuid+annotationsEndpoint, r.URL.Path)

		if apiKey := r.Header.Get(apiKeyHeader); apiKey != testAPIKey {
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte("unauthorized"))
			return
		}

		assert.Equal(t, testAPIKey, r.Header.Get(apiKeyHeader))
		assert.Equal(t, tid, r.Header.Get(tidUtils.TransactionIDHeader))
		assert.Equal(t, "PAC draft-annotations-api", r.Header.Get("User-Agent"))

		w.WriteHeader(status)
		w.Write([]byte(body))
	}))
	return ts
}

func newAnnotationsAPIGTGServerMock(t *testing.T, status int, body string) *httptest.Server {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/content/"+syntheticContentUUID+annotationsEndpoint, r.URL.Path)
		if apiKey := r.Header.Get(apiKeyHeader); apiKey != testAPIKey {
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte("unauthorized"))
			return
		}

		assert.Equal(t, testAPIKey, r.Header.Get(apiKeyHeader))
		assert.Equal(t, "PAC draft-annotations-api", r.Header.Get("User-Agent"))

		w.WriteHeader(status)
		w.Write([]byte(body))
	}))
	return ts
}
