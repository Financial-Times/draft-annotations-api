package annotations

import (
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"

	tidutils "github.com/Financial-Times/transactionid-utils-go"
	"github.com/husobee/vestigo"
	"github.com/stretchr/testify/assert"
)

const testAPIKey = "testAPIKey"
const testTID = "test_tid"

func TestHappyAnnotationsAPI(t *testing.T) {
	annotationsAPIServerMock := newAnnotationsAPIServerMock(t, http.StatusOK, annotationsBody)
	defer annotationsAPIServerMock.Close()

	annotationsAPI := NewAnnotationsAPI(annotationsAPIServerMock.URL+"/content/%v/annotations", testAPIKey)
	assert.Equal(t, annotationsAPIServerMock.URL+"/content/%v/annotations", annotationsAPI.Endpoint())

	h := NewHandler(annotationsAPI)
	r := vestigo.NewRouter()
	r.Get("/drafts/content/:uuid/annotations", h.ServeHTTP)

	req := httptest.NewRequest("GET", "http://api.ft.com/drafts/content/83a201c6-60cd-11e7-91a7-502f7ee26895/annotations", nil)
	req.Header.Set(tidutils.TransactionIDHeader, testTID)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)
	resp := w.Result()
	body, err := ioutil.ReadAll(resp.Body)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.NoError(t, err)
	assert.JSONEq(t, string(annotationsBody), string(body))
}

func TestAnnotationsAPI404(t *testing.T) {
	annotationsAPIServerMock := newAnnotationsAPIServerMock(t, http.StatusNotFound, "not found")
	defer annotationsAPIServerMock.Close()

	annotationsAPI := NewAnnotationsAPI(annotationsAPIServerMock.URL+"/content/%v/annotations", testAPIKey)
	h := NewHandler(annotationsAPI)
	r := vestigo.NewRouter()
	r.Get("/drafts/content/:uuid/annotations", h.ServeHTTP)

	req := httptest.NewRequest("GET", "http://api.ft.com/drafts/content/83a201c6-60cd-11e7-91a7-502f7ee26895/annotations", nil)
	req.Header.Set(tidutils.TransactionIDHeader, testTID)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)
	resp := w.Result()
	body, err := ioutil.ReadAll(resp.Body)

	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	assert.NoError(t, err)
	assert.Equal(t, "not found", string(body))
}

func TestAnnotationsAPI404NoAnnoPostMapping(t *testing.T) {
	annotationsAPIServerMock := newAnnotationsAPIServerMock(t, http.StatusOK, bannedAnnotationsBody)
	defer annotationsAPIServerMock.Close()

	annotationsAPI := NewAnnotationsAPI(annotationsAPIServerMock.URL+"/content/%v/annotations", testAPIKey)
	h := NewHandler(annotationsAPI)
	r := vestigo.NewRouter()
	r.Get("/drafts/content/:uuid/annotations", h.ServeHTTP)

	req := httptest.NewRequest("GET", "http://api.ft.com/drafts/content/83a201c6-60cd-11e7-91a7-502f7ee26895/annotations", nil)
	req.Header.Set(tidutils.TransactionIDHeader, testTID)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)
	resp := w.Result()
	body, err := ioutil.ReadAll(resp.Body)

	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	assert.NoError(t, err)
	assert.Equal(t, "{\"message\":\"No annotations can be found\"}", string(body))
}

func TestAnnotationsAPI500(t *testing.T) {
	annotationsAPIServerMock := newAnnotationsAPIServerMock(t, http.StatusInternalServerError, "fire!")
	defer annotationsAPIServerMock.Close()

	annotationsAPI := NewAnnotationsAPI(annotationsAPIServerMock.URL+"/content/%v/annotations", testAPIKey)
	h := NewHandler(annotationsAPI)
	r := vestigo.NewRouter()
	r.Get("/drafts/content/:uuid/annotations", h.ServeHTTP)

	req := httptest.NewRequest("GET", "http://api.ft.com/drafts/content/83a201c6-60cd-11e7-91a7-502f7ee26895/annotations", nil)
	req.Header.Set(tidutils.TransactionIDHeader, testTID)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)
	resp := w.Result()
	body, err := ioutil.ReadAll(resp.Body)

	assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)
	assert.NoError(t, err)
	assert.Equal(t, `{"message":"Service unavailable"}`, string(body))
}

