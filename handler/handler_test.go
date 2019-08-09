package handler

import (
	"bytes"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"context"
	"encoding/json"
	"errors"

	"github.com/Financial-Times/draft-annotations-api/annotations"
	"github.com/Financial-Times/go-ft-http/fthttp"
	tidutils "github.com/Financial-Times/transactionid-utils-go"
	randomdata "github.com/Pallinder/go-randomdata"
	"github.com/husobee/vestigo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

const testAPIKey = "testAPIKey"
const testTID = "test_tid"

const apiKeyHeader = "X-Api-Key"

var testClient = fthttp.NewClientWithDefaultTimeout("PAC", "draft-annotations-api")

func TestHappyFetchFromAnnotationsRW(t *testing.T) {
	hash := randomdata.RandStringRunes(56)

	rw := new(RWMock)
	rw.On("Read", mock.Anything, "83a201c6-60cd-11e7-91a7-502f7ee26895").Return(&expectedAnnotations, hash, true, nil)
	aug := new(AugmenterMock)
	aug.On("AugmentAnnotations", mock.Anything, expectedAnnotations.Annotations).Return(expectedAnnotations.Annotations, nil)
	annAPI := new(AnnotationsAPIMock)

	h := New(rw, annAPI, nil, aug, time.Second)
	r := vestigo.NewRouter()
	r.Get("/drafts/content/:uuid/annotations", h.ReadAnnotations)

	req := httptest.NewRequest("GET", "http://api.ft.com/drafts/content/83a201c6-60cd-11e7-91a7-502f7ee26895/annotations", nil)
	req.Header.Set(tidutils.TransactionIDHeader, testTID)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)
	resp := w.Result()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	actual := annotations.Annotations{}
	err := json.NewDecoder(resp.Body).Decode(&actual)
	assert.NoError(t, err)

	assert.Equal(t, expectedAnnotations, actual)
	assert.Equal(t, hash, resp.Header.Get(annotations.DocumentHashHeader))

	rw.AssertExpectations(t)
	aug.AssertExpectations(t)
	annAPI.AssertExpectations(t)
}

func TestUnHappyFetchFromAnnotationsRW(t *testing.T) {
	rw := new(RWMock)
	rw.On("Read", mock.Anything, "83a201c6-60cd-11e7-91a7-502f7ee26895").Return(nil, "", false, errors.New("computer says no"))
	aug := new(AugmenterMock)
	annAPI := new(AnnotationsAPIMock)

	h := New(rw, annAPI, nil, aug, time.Second)
	r := vestigo.NewRouter()
	r.Get("/drafts/content/:uuid/annotations", h.ReadAnnotations)

	req := httptest.NewRequest("GET", "http://api.ft.com/drafts/content/83a201c6-60cd-11e7-91a7-502f7ee26895/annotations", nil)
	req.Header.Set(tidutils.TransactionIDHeader, testTID)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)
	resp := w.Result()
	body, err := ioutil.ReadAll(resp.Body)

	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
	assert.NoError(t, err)
	assert.Equal(t, `{"message":"Failed to read annotations: computer says no"}`, string(body))
	assert.Empty(t, resp.Header.Get(annotations.DocumentHashHeader))

	rw.AssertExpectations(t)
	aug.AssertExpectations(t)
	annAPI.AssertExpectations(t)
}

func TestUnHappyAugmenter(t *testing.T) {
	rw := new(RWMock)
	rw.On("Read", mock.Anything, "83a201c6-60cd-11e7-91a7-502f7ee26895").Return(&expectedAnnotations, "", true, nil)
	aug := new(AugmenterMock)
	aug.On("AugmentAnnotations", mock.Anything, expectedAnnotations.Annotations).Return([]annotations.Annotation{}, errors.New("computer says no"))
	annAPI := new(AnnotationsAPIMock)

	h := New(rw, annAPI, nil, aug, time.Second)
	r := vestigo.NewRouter()
	r.Get("/drafts/content/:uuid/annotations", h.ReadAnnotations)

	req := httptest.NewRequest("GET", "http://api.ft.com/drafts/content/83a201c6-60cd-11e7-91a7-502f7ee26895/annotations", nil)
	req.Header.Set(tidutils.TransactionIDHeader, testTID)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)
	resp := w.Result()
	body, err := ioutil.ReadAll(resp.Body)

	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
	assert.NoError(t, err)
	assert.Equal(t, `{"message":"Failed to read annotations: computer says no"}`, string(body))
	assert.Empty(t, resp.Header.Get(annotations.DocumentHashHeader))

	rw.AssertExpectations(t)
	aug.AssertExpectations(t)
	annAPI.AssertExpectations(t)
}

