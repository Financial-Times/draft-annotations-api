package handler

import (
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"context"
	"encoding/json"
	"errors"
	"github.com/Financial-Times/draft-annotations-api/annotations"
	tidutils "github.com/Financial-Times/transactionid-utils-go"
	"github.com/husobee/vestigo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

const testAPIKey = "testAPIKey"
const testTID = "test_tid"

const apiKeyHeader = "X-Api-Key"

func TestHappyFetchFromAnnotationsRW(t *testing.T) {
	var expectedAnnotations []*annotations.Annotation
	json.Unmarshal([]byte(annotationsBody), &expectedAnnotations)

	rw := new(RWMock)
	rw.On("Read", mock.Anything, "83a201c6-60cd-11e7-91a7-502f7ee26895").Return(expectedAnnotations, true, nil)
	aug := new(AugmenterMock)
	aug.On("AugmentAnnotations", mock.Anything, &expectedAnnotations).Return(nil)
	annAPI := new(AnnotationsAPIMock)

	h := New(rw, annAPI, nil, aug)
	r := vestigo.NewRouter()
	r.Get("/drafts/content/:uuid/annotations", h.ReadAnnotations)

	req := httptest.NewRequest("GET", "http://api.ft.com/drafts/content/83a201c6-60cd-11e7-91a7-502f7ee26895/annotations", nil)
	req.Header.Set(tidutils.TransactionIDHeader, testTID)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)
	resp := w.Result()
	body, err := ioutil.ReadAll(resp.Body)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.NoError(t, err)
	assert.JSONEq(t, string(annotationsBody), string(body))

	rw.AssertExpectations(t)
	aug.AssertExpectations(t)
	annAPI.AssertExpectations(t)
}

func TestUnHappyFetchFromAnnotationsRW(t *testing.T) {
	rw := new(RWMock)
	rw.On("Read", mock.Anything, "83a201c6-60cd-11e7-91a7-502f7ee26895").Return([]*annotations.Annotation{}, false, errors.New("computer says no"))
	aug := new(AugmenterMock)
	annAPI := new(AnnotationsAPIMock)

	h := New(rw, annAPI, nil, aug)
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
	assert.Equal(t, `{"message":"Annotations RW error: computer says no"}`, string(body))

	rw.AssertExpectations(t)
	aug.AssertExpectations(t)
	annAPI.AssertExpectations(t)
}

func TestUnHappyAugmenter(t *testing.T) {
	var expectedAnnotations []*annotations.Annotation
	json.Unmarshal([]byte(annotationsBody), &expectedAnnotations)

	rw := new(RWMock)
	rw.On("Read", mock.Anything, "83a201c6-60cd-11e7-91a7-502f7ee26895").Return(expectedAnnotations, true, nil)
	aug := new(AugmenterMock)
	aug.On("AugmentAnnotations", mock.Anything, &expectedAnnotations).Return(errors.New("computer says no"))
	annAPI := new(AnnotationsAPIMock)

	h := New(rw, annAPI, nil, aug)
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
	assert.Equal(t, `{"message":"Annotations augmenter error: computer says no"}`, string(body))

	rw.AssertExpectations(t)
	aug.AssertExpectations(t)
	annAPI.AssertExpectations(t)
}

func TestFetchFromAnnotationsAPIIfNotFoundInRW(t *testing.T) {
	aug := new(AugmenterMock)
	rw := new(RWMock)

	rw.On("Read", mock.Anything, "83a201c6-60cd-11e7-91a7-502f7ee26895").Return([]*annotations.Annotation{}, false, nil)

	annotationsAPIServerMock := newAnnotationsAPIServerMock(t, http.StatusOK, annotationsBody)
	defer annotationsAPIServerMock.Close()

	annotationsAPI := annotations.NewAnnotationsAPI(annotationsAPIServerMock.URL+"/content/%v/annotations", testAPIKey)
	assert.Equal(t, annotationsAPIServerMock.URL+"/content/%v/annotations", annotationsAPI.Endpoint())

	h := New(rw, annotationsAPI, nil, aug)
	r := vestigo.NewRouter()
	r.Get("/drafts/content/:uuid/annotations", h.ReadAnnotations)

	req := httptest.NewRequest("GET", "http://api.ft.com/drafts/content/83a201c6-60cd-11e7-91a7-502f7ee26895/annotations", nil)
	req.Header.Set(tidutils.TransactionIDHeader, testTID)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)
	resp := w.Result()
	body, err := ioutil.ReadAll(resp.Body)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.NoError(t, err)
	assert.JSONEq(t, string(annotationsBody), string(body))

	rw.AssertExpectations(t)
	aug.AssertExpectations(t)
}

