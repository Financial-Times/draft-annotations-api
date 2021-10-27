package annotations

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/Financial-Times/go-ft-http/fthttp"
	tidUtils "github.com/Financial-Times/transactionid-utils-go"
	"github.com/google/uuid"
	"github.com/husobee/vestigo"
	"github.com/stretchr/testify/assert"
)

const testAPIKey = "testAPIKey"

var testClient = fthttp.NewClientWithDefaultTimeout("PAC", "draft-annotations-api")

func TestHappyAnnotationsAPIGTG(t *testing.T) {
	annotationsServerMock := newAnnotationsAPIGTGServerMock(t, http.StatusOK, "I am happy!")
	defer annotationsServerMock.Close()

	annotationsAPI := NewUPPAnnotationsAPI(testClient, annotationsServerMock.URL+"/content/%v/annotations", testAPIKey)
	err := annotationsAPI.GTG()
	assert.NoError(t, err)
}

func TestUnhappyAnnotationsAPIGTG(t *testing.T) {
	annotationsServerMock := newAnnotationsAPIGTGServerMock(t, http.StatusServiceUnavailable, "I am not happy!")
	defer annotationsServerMock.Close()

	annotationsAPI := NewUPPAnnotationsAPI(testClient, annotationsServerMock.URL+"/content/%v/annotations", testAPIKey)
	err := annotationsAPI.GTG()
	assert.True(t, errors.Is(err, ErrGTGNotOK))
}

func TestAnnotationsAPIGTGWrongAPIKey(t *testing.T) {
	annotationsServerMock := newAnnotationsAPIGTGServerMock(t, http.StatusServiceUnavailable, "I not am happy!")
	defer annotationsServerMock.Close()

	annotationsAPI := NewUPPAnnotationsAPI(testClient, annotationsServerMock.URL+"/content/%v/annotations", "a-non-existing-key")
	err := annotationsAPI.GTG()
	assert.True(t, errors.Is(err, ErrGTGNotOK))
}

func TestAnnotationsAPIGTGInvalidURL(t *testing.T) {
	annotationsAPI := NewUPPAnnotationsAPI(testClient, ":#", testAPIKey)
	err := annotationsAPI.GTG()
	var urlErr *url.Error
	assert.True(t, errors.As(err, &urlErr))
	assert.Equal(t, urlErr.Op, "parse")
}

func TestAnnotationsAPIGTGConnectionError(t *testing.T) {
	annotationsServerMock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	annotationsServerMock.Close()

	annotationsAPI := NewUPPAnnotationsAPI(testClient, annotationsServerMock.URL+"/content/%v/annotations", testAPIKey)
	err := annotationsAPI.GTG()
	assert.Error(t, err)
}

func TestHappyAnnotationsAPI(t *testing.T) {
	uuid := uuid.New().String()
	tid := "tid_all-good"
	ctx := tidUtils.TransactionAwareContext(context.TODO(), tid)

	annotationsServerMock := newAnnotationsAPIServerMock(t, tid, uuid, "", http.StatusOK, "I am happy!")
	defer annotationsServerMock.Close()

	annotationsAPI := NewUPPAnnotationsAPI(testClient, annotationsServerMock.URL+"/content/%v/annotations", testAPIKey)
	resp, err := annotationsAPI.getUPPAnnotationsResponse(ctx, uuid)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestHappyAnnotationsAPIWithLifecycles(t *testing.T) {
	uuid := uuid.New().String()
	tid := "tid_all-good"
	ctx := tidUtils.TransactionAwareContext(context.TODO(), tid)

	annotationsServerMock := newAnnotationsAPIServerMock(t, tid, uuid, "lifecycle=pac&lifecycle=v1&lifecycle=next-video", http.StatusOK, "I am happy!")
	defer annotationsServerMock.Close()

	annotationsAPI := NewUPPAnnotationsAPI(testClient, annotationsServerMock.URL+"/content/%v/annotations", testAPIKey)
	resp, err := annotationsAPI.getUPPAnnotationsResponse(ctx, uuid, pacAnnotationLifecycle, v1AnnotationLifecycle, nextVideoAnnotationLifecycle)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestUnhappyAnnotationsAPI(t *testing.T) {
	uuid := uuid.New().String()
	tid := "tid_all-good?"
	ctx := tidUtils.TransactionAwareContext(context.TODO(), tid)

	annotationsServerMock := newAnnotationsAPIServerMock(t, tid, uuid, "", http.StatusServiceUnavailable, "I am definitely not happy!")
	defer annotationsServerMock.Close()

	annotationsAPI := NewUPPAnnotationsAPI(testClient, annotationsServerMock.URL+"/content/%v/annotations", testAPIKey)
	resp, err := annotationsAPI.getUPPAnnotationsResponse(ctx, uuid)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)
}

func TestNoTIDAnnotationsAPI(t *testing.T) {
	uuid := uuid.New().String()
	annotationsServerMock := newAnnotationsAPIServerMock(t, "", uuid, "", http.StatusServiceUnavailable, "I am definitely not happy!")
	defer annotationsServerMock.Close()

	annotationsAPI := NewUPPAnnotationsAPI(testClient, annotationsServerMock.URL+"/content/%v/annotations", testAPIKey)
	resp, err := annotationsAPI.getUPPAnnotationsResponse(context.TODO(), uuid)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)
}