func TestFetchFromAnnotationsAPIIfNotFoundInRW(t *testing.T) {
	aug := new(AugmenterMock)
	aug.On("AugmentAnnotations", mock.Anything, expectedAnnotations.Annotations).Return(expectedAnnotations.Annotations, nil)

	rw := new(RWMock)

	rw.On("Read", mock.Anything, "83a201c6-60cd-11e7-91a7-502f7ee26895").Return(nil, "", false, nil)

	annotationsAPIServerMock := newAnnotationsAPIServerMock(t, http.StatusOK, annotationsAPIBody)
	defer annotationsAPIServerMock.Close()

	annotationsAPI := annotations.NewUPPAnnotationsAPI(testClient, annotationsAPIServerMock.URL+"/content/%v/annotations", testAPIKey)
	assert.Equal(t, annotationsAPIServerMock.URL+"/content/%v/annotations", annotationsAPI.Endpoint())

	h := New(rw, annotationsAPI, nil, aug, time.Second)
	r := vestigo.NewRouter()
	r.Get("/drafts/content/:uuid/annotations", h.ReadAnnotations)

	req := httptest.NewRequest("GET", "http://api.ft.com/drafts/content/83a201c6-60cd-11e7-91a7-502f7ee26895/annotations", nil)
	req.Header.Set(tidutils.TransactionIDHeader, testTID)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)
	resp := w.Result()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	actual := annotations.Annotations{}
	err := json.NewDecoder(resp.Body).Decode(&actual)
	assert.NoError(t, err)

	assert.Equal(t, expectedAnnotations, actual)
	assert.Empty(t, resp.Header.Get(annotations.DocumentHashHeader))

	rw.AssertExpectations(t)
	aug.AssertExpectations(t)
}

func TestFetchFromAnnotationsAPI404(t *testing.T) {
	aug := new(AugmenterMock)
	rw := new(RWMock)
	rw.On("Read", mock.Anything, "83a201c6-60cd-11e7-91a7-502f7ee26895").Return(nil, "", false, nil)

	annotationsAPIServerMock := newAnnotationsAPIServerMock(t, http.StatusNotFound, "not found")
	defer annotationsAPIServerMock.Close()

	annotationsAPI := annotations.NewUPPAnnotationsAPI(testClient, annotationsAPIServerMock.URL+"/content/%v/annotations", testAPIKey)
	h := New(rw, annotationsAPI, nil, aug, time.Second)
	r := vestigo.NewRouter()
	r.Get("/drafts/content/:uuid/annotations", h.ReadAnnotations)

	req := httptest.NewRequest("GET", "http://api.ft.com/drafts/content/83a201c6-60cd-11e7-91a7-502f7ee26895/annotations", nil)
	req.Header.Set(tidutils.TransactionIDHeader, testTID)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)
	resp := w.Result()
	body, err := ioutil.ReadAll(resp.Body)

	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	assert.NoError(t, err)
	assert.Equal(t, "not found", string(body))

	rw.AssertExpectations(t)
	aug.AssertExpectations(t)
}

func TestFetchFromAnnotationsAPI404NoAnnoPostMapping(t *testing.T) {
	rw := new(RWMock)
	rw.On("Read", mock.Anything, "83a201c6-60cd-11e7-91a7-502f7ee26895").Return(nil, "", false, nil)
	aug := new(AugmenterMock)

	annotationsAPIServerMock := newAnnotationsAPIServerMock(t, http.StatusOK, bannedAnnotationsAPIBody)
	defer annotationsAPIServerMock.Close()

	annotationsAPI := annotations.NewUPPAnnotationsAPI(testClient, annotationsAPIServerMock.URL+"/content/%v/annotations", testAPIKey)
	h := New(rw, annotationsAPI, nil, aug, time.Second)
	r := vestigo.NewRouter()
	r.Get("/drafts/content/:uuid/annotations", h.ReadAnnotations)

	req := httptest.NewRequest("GET", "http://api.ft.com/drafts/content/83a201c6-60cd-11e7-91a7-502f7ee26895/annotations", nil)
	req.Header.Set(tidutils.TransactionIDHeader, testTID)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)
	resp := w.Result()
	body, err := ioutil.ReadAll(resp.Body)

	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	assert.NoError(t, err)
	assert.Equal(t, "{\"message\":\"No annotations found\"}", string(body))

	rw.AssertExpectations(t)
	aug.AssertExpectations(t)
}