func TestFetchFromAnnotationsAPI404(t *testing.T) {
	aug := new(AugmenterMock)
	rw := new(RWMock)
	rw.On("Read", mock.Anything, "83a201c6-60cd-11e7-91a7-502f7ee26895").Return([]*annotations.Annotation{}, false, nil)

	annotationsAPIServerMock := newAnnotationsAPIServerMock(t, http.StatusNotFound, "not found")
	defer annotationsAPIServerMock.Close()

	annotationsAPI := annotations.NewAnnotationsAPI(annotationsAPIServerMock.URL+"/content/%v/annotations", testAPIKey)
	h := New(rw, annotationsAPI, nil, aug)
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
	rw.On("Read", mock.Anything, "83a201c6-60cd-11e7-91a7-502f7ee26895").Return([]*annotations.Annotation{}, false, nil)
	aug := new(AugmenterMock)

	annotationsAPIServerMock := newAnnotationsAPIServerMock(t, http.StatusOK, bannedAnnotationsBody)
	defer annotationsAPIServerMock.Close()

	annotationsAPI := annotations.NewAnnotationsAPI(annotationsAPIServerMock.URL+"/content/%v/annotations", testAPIKey)
	h := New(rw, annotationsAPI, nil, aug)
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
	assert.Equal(t, "{\"message\":\"No annotations can be found\"}", string(body))

	rw.AssertExpectations(t)
	aug.AssertExpectations(t)
}

func TestFetchFromAnnotationsAPI500(t *testing.T) {
	rw := new(RWMock)
	rw.On("Read", mock.Anything, "83a201c6-60cd-11e7-91a7-502f7ee26895").Return([]*annotations.Annotation{}, false, nil)
	aug := new(AugmenterMock)
	annotationsAPIServerMock := newAnnotationsAPIServerMock(t, http.StatusInternalServerError, "fire!")
	defer annotationsAPIServerMock.Close()

	annotationsAPI := annotations.NewAnnotationsAPI(annotationsAPIServerMock.URL+"/content/%v/annotations", testAPIKey)
	h := New(rw, annotationsAPI, nil, aug)
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
	rw.On("Read", mock.Anything, "83a201c6-60cd-11e7-91a7-502f7ee26895").Return([]*annotations.Annotation{}, false, nil)
	aug := new(AugmenterMock)
	annotationsAPI := annotations.NewAnnotationsAPI(":#", testAPIKey)
	h := New(rw, annotationsAPI, nil, aug)
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
	assert.Equal(t, "parse :: missing protocol scheme\n", string(body))

	rw.AssertExpectations(t)
	aug.AssertExpectations(t)
}

func TestFetchFromAnnotationsAPIWithConnectionError(t *testing.T) {
	rw := new(RWMock)
	rw.On("Read", mock.Anything, "83a201c6-60cd-11e7-91a7-502f7ee26895").Return([]*annotations.Annotation{}, false, nil)
	aug := new(AugmenterMock)
	annotationsAPIServerMock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	annotationsAPIServerMock.Close()

	annotationsAPI := annotations.NewAnnotationsAPI(annotationsAPIServerMock.URL, testAPIKey)
	h := New(rw, annotationsAPI, nil, aug)
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

func TestSaveAnnotations(t *testing.T) {
	rw := new(RWMock)
	aug := new(AugmenterMock)
	h := New(rw, nil, annotations.NewCanonicalizer(annotations.NewCanonicalAnnotationSorter), aug)
	r := vestigo.NewRouter()
	r.Put("/drafts/content/:uuid/annotations", h.WriteAnnotations)

	req := httptest.NewRequest(
		"PUT",
		"http://api.ft.com/drafts/content/83a201c6-60cd-11e7-91a7-502f7ee26895/annotations",
		strings.NewReader(annotationsBody))

	req.Header.Set(tidutils.TransactionIDHeader, testTID)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)
	resp := w.Result()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	rw.AssertExpectations(t)
	aug.AssertExpectations(t)
}

type AugmenterMock struct {
	mock.Mock
}

func (m *AugmenterMock) AugmentAnnotations(ctx context.Context, annotations *[]*annotations.Annotation) error {
	args := m.Called(ctx, annotations)
	return args.Error(0)
}

type RWMock struct {
	mock.Mock
}

func (m *RWMock) Read(ctx context.Context, contentUUID string) ([]*annotations.Annotation, bool, error) {
	args := m.Called(ctx, contentUUID)
	return args.Get(0).([]*annotations.Annotation), args.Bool(1), args.Error(2)
}

type AnnotationsAPIMock struct {
	mock.Mock
}

func (m *AnnotationsAPIMock) Get(ctx context.Context, contentUUID string) (*http.Response, error) {
	args := m.Called(ctx, contentUUID)
	return args.Get(0).(*http.Response), args.Error(1)
}

func (m *AnnotationsAPIMock) Endpoint() string {
	args := m.Called()
	return args.String(0)
}

func (m *AnnotationsAPIMock) GTG() error {
	args := m.Called()
	return args.Error(0)
}