func TestInvalidURL(t *testing.T) {
	annotationsAPI := NewAnnotationsAPI(":#", testAPIKey)
	h := NewHandler(annotationsAPI)
	r := vestigo.NewRouter()
	r.Get("/drafts/content/:uuid/annotations", h.ServeHTTP)

	req := httptest.NewRequest("GET", "http://api.ft.com/drafts/content/83a201c6-60cd-11e7-91a7-502f7ee26895/annotations", nil)
	req.Header.Set(tidutils.TransactionIDHeader, testTID)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)
	resp := w.Result()
	body, err := ioutil.ReadAll(resp.Body)

	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
	assert.NoError(t, err)
	assert.Equal(t, "parse :: missing protocol scheme\n", string(body))
}

func TestConnectionError(t *testing.T) {
	annotationsAPIServerMock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	annotationsAPIServerMock.Close()

	annotationsAPI := NewAnnotationsAPI(annotationsAPIServerMock.URL, testAPIKey)
	h := NewHandler(annotationsAPI)
	r := vestigo.NewRouter()
	r.Get("/drafts/content/:uuid/annotations", h.ServeHTTP)

	req := httptest.NewRequest("GET", "http://api.ft.com/drafts/content/83a201c6-60cd-11e7-91a7-502f7ee26895/annotations", nil)
	req.Header.Set(tidutils.TransactionIDHeader, testTID)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)
	resp := w.Result()
	_, err := ioutil.ReadAll(resp.Body)

	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
	assert.NoError(t, err)
}

func newAnnotationsAPIServerMock(t *testing.T, status int, body string) *httptest.Server {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if apiKey := r.Header.Get(apiKeyHeader); apiKey != testAPIKey {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		assert.Equal(t, testTID, r.Header.Get(tidutils.TransactionIDHeader))
		w.WriteHeader(status)
		w.Write([]byte(body))
	}))
	return ts
}

const bannedAnnotationsBody = `[
	{
		"predicate": "http://www.ft.com/ontology/classification/isClassifiedBy",
		"id": "http://api.ft.com/things/04789fc2-4598-3b95-9698-14e5ece17261",
		"apiUrl": "http://api.ft.com/things/04789fc2-4598-3b95-9698-14e5ece17261",
		"types": [
		  "http://www.ft.com/ontology/core/Thing",
		  "http://www.ft.com/ontology/concept/Concept",
		  "http://www.ft.com/ontology/classification/Classification",
		  "http://www.ft.com/ontology/SpecialReport"
		],
		"prefLabel": "Destination: North of England"
	}
]`

const annotationsBody = `[
   {
      "predicate": "http://www.ft.com/ontology/annotation/mentions",
      "id": "http://api.ft.com/things/0a619d71-9af5-3755-90dd-f789b686c67a",
      "apiUrl": "http://api.ft.com/people/0a619d71-9af5-3755-90dd-f789b686c67a",
      "types": [
         "http://www.ft.com/ontology/core/Thing",
         "http://www.ft.com/ontology/concept/Concept",
         "http://www.ft.com/ontology/person/Person"
      ],
      "prefLabel": "Barack H. Obama"
   },
   {
      "predicate": "http://www.ft.com/ontology/annotation/hasAuthor",
      "id": "http://api.ft.com/things/838b3fbe-efbc-3cfe-b5c0-d38c046492a4",
      "apiUrl": "http://api.ft.com/people/838b3fbe-efbc-3cfe-b5c0-d38c046492a4",
      "types": [
         "http://www.ft.com/ontology/core/Thing",
         "http://www.ft.com/ontology/concept/Concept",
         "http://www.ft.com/ontology/person/Person"
      ],
      "prefLabel": "David J Lynch"
   }
]`