func TestFetchFromAnnotationsAPI500(t *testing.T) {
	rw := new(RWMock)
	rw.On("Read", mock.Anything, "83a201c6-60cd-11e7-91a7-502f7ee26895").Return(nil, "", false, nil)
	aug := new(AugmenterMock)
	annotationsAPIServerMock := newAnnotationsAPIServerMock(t, http.StatusInternalServerError, "fire!")
	defer annotationsAPIServerMock.Close()

	annotationsAPI := annotations.NewUPPAnnotationsAPI(testClient, annotationsAPIServerMock.URL+"/content/%v/annotations", testAPIKey)
	h := New(rw, annotationsAPI, nil, aug, time.Second)
	r := vestigo.NewRouter()
	r.Get("/drafts/content/:uuid/annotations", h.ReadAnnotations)

	req := httptest.NewRequest("GET", "http://api.ft.com/drafts/content/83a201c6-60cd-11e7-91a7-502f7ee26895/annotations", nil)
	req.Header.Set(tidutils.TransactionIDHeader, testTID)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)
	resp := w.Result()
	body, err := ioutil.ReadAll(resp.Body)

	assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)
	assert.NoError(t, err)
	assert.Equal(t, `{"message":"Service unavailable"}`, string(body))

	rw.AssertExpectations(t)
	aug.AssertExpectations(t)
}

func TestFetchFromAnnotationsAPIWithInvalidURL(t *testing.T) {
	rw := new(RWMock)
	rw.On("Read", mock.Anything, "83a201c6-60cd-11e7-91a7-502f7ee26895").Return(nil, "", false, nil)
	aug := new(AugmenterMock)
	annotationsAPI := annotations.NewUPPAnnotationsAPI(testClient, ":#", testAPIKey)
	h := New(rw, annotationsAPI, nil, aug, time.Second)
	r := vestigo.NewRouter()
	r.Get("/drafts/content/:uuid/annotations", h.ReadAnnotations)

	req := httptest.NewRequest("GET", "http://api.ft.com/drafts/content/83a201c6-60cd-11e7-91a7-502f7ee26895/annotations", nil)
	req.Header.Set(tidutils.TransactionIDHeader, testTID)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)
	resp := w.Result()
	body, err := ioutil.ReadAll(resp.Body)

	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
	assert.NoError(t, err)
	assert.JSONEq(t, "{\"message\":\"Failed to read annotations: parse :: missing protocol scheme\"}", string(body))

	rw.AssertExpectations(t)
	aug.AssertExpectations(t)
}

func TestFetchFromAnnotationsAPIWithConnectionError(t *testing.T) {
	rw := new(RWMock)
	rw.On("Read", mock.Anything, "83a201c6-60cd-11e7-91a7-502f7ee26895").Return(nil, "", false, nil)
	aug := new(AugmenterMock)
	annotationsAPIServerMock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	annotationsAPIServerMock.Close()

	annotationsAPI := annotations.NewUPPAnnotationsAPI(testClient, annotationsAPIServerMock.URL, testAPIKey)
	h := New(rw, annotationsAPI, nil, aug, time.Second)
	r := vestigo.NewRouter()
	r.Get("/drafts/content/:uuid/annotations", h.ReadAnnotations)

	req := httptest.NewRequest("GET", "http://api.ft.com/drafts/content/83a201c6-60cd-11e7-91a7-502f7ee26895/annotations", nil)
	req.Header.Set(tidutils.TransactionIDHeader, testTID)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)
	resp := w.Result()
	_, err := ioutil.ReadAll(resp.Body)

	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
	assert.NoError(t, err)

	rw.AssertExpectations(t)
	aug.AssertExpectations(t)
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

