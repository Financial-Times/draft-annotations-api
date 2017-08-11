package annotations

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHappyAnnotationsAPIGTG(t *testing.T) {
	annotationsServerMock := newAnnotationsAPIGTGServerMock(t, http.StatusOK, "I am happy!")
	defer annotationsServerMock.Close()

	annotationsAPI := NewAnnotationsAPI(annotationsServerMock.URL+"/content/%v/annotations", testAPIKey)
	err := annotationsAPI.GTG()
	assert.NoError(t, err)
}

func TestUnhappyAnnotationsAPIGTG(t *testing.T) {
	annotationsServerMock := newAnnotationsAPIGTGServerMock(t, http.StatusServiceUnavailable, "I am not happy!")
	defer annotationsServerMock.Close()

	annotationsAPI := NewAnnotationsAPI(annotationsServerMock.URL+"/content/%v/annotations", testAPIKey)
	err := annotationsAPI.GTG()
	assert.EqualError(t, err, "gtg returned a non-200 HTTP status [503]: I am not happy!")
}

func TestAnnotationsAPIGTGWrongAPIKey(t *testing.T) {
	annotationsServerMock := newAnnotationsAPIGTGServerMock(t, http.StatusServiceUnavailable, "I not am happy!")
	defer annotationsServerMock.Close()

	annotationsAPI := NewAnnotationsAPI(annotationsServerMock.URL+"/content/%v/annotations", "a-non-existing-key")
	err := annotationsAPI.GTG()
	assert.EqualError(t, err, "gtg returned a non-200 HTTP status [401]: unauthorized")
}

func TestAnnotationsAPIGTGInvalidURL(t *testing.T) {
	annotationsAPI := NewAnnotationsAPI(":#", testAPIKey)
	err := annotationsAPI.GTG()
	assert.EqualError(t, err, "gtg request error: parse :: missing protocol scheme")
}

func TestAnnotationsAPIGTGConnectionError(t *testing.T) {
	annotationsServerMock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	annotationsServerMock.Close()

	annotationsAPI := NewAnnotationsAPI(annotationsServerMock.URL+"/content/%v/annotations", testAPIKey)
	err := annotationsAPI.GTG()
	assert.Error(t, err)
}

func newAnnotationsAPIGTGServerMock(t *testing.T, status int, body string) *httptest.Server {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/content/"+syntheticContentUUID+annotationsEndpoint, r.URL.Path)
		if apiKey := r.Header.Get(apiKeyHeader); apiKey != testAPIKey {
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte("unauthorized"))
			return
		}
		w.WriteHeader(status)
		w.Write([]byte(body))
	}))
	return ts
}