func TestRequestFailsAnnotationsAPI(t *testing.T) {
	annotationsAPI := NewUPPAnnotationsAPI(testClient, ":#", testAPIKey)
	resp, err := annotationsAPI.getUPPAnnotationsResponse(context.TODO(), "")

	assert.Error(t, err)
	assert.Nil(t, resp)
}

func TestResponseFailsAnnotationsAPI(t *testing.T) {
	annotationsAPI := NewUPPAnnotationsAPI(testClient, "#:", testAPIKey)
	resp, err := annotationsAPI.getUPPAnnotationsResponse(context.TODO(), "")

	assert.Error(t, err)
	assert.Nil(t, resp)
}

func TestAnnotationsAPITimeout(t *testing.T) {
	r := vestigo.NewRouter()
	r.Get("/content/:uuid/annotations", func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(500 * time.Millisecond)
	})

	s := httptest.NewServer(r)
	annotationsAPI := NewUPPAnnotationsAPI(testClient, s.URL+"/content/%v/annotations", testAPIKey)

	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond*50)
	defer cancel()

	_, err := annotationsAPI.getUPPAnnotationsResponse(ctx, testContentUUID)
	assert.Error(t, err)
	assert.True(t, (err.(net.Error)).Timeout())
}

func TestGetAnnotationsHappy(t *testing.T) {

	var testCases = []struct {
		name                string
		annotationsStatus   int
		annotationsBody     string
		expectedAnnotations []Annotation
		expectError         bool
		expectedError       error
	}{
		{
			name:              "happy case",
			annotationsStatus: http.StatusOK,
			annotationsBody: `[{
				"predicate": "http://www.ft.com/ontology/annotation/about",
				"id": "http://api.ft.com/things/dd158946-e88b-3a85-abe4-5848319501ce",
				"apiUrl": "http://api.ft.com/things/dd158946-e88b-3a85-abe4-5848319501ce",
	        	"types": [
	            	"http://www.ft.com/ontology/core/Thing",
	            	"http://www.ft.com/ontology/concept/Concept",
	            	"http://www.ft.com/ontology/Location"
	        	],
	        	"prefLabel": "Canada"
	    	},
	    	{
	        	"predicate": "http://www.ft.com/ontology/classification/isClassifiedBy",
	        	"id": "http://api.ft.com/things/a579350c-61ce-4c00-97ca-ddaa2e0cacf6",
	        	"apiUrl": "http://api.ft.com/things/a579350c-61ce-4c00-97ca-ddaa2e0cacf6",
	        	"types": [
	            	"http://www.ft.com/ontology/core/Thing",
	            	"http://www.ft.com/ontology/concept/Concept",
	            	"http://www.ft.com/ontology/classification/Classification",
	            	"http://www.ft.com/ontology/Genre"
	        	],
	        	"prefLabel": "News"
	    	}]`,
			expectedAnnotations: []Annotation{
				{
					Predicate: "http://www.ft.com/ontology/annotation/about",
					ConceptId: "http://www.ft.com/thing/dd158946-e88b-3a85-abe4-5848319501ce",
					ApiUrl:    "http://api.ft.com/things/dd158946-e88b-3a85-abe4-5848319501ce",
					Type:      "http://www.ft.com/ontology/Location",
					PrefLabel: "Canada",
				},
				{
					Predicate: "http://www.ft.com/ontology/classification/isClassifiedBy",
					ConceptId: "http://www.ft.com/thing/a579350c-61ce-4c00-97ca-ddaa2e0cacf6",
					ApiUrl:    "http://api.ft.com/things/a579350c-61ce-4c00-97ca-ddaa2e0cacf6",
					Type:      "http://www.ft.com/ontology/Genre",
					PrefLabel: "News",
				},
			},
			expectError:   false,
			expectedError: nil,
		},
		{
			name:                "empty body",
			annotationsStatus:   200,
			annotationsBody:     "[]",
			expectedAnnotations: nil,
			expectError:         true,
			expectedError:       UPPError{msg: NoAnnotationsMsg, status: http.StatusNotFound, uppBody: nil},
		},
		{
			name:                "bad request",
			annotationsStatus:   http.StatusBadRequest,
			annotationsBody:     "[]",
			expectedAnnotations: nil,
			expectError:         true,
			expectedError:       UPPError{msg: UPPBadRequestMsg, status: http.StatusBadRequest, uppBody: []byte("[]")},
		},
		{
			name:                "not found",
			annotationsStatus:   http.StatusNotFound,
			annotationsBody:     "[]",
			expectedAnnotations: nil,
			expectError:         true,
			expectedError:       UPPError{msg: UPPNotFoundMsg, status: http.StatusNotFound, uppBody: []byte("[]")},
		},
		{
			name:                "server error",
			annotationsStatus:   http.StatusInternalServerError,
			annotationsBody:     "[]",
			expectedAnnotations: nil,
			expectError:         true,
			expectedError:       UPPError{msg: UPPServiceUnavailableMsg, status: http.StatusServiceUnavailable, uppBody: nil},
		},
	}

	for _, test := range testCases {
		t.Run(test.name, func(t *testing.T) {
			uuid := uuid.New().String()
			tid := "tid_all-good"
			ctx := tidUtils.TransactionAwareContext(context.TODO(), tid)

			annotationsServerMock := newAnnotationsAPIServerMock(t, tid, uuid, "", test.annotationsStatus, test.annotationsBody)
			defer annotationsServerMock.Close()

			annotationsAPI := NewUPPAnnotationsAPI(testClient, annotationsServerMock.URL+"/content/%v/annotations", testAPIKey)
			annotations, err := annotationsAPI.getAnnotations(ctx, uuid)

			assert.ElementsMatch(t, annotations, test.expectedAnnotations)
			if !test.expectError {
				assert.NoError(t, err)
			} else {
				assert.EqualValues(t, err, test.expectedError)
			}
		})
	}
}