const bannedAnnotationsAPIBody = `[
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

const annotationsAPIBody = `[
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
   },
   {
      "predicate": "http://www.ft.com/ontology/annotation/mentions",
      "id": "http://api.ft.com/thing/d33215f2-9804-3e4b-9774-736b749f6472",
      "apiUrl": "http://api.ft.com/concepts/d33215f2-9804-3e4b-9774-736b749f6472",
      "types": [
         "http://www.ft.com/ontology/core/Thing",
         "http://www.ft.com/ontology/concept/Concept",
         "http://www.ft.com/ontology/person/Person"
      ],
      "prefLabel": "John Ridding"
   }
]`

const expectedAnnotationsBody = `[
   {
      "predicate": "http://www.ft.com/ontology/annotation/mentions",
      "id": "http://www.ft.com/thing/0a619d71-9af5-3755-90dd-f789b686c67a",
      "apiUrl": "http://api.ft.com/people/0a619d71-9af5-3755-90dd-f789b686c67a",
      "type": "http://www.ft.com/ontology/person/Person",
      "prefLabel": "Barack H. Obama"
   },
   {
      "predicate": "http://www.ft.com/ontology/annotation/hasAuthor",
      "id": "http://www.ft.com/thing/838b3fbe-efbc-3cfe-b5c0-d38c046492a4",
      "apiUrl": "http://api.ft.com/people/838b3fbe-efbc-3cfe-b5c0-d38c046492a4",
      "type": "http://www.ft.com/ontology/person/Person",
      "prefLabel": "David J Lynch"
   },
   {
      "predicate": "http://www.ft.com/ontology/annotation/mentions",
      "id": "http://www.ft.com/thing/d33215f2-9804-3e4b-9774-736b749f6472",
      "apiUrl": "http://api.ft.com/concepts/d33215f2-9804-3e4b-9774-736b749f6472",
      "type": "http://www.ft.com/ontology/person/Person",
      "prefLabel": "John Ridding"
   }
]`

var expectedAnnotations = annotations.Annotations{
	Annotations: []annotations.Annotation{
		{
			Predicate: "http://www.ft.com/ontology/annotation/mentions",
			ConceptId: "http://www.ft.com/thing/0a619d71-9af5-3755-90dd-f789b686c67a",
			ApiUrl:    "http://api.ft.com/people/0a619d71-9af5-3755-90dd-f789b686c67a",
			Type:      "http://www.ft.com/ontology/person/Person",
			PrefLabel: "Barack H. Obama",
		},
		{
			Predicate: "http://www.ft.com/ontology/annotation/hasAuthor",
			ConceptId: "http://www.ft.com/thing/838b3fbe-efbc-3cfe-b5c0-d38c046492a4",
			ApiUrl:    "http://api.ft.com/people/838b3fbe-efbc-3cfe-b5c0-d38c046492a4",
			Type:      "http://www.ft.com/ontology/person/Person",
			PrefLabel: "David J Lynch",
		},
		{
			Predicate: "http://www.ft.com/ontology/annotation/mentions",
			ConceptId: "http://www.ft.com/thing/d33215f2-9804-3e4b-9774-736b749f6472",
			ApiUrl:    "http://api.ft.com/concepts/d33215f2-9804-3e4b-9774-736b749f6472",
			Type:      "http://www.ft.com/ontology/person/Person",
			PrefLabel: "John Ridding",
		},
	},
}

var expectedCanonicalisedAnnotationsBody = annotations.Annotations{
	Annotations: []annotations.Annotation{
		{
			Predicate: "http://www.ft.com/ontology/annotation/hasAuthor",
			ConceptId: "http://www.ft.com/thing/838b3fbe-efbc-3cfe-b5c0-d38c046492a4",
		},
		{
			Predicate: "http://www.ft.com/ontology/annotation/mentions",
			ConceptId: "http://www.ft.com/thing/0a619d71-9af5-3755-90dd-f789b686c67a",
		},
		{
			Predicate: "http://www.ft.com/ontology/annotation/mentions",
			ConceptId: "http://www.ft.com/thing/d33215f2-9804-3e4b-9774-736b749f6472",
		},
	},
}

var expectedCanonicalisedAnnotationsAfterDelete = annotations.Annotations{
	Annotations: []annotations.Annotation{
		{
			Predicate: "http://www.ft.com/ontology/annotation/hasAuthor",
			ConceptId: "http://www.ft.com/thing/838b3fbe-efbc-3cfe-b5c0-d38c046492a4",
		},
		{
			Predicate: "http://www.ft.com/ontology/annotation/mentions",
			ConceptId: "http://www.ft.com/thing/d33215f2-9804-3e4b-9774-736b749f6472",
		},
	},
}

func TestSaveAnnotations(t *testing.T) {
	oldHash := randomdata.RandStringRunes(56)
	newHash := randomdata.RandStringRunes(56)
	rw := new(RWMock)
	rw.On("Write", mock.AnythingOfType("*context.valueCtx"), "83a201c6-60cd-11e7-91a7-502f7ee26895", &expectedCanonicalisedAnnotationsBody, oldHash).Return(newHash, nil)

	aug := new(AugmenterMock)
	annotationsAPI := new(AnnotationsAPIMock)

	h := New(rw, annotationsAPI, annotations.NewCanonicalizer(annotations.NewCanonicalAnnotationSorter), aug, time.Second)
	r := vestigo.NewRouter()
	r.Put("/drafts/content/:uuid/annotations", h.WriteAnnotations)

	entity := bytes.Buffer{}
	err := json.NewEncoder(&entity).Encode(&expectedAnnotations)
	if err != nil {
		t.Fatalf("failed to encode annotations: %v", err)
	}

	req := httptest.NewRequest(
		"PUT",
		"http://api.ft.com/drafts/content/83a201c6-60cd-11e7-91a7-502f7ee26895/annotations",
		&entity)

	req.Header.Set(tidutils.TransactionIDHeader, testTID)
	req.Header.Set(annotations.PreviousDocumentHashHeader, oldHash)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)
	resp := w.Result()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	actual := annotations.Annotations{}
	err = json.NewDecoder(resp.Body).Decode(&actual)
	assert.NoError(t, err)

	assert.Equal(t, expectedCanonicalisedAnnotationsBody, actual)
	assert.Equal(t, newHash, resp.Header.Get(annotations.DocumentHashHeader))

	rw.AssertExpectations(t)
	aug.AssertExpectations(t)
	annotationsAPI.AssertExpectations(t)
}

func TestSaveAnnotationsInvalidContentUUID(t *testing.T) {
	oldHash := randomdata.RandStringRunes(56)
	rw := new(RWMock)
	aug := new(AugmenterMock)
	annotationsAPI := new(AnnotationsAPIMock)

	h := New(rw, annotationsAPI, annotations.NewCanonicalizer(annotations.NewCanonicalAnnotationSorter), aug, time.Second)
	r := vestigo.NewRouter()
	r.Put("/drafts/content/:uuid/annotations", h.WriteAnnotations)

	req := httptest.NewRequest(
		"PUT",
		"http://api.ft.com/drafts/content/not-a-valid-uuid/annotations",
		strings.NewReader(expectedAnnotationsBody))

	req.Header.Set(tidutils.TransactionIDHeader, testTID)
	req.Header.Set(annotations.PreviousDocumentHashHeader, oldHash)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)
	resp := w.Result()
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	body, err := ioutil.ReadAll(resp.Body)
	assert.NoError(t, err)
	assert.JSONEq(t, `{"message":"Invalid content UUID: uuid: UUID string too short: not-a-valid-uuid"}`, string(body))

	rw.AssertExpectations(t)
	aug.AssertExpectations(t)
	annotationsAPI.AssertExpectations(t)
}

func TestSaveAnnotationsInvalidAnnotationsBody(t *testing.T) {
	oldHash := randomdata.RandStringRunes(56)
	rw := new(RWMock)
	aug := new(AugmenterMock)
	annotationsAPI := new(AnnotationsAPIMock)

	h := New(rw, annotationsAPI, annotations.NewCanonicalizer(annotations.NewCanonicalAnnotationSorter), aug, time.Second)
	r := vestigo.NewRouter()
	r.Put("/drafts/content/:uuid/annotations", h.WriteAnnotations)

	req := httptest.NewRequest(
		"PUT",
		"http://api.ft.com/drafts/content/83a201c6-60cd-11e7-91a7-502f7ee26895/annotations",
		strings.NewReader(`{invalid-json}`))

	req.Header.Set(tidutils.TransactionIDHeader, testTID)
	req.Header.Set(annotations.PreviousDocumentHashHeader, oldHash)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)
	resp := w.Result()
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	body, err := ioutil.ReadAll(resp.Body)
	assert.NoError(t, err)
	assert.JSONEq(t, `{"message":"Unable to unmarshal annotations body: invalid character 'i' looking for beginning of object key string"}`, string(body))

	rw.AssertExpectations(t)
	aug.AssertExpectations(t)
	annotationsAPI.AssertExpectations(t)
}

func TestSaveAnnotationsErrorFromRW(t *testing.T) {
	oldHash := randomdata.RandStringRunes(56)
	rw := new(RWMock)
	rw.On("Write", mock.AnythingOfType("*context.valueCtx"), "83a201c6-60cd-11e7-91a7-502f7ee26895", &expectedCanonicalisedAnnotationsBody, oldHash).Return("", errors.New("computer says no"))

	aug := new(AugmenterMock)
	annotationsAPI := new(AnnotationsAPIMock)

	h := New(rw, annotationsAPI, annotations.NewCanonicalizer(annotations.NewCanonicalAnnotationSorter), aug, time.Second)
	r := vestigo.NewRouter()
	r.Put("/drafts/content/:uuid/annotations", h.WriteAnnotations)

	entity := bytes.Buffer{}
	err := json.NewEncoder(&entity).Encode(&expectedAnnotations)
	if err != nil {
		t.Fatalf("failed to encode annotations: %v", err)
	}

	req := httptest.NewRequest(
		"PUT",
		"http://api.ft.com/drafts/content/83a201c6-60cd-11e7-91a7-502f7ee26895/annotations",
		&entity)

	req.Header.Set(tidutils.TransactionIDHeader, testTID)
	req.Header.Set(annotations.PreviousDocumentHashHeader, oldHash)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)
	resp := w.Result()
	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
	body, err := ioutil.ReadAll(resp.Body)
	assert.NoError(t, err)
	assert.JSONEq(t, `{"message":"Error in writing draft annotations: computer says no"}`, string(body))

	rw.AssertExpectations(t)
	aug.AssertExpectations(t)
	annotationsAPI.AssertExpectations(t)
}

func TestAnnotationsReadTimeoutGenericRW(t *testing.T) {
	rw := new(RWMock)
	rw.On("Read", mock.Anything, "83a201c6-60cd-11e7-91a7-502f7ee26895").Return(nil, "", false, &url.Error{Err: context.DeadlineExceeded})

	aug := new(AugmenterMock)
	annAPI := new(AnnotationsAPIMock)

	h := New(rw, annAPI, nil, aug, time.Second)
	r := vestigo.NewRouter()
	r.Get("/drafts/content/:uuid/annotations", h.ReadAnnotations)

	req := httptest.NewRequest("GET", "http://api.ft.com/drafts/content/83a201c6-60cd-11e7-91a7-502f7ee26895/annotations", nil)
	req.Header.Set(tidutils.TransactionIDHeader, testTID)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)
	resp := w.Result()
	assert.Equal(t, http.StatusGatewayTimeout, resp.StatusCode)
	assert.JSONEq(t, `{"message":"Timeout while reading annotations"}`, w.Body.String())

	rw.AssertExpectations(t)
	aug.AssertExpectations(t)
	annAPI.AssertExpectations(t)
}

func TestAnnotationsReadTimeoutUPP(t *testing.T) {
	rw := new(RWMock)
	rw.On("Read", mock.Anything, "83a201c6-60cd-11e7-91a7-502f7ee26895").Return(nil, "", false, nil)

	aug := new(AugmenterMock)
	annAPI := new(AnnotationsAPIMock)
	annAPI.On("GetAll", mock.Anything, "83a201c6-60cd-11e7-91a7-502f7ee26895").Return([]annotations.Annotation{}, &url.Error{Err: context.DeadlineExceeded})

	h := New(rw, annAPI, nil, aug, time.Second)
	r := vestigo.NewRouter()
	r.Get("/drafts/content/:uuid/annotations", h.ReadAnnotations)

	req := httptest.NewRequest("GET", "http://api.ft.com/drafts/content/83a201c6-60cd-11e7-91a7-502f7ee26895/annotations", nil)
	req.Header.Set(tidutils.TransactionIDHeader, testTID)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)
	resp := w.Result()
	assert.Equal(t, http.StatusGatewayTimeout, resp.StatusCode)
	assert.JSONEq(t, `{"message":"Timeout while reading annotations"}`, w.Body.String())

	rw.AssertExpectations(t)
	aug.AssertExpectations(t)
	annAPI.AssertExpectations(t)
}

func TestIsTimeoutErr(t *testing.T) {
	r := vestigo.NewRouter()
	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(500 * time.Millisecond)
	})

	s := httptest.NewServer(r)

	req, _ := http.NewRequest("GET", s.URL+"/", nil)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := http.DefaultClient.Do(req.WithContext(ctx))
	assert.True(t, isTimeoutErr(err))
}

func TestAnnotationsWriteTimeout(t *testing.T) {
	oldHash := randomdata.RandStringRunes(56)
	rw := new(RWMock)
	rw.On("Write", mock.AnythingOfType("*context.valueCtx"), "83a201c6-60cd-11e7-91a7-502f7ee26895", &expectedCanonicalisedAnnotationsBody, oldHash).Return("", &url.Error{Err: context.DeadlineExceeded})

	aug := new(AugmenterMock)
	annotationsAPI := new(AnnotationsAPIMock)

	h := New(rw, annotationsAPI, annotations.NewCanonicalizer(annotations.NewCanonicalAnnotationSorter), aug, time.Second)
	r := vestigo.NewRouter()
	r.Put("/drafts/content/:uuid/annotations", h.WriteAnnotations)

	entity := bytes.Buffer{}
	err := json.NewEncoder(&entity).Encode(&expectedAnnotations)
	if err != nil {
		t.Fatalf("failed to encode annotations: %v", err)
	}

	req := httptest.NewRequest(
		"PUT",
		"http://api.ft.com/drafts/content/83a201c6-60cd-11e7-91a7-502f7ee26895/annotations",
		&entity)

	req.Header.Set(tidutils.TransactionIDHeader, testTID)
	req.Header.Set(annotations.PreviousDocumentHashHeader, oldHash)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	resp := w.Result()
	assert.Equal(t, http.StatusGatewayTimeout, resp.StatusCode)

	body, err := ioutil.ReadAll(resp.Body)
	assert.NoError(t, err)
	assert.JSONEq(t, `{"message":"Timeout while waiting to write draft annotations"}`, string(body))

	rw.AssertExpectations(t)
	aug.AssertExpectations(t)
	annotationsAPI.AssertExpectations(t)
}

func TestHappyDeleteAnnotations(t *testing.T) {
	rw := new(RWMock)
	oldHash := randomdata.RandStringRunes(56)
	newHash := randomdata.RandStringRunes(56)
	rw.On("Write", mock.AnythingOfType("*context.valueCtx"), "83a201c6-60cd-11e7-91a7-502f7ee26895",
		&expectedCanonicalisedAnnotationsAfterDelete, oldHash).Return(newHash, nil)
	annAPI := new(AnnotationsAPIMock)
	annAPI.On("GetAllButV2", mock.Anything, "83a201c6-60cd-11e7-91a7-502f7ee26895").
		Return(expectedAnnotations.Annotations, nil)
	aug := new(AugmenterMock)

	h := New(rw, annAPI, annotations.NewCanonicalizer(annotations.NewCanonicalAnnotationSorter), aug, time.Second)
	r := vestigo.NewRouter()
	r.Delete("/drafts/content/:uuid/annotations/:cuuid", h.DeleteAnnotation)

	req := httptest.NewRequest(
		"DELETE",
		"http://api.ft.com/drafts/content/83a201c6-60cd-11e7-91a7-502f7ee26895/annotations/0a619d71-9af5-3755-90dd-f789b686c67a",
		nil)
	req.Header.Set(tidutils.TransactionIDHeader, testTID)
	req.Header.Set(annotations.PreviousDocumentHashHeader, oldHash)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)
	resp := w.Result()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	actual := annotations.Annotations{}
	err := json.NewDecoder(resp.Body).Decode(&actual)
	assert.NoError(t, err)

	assert.Equal(t, expectedCanonicalisedAnnotationsAfterDelete, actual)
	assert.Equal(t, newHash, resp.Header.Get(annotations.DocumentHashHeader))

	rw.AssertExpectations(t)
	aug.AssertExpectations(t)
	annAPI.AssertExpectations(t)
}

func TestUnHappyDeleteAnnotationsWhenMissingContentUUID(t *testing.T) {
	rw := new(RWMock)
	annAPI := new(AnnotationsAPIMock)
	aug := new(AugmenterMock)

	h := New(rw, annAPI, nil, aug, time.Second)
	r := vestigo.NewRouter()
	r.Delete("/drafts/content/:uuid/annotations/:cuuid", h.DeleteAnnotation)

	req := httptest.NewRequest(
		"DELETE",
		"http://api.ft.com/drafts/content/foo/annotations/eccb0da2-54f3-4f9f-bafa-fcec10e1758c",
		nil)
	req.Header.Set(tidutils.TransactionIDHeader, testTID)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)
	resp := w.Result()

	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
}

func TestUnHappyDeleteAnnotationsWhenMissingConceptUUID(t *testing.T) {
	rw := new(RWMock)
	annAPI := new(AnnotationsAPIMock)
	aug := new(AugmenterMock)

	h := New(rw, annAPI, nil, aug, time.Second)
	r := vestigo.NewRouter()
	r.Delete("/drafts/content/:uuid/annotations/:cuuid", h.DeleteAnnotation)

	req := httptest.NewRequest(
		"DELETE",
		"http://api.ft.com/drafts/content/83a201c6-60cd-11e7-91a7-502f7ee26895/annotations/bar",
		nil)
	req.Header.Set(tidutils.TransactionIDHeader, testTID)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)
	resp := w.Result()

	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
}

func TestUnHappyDeleteAnnotationsWhenRetrievingAnnotationsFails(t *testing.T) {
	rw := new(RWMock)
	annAPI := new(AnnotationsAPIMock)
	annAPI.On("GetAllButV2", mock.Anything, "83a201c6-60cd-11e7-91a7-502f7ee26895").
		Return([]annotations.Annotation{}, errors.New("sorry something failed"))
	aug := new(AugmenterMock)

	h := New(rw, annAPI, nil, aug, time.Second)
	r := vestigo.NewRouter()
	r.Delete("/drafts/content/:uuid/annotations/:cuuid", h.DeleteAnnotation)

	req := httptest.NewRequest(
		"DELETE",
		"http://api.ft.com/drafts/content/83a201c6-60cd-11e7-91a7-502f7ee26895/annotations/eccb0da2-54f3-4f9f-bafa-fcec10e1758c",
		nil)
	req.Header.Set(tidutils.TransactionIDHeader, testTID)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)
	resp := w.Result()

	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
}

func TestUnHappyDeleteAnnotationsWhenWritingAnnotationsFails(t *testing.T) {
	rw := new(RWMock)
	rw.On("Write", mock.AnythingOfType("*context.valueCtx"), "83a201c6-60cd-11e7-91a7-502f7ee26895", &expectedCanonicalisedAnnotationsBody, "").Return(mock.Anything, errors.New("sorry something failed"))
	annAPI := new(AnnotationsAPIMock)
	annAPI.On("GetAllButV2", mock.Anything, "83a201c6-60cd-11e7-91a7-502f7ee26895").
		Return(expectedAnnotations.Annotations, nil)
	aug := new(AugmenterMock)

	h := New(rw, annAPI, annotations.NewCanonicalizer(annotations.NewCanonicalAnnotationSorter), aug, time.Second)
	r := vestigo.NewRouter()
	r.Delete("/drafts/content/:uuid/annotations/:cuuid", h.DeleteAnnotation)

	req := httptest.NewRequest(
		"DELETE",
		"http://api.ft.com/drafts/content/83a201c6-60cd-11e7-91a7-502f7ee26895/annotations/eccb0da2-54f3-4f9f-bafa-fcec10e1758c",
		nil)
	req.Header.Set(tidutils.TransactionIDHeader, testTID)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)
	resp := w.Result()

	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
}

type AugmenterMock struct {
	mock.Mock
}

func (m *AugmenterMock) AugmentAnnotations(ctx context.Context, depletedAnnotations []annotations.Annotation) ([]annotations.Annotation, error) {
	args := m.Called(ctx, depletedAnnotations)
	return args.Get(0).([]annotations.Annotation), args.Error(1)
}

type RWMock struct {
	mock.Mock
}

func (m *RWMock) Read(ctx context.Context, contentUUID string) (*annotations.Annotations, string, bool, error) {
	args := m.Called(ctx, contentUUID)

	var ann *annotations.Annotations
	if v := args.Get(0); v != nil {
		ann = v.(*annotations.Annotations)
	}

	return ann, args.String(1), args.Bool(2), args.Error(3)
}

func (m *RWMock) Write(ctx context.Context, contentUUID string, a *annotations.Annotations, hash string) (string, error) {
	args := m.Called(ctx, contentUUID, a, hash)
	return args.String(0), args.Error(1)
}

func (m *RWMock) Endpoint() string {
	args := m.Called()
	return args.String(0)
}

func (m *RWMock) GTG() error {
	args := m.Called()
	return args.Error(0)
}

type AnnotationsAPIMock struct {
	mock.Mock
}

func (m *AnnotationsAPIMock) GetAll(ctx context.Context, contentUUID string) ([]annotations.Annotation, error) {
	args := m.Called(ctx, contentUUID)
	return args.Get(0).([]annotations.Annotation), args.Error(1)
}

func (m *AnnotationsAPIMock) GetAllButV2(ctx context.Context, contentUUID string) ([]annotations.Annotation, error) {
	args := m.Called(ctx, contentUUID)
	return args.Get(0).([]annotations.Annotation), args.Error(1)
}

func (m *AnnotationsAPIMock) Endpoint() string {
	args := m.Called()
	return args.String(0)
}

func (m *AnnotationsAPIMock) GTG() error {
	args := m.Called()
	return args.Error(0)
}