func newAnnotationsAPIServerMock(t *testing.T, tid string, uuid string, lifecycles string, status int, body string) *httptest.Server {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/content/"+uuid+annotationsEndpoint, r.URL.Path)
		assert.Equal(t, lifecycles, r.URL.RawQuery)

		if apiKey := r.Header.Get(apiKeyHeader); apiKey != testAPIKey {
			w.WriteHeader(http.StatusUnauthorized)
			_, err := w.Write([]byte("unauthorized"))
			if err != nil {
				t.Fatalf("write error: %v", err)
			}
			return
		}

		assert.Equal(t, testAPIKey, r.Header.Get(apiKeyHeader))
		assert.Equal(t, tid, r.Header.Get(tidUtils.TransactionIDHeader))
		assert.Equal(t, "PAC-draft-annotations-api/Version--is-not-a-semantic-version", r.Header.Get("User-Agent"))

		w.WriteHeader(status)
		_, err := w.Write([]byte(body))
		if err != nil {
			t.Fatalf("write error: %v", err)
		}
	}))
	return ts
}

func newAnnotationsAPIGTGServerMock(t *testing.T, status int, body string) *httptest.Server {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/content/"+syntheticContentUUID+annotationsEndpoint, r.URL.Path)
		if apiKey := r.Header.Get(apiKeyHeader); apiKey != testAPIKey {
			w.WriteHeader(http.StatusUnauthorized)
			_, err := w.Write([]byte("unauthorized"))
			if err != nil {
				t.Fatalf("write error: %v", err)
			}
			return
		}

		assert.Equal(t, testAPIKey, r.Header.Get(apiKeyHeader))
		assert.Equal(t, "PAC-draft-annotations-api/Version--is-not-a-semantic-version", r.Header.Get("User-Agent"))

		w.WriteHeader(status)
		_, err := w.Write([]byte(body))
		if err != nil {
			t.Fatalf("write error: %v", err)
		}
	}))
	return ts
}
