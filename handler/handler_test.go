package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Financial-Times/draft-annotations-api/annotations"
	"github.com/Financial-Times/draft-annotations-api/basicauth"
	"github.com/Financial-Times/draft-annotations-api/handler"
	"github.com/Financial-Times/go-ft-http/fthttp"
	tidutils "github.com/Financial-Times/transactionid-utils-go"
	randomdata "github.com/Pallinder/go-randomdata"
	"github.com/husobee/vestigo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

const testTID = "test_tid"

var testClient = fthttp.NewClientWithDefaultTimeout("PAC", "draft-annotations-api")

func TestHappyFetchFromAnnotationsRW(t *testing.T) {
	hash := randomdata.RandStringRunes(56)

	rw := new(RWMock)
	rw.On("Read", mock.Anything, "83a201c6-60cd-11e7-91a7-502f7ee26895").Return(&expectedAnnotations, hash, true, nil)
	aug := new(AugmenterMock)
	aug.On("AugmentAnnotations", mock.Anything, expectedAnnotations.Annotations).Return(expectedAnnotations.Annotations, nil)
	annAPI := new(AnnotationsAPIMock)

	h := handler.New(rw, annAPI, nil, aug, time.Second)
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

func TestReadHasBrandAnnotation(t *testing.T) {

	tests := map[string]struct {
		readAnnotations     []annotations.Annotation
		expectedAnnotations []annotations.Annotation
		sendHasBrand        bool
	}{
		"show hasBrand annotations": {
			readAnnotations: []annotations.Annotation{
				{
					Predicate: "http://www.ft.com/ontology/hasBrand",
					ConceptId: "http://www.ft.com/thing/87645070-7d8a-492e-9695-bf61ac2b4d18",
					Type:      "http://www.ft.com/ontology/product/Brand",
				},
			},
			expectedAnnotations: []annotations.Annotation{
				{
					Predicate: "http://www.ft.com/ontology/hasBrand",
					ConceptId: "http://www.ft.com/thing/87645070-7d8a-492e-9695-bf61ac2b4d18",
					Type:      "http://www.ft.com/ontology/product/Brand",
				},
			},
			sendHasBrand: true,
		},
		"hide hasBrand annotations": {
			readAnnotations: []annotations.Annotation{
				{
					Predicate: "http://www.ft.com/ontology/hasBrand",
					ConceptId: "http://www.ft.com/thing/87645070-7d8a-492e-9695-bf61ac2b4d18",
					Type:      "http://www.ft.com/ontology/product/Brand",
				},
			},
			expectedAnnotations: []annotations.Annotation{
				{
					Predicate: "http://www.ft.com/ontology/classification/isClassifiedBy",
					ConceptId: "http://www.ft.com/thing/87645070-7d8a-492e-9695-bf61ac2b4d18",
					Type:      "http://www.ft.com/ontology/product/Brand",
				},
			},
			sendHasBrand: false,
		},
	}

	rw := &RWMock{}
	aug := &AugmenterMock{}
	annAPI := &AnnotationsAPIMock{}
	h := handler.New(rw, annAPI, nil, aug, time.Second)
	r := vestigo.NewRouter()
	r.Get("/drafts/content/:uuid/annotations", h.ReadAnnotations)

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			hash := randomdata.RandStringRunes(56)
			rw.read = func(ctx context.Context, contentUUID string) (*annotations.Annotations, string, bool, error) {
				return &annotations.Annotations{Annotations: test.readAnnotations}, hash, true, nil
			}
			aug.augment = func(ctx context.Context, depletedAnnotations []annotations.Annotation) ([]annotations.Annotation, error) {
				return test.readAnnotations, nil
			}

			req := httptest.NewRequest("GET", "/drafts/content/83a201c6-60cd-11e7-91a7-502f7ee26895/annotations", nil)
			q := req.URL.Query()
			q.Add("sendHasBrand", strconv.FormatBool(test.sendHasBrand))
			req.URL.RawQuery = q.Encode()
			req.Header.Set(tidutils.TransactionIDHeader, testTID)
			w := httptest.NewRecorder()

			r.ServeHTTP(w, req)
			resp := w.Result()
			defer resp.Body.Close()
			assert.Equal(t, http.StatusOK, resp.StatusCode)

			actual := annotations.Annotations{}
			err := json.NewDecoder(resp.Body).Decode(&actual)
			assert.NoError(t, err)

			assert.Equal(t, annotations.Annotations{Annotations: test.expectedAnnotations}, actual)
			assert.Equal(t, hash, resp.Header.Get(annotations.DocumentHashHeader))

		})
	}
}

func TestAddAnnotation(t *testing.T) {
	rw := &RWMock{}
	annAPI := &AnnotationsAPIMock{}
	aug := &AugmenterMock{}

	handler := handler.New(rw, annAPI, annotations.NewCanonicalizer(annotations.NewCanonicalAnnotationSorter), aug, time.Second)
	router := vestigo.NewRouter()
	router.Post("/drafts/content/:uuid/annotations", handler.AddAnnotation)

	oldHash := randomdata.RandStringRunes(56)
	newHash := randomdata.RandStringRunes(56)

	tests := map[string]struct {
		saved             []annotations.Annotation
		augmented         []annotations.Annotation
		added             annotations.Annotation
		requestStatusCode int
	}{
		"success - accept hasBrand annotation": {
			augmented: []annotations.Annotation{
				{
					Predicate: "http://www.ft.com/ontology/hasBrand",
					ConceptId: "http://www.ft.com/thing/100e3cc0-aecc-4458-8ebd-6b1fbc7345ed",
					ApiUrl:    "http://api.ft.com/concepts/100e3cc0-aecc-4458-8ebd-6b1fbc7345ed",
					Type:      "http://www.ft.com/ontology/product/Brand",
					PrefLabel: "Temp brand",
				},
			},
			saved: []annotations.Annotation{
				{
					Predicate: "http://www.ft.com/ontology/hasBrand",
					ConceptId: "http://www.ft.com/thing/100e3cc0-aecc-4458-8ebd-6b1fbc7345ed",
				},
			},
			added: annotations.Annotation{
				Predicate: "http://www.ft.com/ontology/hasBrand",
				ConceptId: "http://www.ft.com/thing/100e3cc0-aecc-4458-8ebd-6b1fbc7345ed",
			},
			requestStatusCode: http.StatusOK,
		},
		"success - switch isClassifiedBy to hasBrand annotation": {
			augmented: []annotations.Annotation{
				{
					Predicate: "http://www.ft.com/ontology/hasBrand",
					ConceptId: "http://www.ft.com/thing/100e3cc0-aecc-4458-8ebd-6b1fbc7345ed",
					ApiUrl:    "http://api.ft.com/concepts/100e3cc0-aecc-4458-8ebd-6b1fbc7345ed",
					Type:      "http://www.ft.com/ontology/product/Brand",
					PrefLabel: "Temp brand",
				},
			},
			saved: []annotations.Annotation{
				{
					Predicate: "http://www.ft.com/ontology/hasBrand",
					ConceptId: "http://www.ft.com/thing/100e3cc0-aecc-4458-8ebd-6b1fbc7345ed",
				},
			},
			added: annotations.Annotation{
				Predicate: "http://www.ft.com/ontology/classification/isClassifiedBy",
				ConceptId: "http://www.ft.com/thing/100e3cc0-aecc-4458-8ebd-6b1fbc7345ed",
			},
			requestStatusCode: http.StatusOK,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			calledGetAll := false
			rw.write = func(ctx context.Context, contentUUID string, a *annotations.Annotations, hash string) (string, error) {
				assert.Equal(t, &annotations.Annotations{Annotations: test.saved}, a)
				assert.Equal(t, oldHash, hash)
				return newHash, nil
			}
			annAPI.getAllButV2 = func(ctx context.Context, contentUUID string) ([]annotations.Annotation, error) {
				calledGetAll = true
				return []annotations.Annotation{}, nil
			}
			aug.augment = func(ctx context.Context, depletedAnnotations []annotations.Annotation) ([]annotations.Annotation, error) {
				expect := []annotations.Annotation{test.added}
				assert.Equal(t, expect, depletedAnnotations)
				return test.augmented, nil
			}

			b, _ := json.Marshal(test.added)

			req := httptest.NewRequest(
				"POST",
				"http://api.ft.com/drafts/content/83a201c6-60cd-11e7-91a7-502f7ee26895/annotations",
				bytes.NewBuffer(b))

			req.Header.Set(tidutils.TransactionIDHeader, testTID)
			req.Header.Set(annotations.PreviousDocumentHashHeader, oldHash)
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			resp := w.Result()
			defer resp.Body.Close()
			assert.Equal(t, test.requestStatusCode, resp.StatusCode)
			if test.requestStatusCode != http.StatusOK {
				return
			}
			assert.Equal(t, newHash, resp.Header.Get(annotations.DocumentHashHeader))
			assert.True(t, calledGetAll, "expect to request latest annotations from UPP")

		})
	}
}

func TestWriteHasBrandAnnotation(t *testing.T) {
	rw := &RWMock{}
	annAPI := &AnnotationsAPIMock{}
	aug := &AugmenterMock{}

	handler := handler.New(rw, annAPI, annotations.NewCanonicalizer(annotations.NewCanonicalAnnotationSorter), aug, time.Second)
	router := vestigo.NewRouter()
	router.Put("/drafts/content/:uuid/annotations", handler.WriteAnnotations)

	oldHash := randomdata.RandStringRunes(56)
	newHash := randomdata.RandStringRunes(56)

	tests := map[string]struct {
		written           []annotations.Annotation
		augmented         []annotations.Annotation
		saved             []annotations.Annotation
		requestStatusCode int
	}{
		"success - accept hasBrand annotation": {
			written: []annotations.Annotation{
				{
					Predicate: "http://www.ft.com/ontology/hasBrand",
					ConceptId: "http://www.ft.com/thing/100e3cc0-aecc-4458-8ebd-6b1fbc7345ed",
				},
			},
			augmented: []annotations.Annotation{
				{
					Predicate: "http://www.ft.com/ontology/hasBrand",
					ConceptId: "http://www.ft.com/thing/100e3cc0-aecc-4458-8ebd-6b1fbc7345ed",
					ApiUrl:    "http://api.ft.com/concepts/100e3cc0-aecc-4458-8ebd-6b1fbc7345ed",
					Type:      "http://www.ft.com/ontology/product/Brand",
					PrefLabel: "Temp brand",
				},
			},
			saved: []annotations.Annotation{
				{
					Predicate: "http://www.ft.com/ontology/hasBrand",
					ConceptId: "http://www.ft.com/thing/100e3cc0-aecc-4458-8ebd-6b1fbc7345ed",
				},
			},
			requestStatusCode: http.StatusOK,
		},
		"success - switch isClassifiedBy to hasBrand annotation": {
			written: []annotations.Annotation{
				{
					Predicate: "http://www.ft.com//classification/isClassifiedBy",
					ConceptId: "http://www.ft.com/thing/100e3cc0-aecc-4458-8ebd-6b1fbc7345ed",
				},
			},
			augmented: []annotations.Annotation{
				{
					Predicate: "http://www.ft.com/ontology/hasBrand",
					ConceptId: "http://www.ft.com/thing/100e3cc0-aecc-4458-8ebd-6b1fbc7345ed",
					ApiUrl:    "http://api.ft.com/concepts/100e3cc0-aecc-4458-8ebd-6b1fbc7345ed",
					Type:      "http://www.ft.com/ontology/product/Brand",
					PrefLabel: "Temp brand",
				},
			},
			saved: []annotations.Annotation{
				{
					Predicate: "http://www.ft.com/ontology/hasBrand",
					ConceptId: "http://www.ft.com/thing/100e3cc0-aecc-4458-8ebd-6b1fbc7345ed",
				},
			},
			requestStatusCode: http.StatusOK,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			rw.write = func(ctx context.Context, contentUUID string, a *annotations.Annotations, hash string) (string, error) {
				assert.Equal(t, &annotations.Annotations{Annotations: test.saved}, a)
				assert.Equal(t, oldHash, hash)
				return newHash, nil
			}
			aug.augment = func(ctx context.Context, depletedAnnotations []annotations.Annotation) ([]annotations.Annotation, error) {
				assert.Equal(t, test.written, depletedAnnotations)
				return test.augmented, nil
			}

			b, _ := json.Marshal(annotations.Annotations{Annotations: test.written})

			req := httptest.NewRequest(
				"PUT",
				"http://api.ft.com/drafts/content/83a201c6-60cd-11e7-91a7-502f7ee26895/annotations",
				bytes.NewBuffer(b))

			req.Header.Set(tidutils.TransactionIDHeader, testTID)
			req.Header.Set(annotations.PreviousDocumentHashHeader, oldHash)
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			resp := w.Result()
			defer resp.Body.Close()
			assert.Equal(t, test.requestStatusCode, resp.StatusCode)

			if test.requestStatusCode != http.StatusOK {
				return
			}
			assert.Equal(t, newHash, resp.Header.Get(annotations.DocumentHashHeader))
		})
	}
}

func TestReplaceHasBrandAnnotation(t *testing.T) {
	rw := &RWMock{}
	annAPI := &AnnotationsAPIMock{}
	aug := &AugmenterMock{}
	canonicalizer := annotations.NewCanonicalizer(annotations.NewCanonicalAnnotationSorter)

	handler := handler.New(rw, annAPI, canonicalizer, aug, time.Second)
	router := vestigo.NewRouter()
	router.Patch("/drafts/content/:uuid/annotations/:cuuid", handler.ReplaceAnnotation)

	oldHash := randomdata.RandStringRunes(56)
	newHash := randomdata.RandStringRunes(56)

	tests := map[string]struct {
		fromUpp           []annotations.Annotation
		toReplace         string
		replaceWith       annotations.Annotation
		afterReplace      []annotations.Annotation
		augmented         []annotations.Annotation
		toStore           []annotations.Annotation
		requestStatusCode int
	}{
		"success - accept hasBrand annotation": {
			fromUpp: []annotations.Annotation{
				{
					Predicate: "http://www.ft.com/ontology/annotation/mentions",
					ConceptId: "http://www.ft.com/thing/0a619d71-9af5-3755-90dd-f789b686c67a",
					ApiUrl:    "http://api.ft.com/people/0a619d71-9af5-3755-90dd-f789b686c67a",
					Type:      "http://www.ft.com/ontology/person/Person",
					PrefLabel: "Barack H. Obama",
				},
				{
					Predicate: "http://www.ft.com/ontology/annotation/about",
					ConceptId: "http://www.ft.com/thing/9577c6d4-b09e-4552-b88f-e52745abe02b",
					ApiUrl:    "http://api.ft.com/concepts/9577c6d4-b09e-4552-b88f-e52745abe02b",
					Type:      "http://www.ft.com/ontology/Topic",
					PrefLabel: "US interest rates",
				},
			},
			toReplace: "9577c6d4-b09e-4552-b88f-e52745abe02b",
			replaceWith: annotations.Annotation{
				Predicate: "http://www.ft.com/ontology/hasBrand",
				ConceptId: "http://www.ft.com/thing/100e3cc0-aecc-4458-8ebd-6b1fbc7345ed",
			},
			afterReplace: []annotations.Annotation{
				{
					Predicate: "http://www.ft.com/ontology/annotation/mentions",
					ConceptId: "http://www.ft.com/thing/0a619d71-9af5-3755-90dd-f789b686c67a",
				},
				{
					Predicate: "http://www.ft.com/ontology/hasBrand",
					ConceptId: "http://www.ft.com/thing/100e3cc0-aecc-4458-8ebd-6b1fbc7345ed",
				},
			},
			augmented: []annotations.Annotation{
				{
					Predicate: "http://www.ft.com/ontology/annotation/mentions",
					ConceptId: "http://www.ft.com/thing/0a619d71-9af5-3755-90dd-f789b686c67a",
					ApiUrl:    "http://api.ft.com/people/0a619d71-9af5-3755-90dd-f789b686c67a",
					Type:      "http://www.ft.com/ontology/person/Person",
					PrefLabel: "Barack H. Obama",
				},
				{
					Predicate: "http://www.ft.com/ontology/hasBrand",
					ConceptId: "http://www.ft.com/thing/100e3cc0-aecc-4458-8ebd-6b1fbc7345ed",
					ApiUrl:    "http://api.ft.com/concepts/100e3cc0-aecc-4458-8ebd-6b1fbc7345ed",
					Type:      "http://www.ft.com/ontology/product/Brand",
					PrefLabel: "Random brand",
				},
			},
			toStore: []annotations.Annotation{
				{
					Predicate: "http://www.ft.com/ontology/annotation/mentions",
					ConceptId: "http://www.ft.com/thing/0a619d71-9af5-3755-90dd-f789b686c67a",
				},
				{
					Predicate: "http://www.ft.com/ontology/hasBrand",
					ConceptId: "http://www.ft.com/thing/100e3cc0-aecc-4458-8ebd-6b1fbc7345ed",
				},
			},
			requestStatusCode: http.StatusOK,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			rw.write = func(ctx context.Context, contentUUID string, a *annotations.Annotations, hash string) (string, error) {
				assert.Equal(t, &annotations.Annotations{Annotations: test.toStore}, a)
				assert.Equal(t, oldHash, hash)
				return newHash, nil
			}
			getAllCalled := false
			annAPI.getAllButV2 = func(ctx context.Context, contentUUID string) ([]annotations.Annotation, error) {
				getAllCalled = true
				return test.fromUpp, nil
			}
			aug.augment = func(ctx context.Context, depletedAnnotations []annotations.Annotation) ([]annotations.Annotation, error) {
				depletedAnnotations = canonicalizer.Canonicalize(depletedAnnotations)
				assert.Equal(t, test.afterReplace, depletedAnnotations)
				return test.augmented, nil
			}

			b, _ := json.Marshal(test.replaceWith)
			url := "http://api.ft.com/drafts/content/83a201c6-60cd-11e7-91a7-502f7ee26895/annotations/" + test.toReplace
			req := httptest.NewRequest(
				"PATCH",
				url,
				bytes.NewBuffer(b))

			req.Header.Set(tidutils.TransactionIDHeader, testTID)
			req.Header.Set(annotations.PreviousDocumentHashHeader, oldHash)
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			resp := w.Result()
			defer resp.Body.Close()
			assert.Equal(t, test.requestStatusCode, resp.StatusCode)

			if test.requestStatusCode != http.StatusOK {
				return
			}
			assert.Equal(t, newHash, resp.Header.Get(annotations.DocumentHashHeader))
			assert.True(t, getAllCalled, "expect to get annotations from UPP")
		})
	}
}

func TestUnHappyFetchFromAnnotationsRW(t *testing.T) {
	rw := new(RWMock)
	rw.On("Read", mock.Anything, "83a201c6-60cd-11e7-91a7-502f7ee26895").Return(nil, "", false, errors.New("computer says no"))
	aug := new(AugmenterMock)
	annAPI := new(AnnotationsAPIMock)

	h := handler.New(rw, annAPI, nil, aug, time.Second)
	r := vestigo.NewRouter()
	r.Get("/drafts/content/:uuid/annotations", h.ReadAnnotations)

	req := httptest.NewRequest("GET", "http://api.ft.com/drafts/content/83a201c6-60cd-11e7-91a7-502f7ee26895/annotations", nil)
	req.Header.Set(tidutils.TransactionIDHeader, testTID)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)
	resp := w.Result()
	body, err := io.ReadAll(resp.Body)

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

	h := handler.New(rw, annAPI, nil, aug, time.Second)
	r := vestigo.NewRouter()
	r.Get("/drafts/content/:uuid/annotations", h.ReadAnnotations)

	req := httptest.NewRequest("GET", "http://api.ft.com/drafts/content/83a201c6-60cd-11e7-91a7-502f7ee26895/annotations", nil)
	req.Header.Set(tidutils.TransactionIDHeader, testTID)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)
	resp := w.Result()
	body, err := io.ReadAll(resp.Body)

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

	annotationsAPI := annotations.NewUPPAnnotationsAPI(testClient, annotationsAPIServerMock.URL+"/content/%v/annotations", basicauth.TestBasicAuthUsername, basicauth.TestBasicAuthPassword)
	assert.Equal(t, annotationsAPIServerMock.URL+"/content/%v/annotations", annotationsAPI.Endpoint())

	h := handler.New(rw, annotationsAPI, nil, aug, time.Second)
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

	annotationsAPI := annotations.NewUPPAnnotationsAPI(testClient, annotationsAPIServerMock.URL+"/content/%v/annotations", basicauth.TestBasicAuthUsername, basicauth.TestBasicAuthPassword)
	h := handler.New(rw, annotationsAPI, nil, aug, time.Second)
	r := vestigo.NewRouter()
	r.Get("/drafts/content/:uuid/annotations", h.ReadAnnotations)

	req := httptest.NewRequest("GET", "http://api.ft.com/drafts/content/83a201c6-60cd-11e7-91a7-502f7ee26895/annotations", nil)
	req.Header.Set(tidutils.TransactionIDHeader, testTID)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)
	resp := w.Result()
	body, err := io.ReadAll(resp.Body)

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

	annotationsAPI := annotations.NewUPPAnnotationsAPI(testClient, annotationsAPIServerMock.URL+"/content/%v/annotations", basicauth.TestBasicAuthUsername, basicauth.TestBasicAuthPassword)
	h := handler.New(rw, annotationsAPI, nil, aug, time.Second)
	r := vestigo.NewRouter()
	r.Get("/drafts/content/:uuid/annotations", h.ReadAnnotations)

	req := httptest.NewRequest("GET", "http://api.ft.com/drafts/content/83a201c6-60cd-11e7-91a7-502f7ee26895/annotations", nil)
	req.Header.Set(tidutils.TransactionIDHeader, testTID)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)
	resp := w.Result()
	body, err := io.ReadAll(resp.Body)

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

	annotationsAPI := annotations.NewUPPAnnotationsAPI(testClient, annotationsAPIServerMock.URL+"/content/%v/annotations", basicauth.TestBasicAuthUsername, basicauth.TestBasicAuthPassword)
	h := handler.New(rw, annotationsAPI, nil, aug, time.Second)
	r := vestigo.NewRouter()
	r.Get("/drafts/content/:uuid/annotations", h.ReadAnnotations)

	req := httptest.NewRequest("GET", "http://api.ft.com/drafts/content/83a201c6-60cd-11e7-91a7-502f7ee26895/annotations", nil)
	req.Header.Set(tidutils.TransactionIDHeader, testTID)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)
	resp := w.Result()
	body, err := io.ReadAll(resp.Body)

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
	annotationsAPI := annotations.NewUPPAnnotationsAPI(testClient, ":#", basicauth.TestBasicAuthUsername, basicauth.TestBasicAuthPassword)
	h := handler.New(rw, annotationsAPI, nil, aug, time.Second)
	r := vestigo.NewRouter()
	r.Get("/drafts/content/:uuid/annotations", h.ReadAnnotations)

	req := httptest.NewRequest("GET", "http://api.ft.com/drafts/content/83a201c6-60cd-11e7-91a7-502f7ee26895/annotations", nil)
	req.Header.Set(tidutils.TransactionIDHeader, testTID)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)
	resp := w.Result()
	body, err := io.ReadAll(resp.Body)

	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
	assert.NoError(t, err)
	assert.Contains(t, string(body), "Failed to read annotations")
	rw.AssertExpectations(t)
	aug.AssertExpectations(t)
}

func TestFetchFromAnnotationsAPIWithConnectionError(t *testing.T) {
	rw := new(RWMock)
	rw.On("Read", mock.Anything, "83a201c6-60cd-11e7-91a7-502f7ee26895").Return(nil, "", false, nil)
	aug := new(AugmenterMock)
	annotationsAPIServerMock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	annotationsAPIServerMock.Close()

	annotationsAPI := annotations.NewUPPAnnotationsAPI(testClient, annotationsAPIServerMock.URL, basicauth.TestBasicAuthUsername, basicauth.TestBasicAuthPassword)
	h := handler.New(rw, annotationsAPI, nil, aug, time.Second)
	r := vestigo.NewRouter()
	r.Get("/drafts/content/:uuid/annotations", h.ReadAnnotations)

	req := httptest.NewRequest("GET", "http://api.ft.com/drafts/content/83a201c6-60cd-11e7-91a7-502f7ee26895/annotations", nil)
	req.Header.Set(tidutils.TransactionIDHeader, testTID)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)
	resp := w.Result()
	_, err := io.ReadAll(resp.Body)

	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
	assert.NoError(t, err)

	rw.AssertExpectations(t)
	aug.AssertExpectations(t)
}

func newAnnotationsAPIServerMock(t *testing.T, status int, body string) *httptest.Server {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if basicAuth := r.Header.Get(basicauth.AuthorizationHeader); basicAuth != basicauth.EncodeBasicAuthForTests(t) {
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
      "predicate": "http://www.ft.com/ontology/annotation/about",
      "id": "http://api.ft.com/thing/9577c6d4-b09e-4552-b88f-e52745abe02b",
      "apiUrl": "http://api.ft.com/concepts/9577c6d4-b09e-4552-b88f-e52745abe02b",
      "types": [
         "http://www.ft.com/ontology/core/Thing",
         "http://www.ft.com/ontology/concept/Concept",
         "http://www.ft.com/ontology/Topic"
      ],
      "prefLabel": "US interest rates"
   },
   {
      "predicate": "http://www.ft.com/ontology/hasDisplayTag",
      "id": "http://api.ft.com/thing/9577c6d4-b09e-4552-b88f-e52745abe02b",
      "apiUrl": "http://api.ft.com/concepts/9577c6d4-b09e-4552-b88f-e52745abe02b",
      "types": [
         "http://www.ft.com/ontology/core/Thing",
         "http://www.ft.com/ontology/concept/Concept",
         "http://www.ft.com/ontology/Topic"
      ],
      "prefLabel": "US interest rates"
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
      "predicate": "http://www.ft.com/ontology/annotation/about",
      "id": "http://api.ft.com/thing/9577c6d4-b09e-4552-b88f-e52745abe02b",
      "apiUrl": "http://api.ft.com/concepts/9577c6d4-b09e-4552-b88f-e52745abe02b",
      "type": "http://www.ft.com/ontology/Topic",
      "prefLabel": "US interest rates"
   },
   {
      "predicate": "http://www.ft.com/ontology/hasDisplayTag",
      "id": "http://api.ft.com/thing/9577c6d4-b09e-4552-b88f-e52745abe02b",
      "apiUrl": "http://api.ft.com/concepts/9577c6d4-b09e-4552-b88f-e52745abe02b",
      "type": "http://www.ft.com/ontology/Topic",
      "prefLabel": "US interest rates"
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
			Predicate: "http://www.ft.com/ontology/annotation/about",
			ConceptId: "http://www.ft.com/thing/9577c6d4-b09e-4552-b88f-e52745abe02b",
			ApiUrl:    "http://api.ft.com/concepts/9577c6d4-b09e-4552-b88f-e52745abe02b",
			Type:      "http://www.ft.com/ontology/Topic",
			PrefLabel: "US interest rates",
		},
		{
			Predicate: "http://www.ft.com/ontology/hasDisplayTag",
			ConceptId: "http://www.ft.com/thing/9577c6d4-b09e-4552-b88f-e52745abe02b",
			ApiUrl:    "http://api.ft.com/concepts/9577c6d4-b09e-4552-b88f-e52745abe02b",
			Type:      "http://www.ft.com/ontology/Topic",
			PrefLabel: "US interest rates",
		},
	},
}

var augmentedAnnotationsAfterAddition = annotations.Annotations{
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
			Predicate: "http://www.ft.com/ontology/annotation/about",
			ConceptId: "http://www.ft.com/thing/9577c6d4-b09e-4552-b88f-e52745abe02b",
			ApiUrl:    "http://api.ft.com/concepts/9577c6d4-b09e-4552-b88f-e52745abe02b",
			Type:      "http://www.ft.com/ontology/Topic",
			PrefLabel: "US interest rates",
		},
		{
			Predicate: "http://www.ft.com/ontology/hasDisplayTag",
			ConceptId: "http://www.ft.com/thing/9577c6d4-b09e-4552-b88f-e52745abe02b",
			ApiUrl:    "http://api.ft.com/concepts/9577c6d4-b09e-4552-b88f-e52745abe02b",
			Type:      "http://www.ft.com/ontology/Topic",
			PrefLabel: "US interest rates",
		},
		{
			Predicate: "http://www.ft.com/ontology/annotation/mentions",
			ConceptId: "http://www.ft.com/thing/100e3cc0-aecc-4458-8ebd-6b1fbc7345ed",
			ApiUrl:    "http://api.ft.com/organisations/100e3cc0-aecc-4458-8ebd-6b1fbc7345ed",
			Type:      "http://www.ft.com/ontology/organisation/Organisation",
			PrefLabel: "Office for National Statistics UK",
		},
	},
}

var expectedAnnotationsReplace = annotations.Annotations{
	Annotations: []annotations.Annotation{
		{
			Predicate: "http://www.ft.com/ontology/annotation/mentions",
			ConceptId: "http://www.ft.com/thing/0a619d71-9af5-3755-90dd-f789b686c67a",
			ApiUrl:    "http://api.ft.com/people/0a619d71-9af5-3755-90dd-f789b686c67a",
			Type:      "http://www.ft.com/ontology/person/Person",
			PrefLabel: "Barack H. Obama",
		},
	},
}

var expectedAnnotationsReplaceExisting = annotations.Annotations{
	Annotations: []annotations.Annotation{
		{
			Predicate: "http://www.ft.com/ontology/annotation/mentions",
			ConceptId: "http://www.ft.com/thing/0a619d71-9af5-3755-90dd-f789b686c67a",
		},
	},
}

var expectedCanonicalisedAnnotationsBody = annotations.Annotations{
	Annotations: []annotations.Annotation{
		{
			Predicate: "http://www.ft.com/ontology/annotation/about",
			ConceptId: "http://www.ft.com/thing/9577c6d4-b09e-4552-b88f-e52745abe02b",
		},
		{
			Predicate: "http://www.ft.com/ontology/annotation/hasAuthor",
			ConceptId: "http://www.ft.com/thing/838b3fbe-efbc-3cfe-b5c0-d38c046492a4",
		},
		{
			Predicate: "http://www.ft.com/ontology/annotation/mentions",
			ConceptId: "http://www.ft.com/thing/0a619d71-9af5-3755-90dd-f789b686c67a",
		},
		{
			Predicate: "http://www.ft.com/ontology/hasDisplayTag",
			ConceptId: "http://www.ft.com/thing/9577c6d4-b09e-4552-b88f-e52745abe02b",
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
			ConceptId: "http://www.ft.com/thing/0a619d71-9af5-3755-90dd-f789b686c67a",
		},
	},
}

var augmentedAnnotationsAfterDelete = annotations.Annotations{
	Annotations: []annotations.Annotation{
		{
			Predicate: "http://www.ft.com/ontology/annotation/hasAuthor",
			ConceptId: "http://www.ft.com/thing/838b3fbe-efbc-3cfe-b5c0-d38c046492a4",
			ApiUrl:    "http://api.ft.com/people/838b3fbe-efbc-3cfe-b5c0-d38c046492a4",
			Type:      "http://www.ft.com/ontology/person/Person",
			PrefLabel: "David J Lynch",
		},
		{
			Predicate: "http://www.ft.com/ontology/annotation/mentions",
			ConceptId: "http://www.ft.com/thing/0a619d71-9af5-3755-90dd-f789b686c67a",
			ApiUrl:    "http://api.ft.com/people/0a619d71-9af5-3755-90dd-f789b686c67a",
			Type:      "http://www.ft.com/ontology/person/Person",
			PrefLabel: "Barack H. Obama",
		},
	},
}

var expectedCanonicalisedAnnotationsAfterAdditon = annotations.Annotations{
	Annotations: []annotations.Annotation{
		{
			Predicate: "http://www.ft.com/ontology/annotation/about",
			ConceptId: "http://www.ft.com/thing/9577c6d4-b09e-4552-b88f-e52745abe02b",
		},
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
			ConceptId: "http://www.ft.com/thing/100e3cc0-aecc-4458-8ebd-6b1fbc7345ed",
		},
		{
			Predicate: "http://www.ft.com/ontology/hasDisplayTag",
			ConceptId: "http://www.ft.com/thing/9577c6d4-b09e-4552-b88f-e52745abe02b",
		},
	},
}

var expectedCanonicalisedAnnotationsAfterReplace = annotations.Annotations{
	Annotations: []annotations.Annotation{
		{
			Predicate: "http://www.ft.com/ontology/annotation/about",
			ConceptId: "http://www.ft.com/thing/100e3cc0-aecc-4458-8ebd-6b1fbc7345ed",
		},
		{
			Predicate: "http://www.ft.com/ontology/annotation/hasAuthor",
			ConceptId: "http://www.ft.com/thing/838b3fbe-efbc-3cfe-b5c0-d38c046492a4",
		},
		{
			Predicate: "http://www.ft.com/ontology/annotation/mentions",
			ConceptId: "http://www.ft.com/thing/0a619d71-9af5-3755-90dd-f789b686c67a",
		},
		{
			Predicate: "http://www.ft.com/ontology/hasDisplayTag",
			ConceptId: "http://www.ft.com/thing/100e3cc0-aecc-4458-8ebd-6b1fbc7345ed",
		},
	},
}

var augmentedAnnotationsAfterReplace = annotations.Annotations{
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
			Predicate: "http://www.ft.com/ontology/annotation/about",
			ConceptId: "http://www.ft.com/thing/100e3cc0-aecc-4458-8ebd-6b1fbc7345ed",
			ApiUrl:    "http://api.ft.com/organisations/100e3cc0-aecc-4458-8ebd-6b1fbc7345ed",
			Type:      "http://www.ft.com/ontology/organisation/Organisation",
			PrefLabel: "Office for National Statistics UK",
		},
		{
			Predicate: "http://www.ft.com/ontology/hasDisplayTag",
			ConceptId: "http://www.ft.com/thing/100e3cc0-aecc-4458-8ebd-6b1fbc7345ed",
			ApiUrl:    "http://api.ft.com/organisations/100e3cc0-aecc-4458-8ebd-6b1fbc7345ed",
			Type:      "http://www.ft.com/ontology/organisation/Organisation",
			PrefLabel: "Office for National Statistics UK",
		},
	},
}

var expectedCanonicalisedAnnotationsSameConceptId = annotations.Annotations{
	Annotations: []annotations.Annotation{
		{
			Predicate: "http://www.ft.com/ontology/annotation/about",
			ConceptId: "http://www.ft.com/thing/9577c6d4-b09e-4552-b88f-e52745abe02b",
		},
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
			ConceptId: "http://www.ft.com/thing/838b3fbe-efbc-3cfe-b5c0-d38c046492a4",
		},
		{
			Predicate: "http://www.ft.com/ontology/hasDisplayTag",
			ConceptId: "http://www.ft.com/thing/9577c6d4-b09e-4552-b88f-e52745abe02b",
		},
	},
}

var augmentedAnnotationsSameConceptId = annotations.Annotations{
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
			ConceptId: "http://www.ft.com/thing/838b3fbe-efbc-3cfe-b5c0-d38c046492a4",
			ApiUrl:    "http://api.ft.com/people/838b3fbe-efbc-3cfe-b5c0-d38c046492a4",
			Type:      "http://www.ft.com/ontology/person/Person",
			PrefLabel: "David J Lynch",
		},
		{
			Predicate: "http://www.ft.com/ontology/annotation/about",
			ConceptId: "http://www.ft.com/thing/9577c6d4-b09e-4552-b88f-e52745abe02b",
			ApiUrl:    "http://api.ft.com/concepts/9577c6d4-b09e-4552-b88f-e52745abe02b",
			Type:      "http://www.ft.com/ontology/Topic",
			PrefLabel: "US interest rates",
		},
		{
			Predicate: "http://www.ft.com/ontology/hasDisplayTag",
			ConceptId: "http://www.ft.com/thing/9577c6d4-b09e-4552-b88f-e52745abe02b",
			ApiUrl:    "http://api.ft.com/concepts/9577c6d4-b09e-4552-b88f-e52745abe02b",
			Type:      "http://www.ft.com/ontology/Topic",
			PrefLabel: "US interest rates",
		},
	},
}

func TestSaveAnnotations(t *testing.T) {
	oldHash := randomdata.RandStringRunes(56)
	newHash := randomdata.RandStringRunes(56)
	rw := new(RWMock)
	rw.On("Write", mock.AnythingOfType("*context.valueCtx"), "83a201c6-60cd-11e7-91a7-502f7ee26895", &expectedCanonicalisedAnnotationsBody, oldHash).Return(newHash, nil)

	canonicalizer := annotations.NewCanonicalizer(annotations.NewCanonicalAnnotationSorter)
	annotationsAPI := new(AnnotationsAPIMock)
	aug := &AugmenterMock{
		augment: func(ctx context.Context, depletedAnnotations []annotations.Annotation) ([]annotations.Annotation, error) {
			depletedAnnotations = canonicalizer.Canonicalize(depletedAnnotations)
			assert.Equal(t, expectedCanonicalisedAnnotationsBody.Annotations, depletedAnnotations)
			return expectedAnnotations.Annotations, nil
		},
	}

	h := handler.New(rw, annotationsAPI, canonicalizer, aug, time.Second)
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

	h := handler.New(rw, annotationsAPI, annotations.NewCanonicalizer(annotations.NewCanonicalAnnotationSorter), aug, time.Second)
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
	body, err := io.ReadAll(resp.Body)
	assert.NoError(t, err)
	assert.JSONEq(t, fmt.Sprintf(`{"message":"Invalid content UUID: invalid UUID length: %d"}`, len("not-a-valid-uuid")), string(body))

	rw.AssertExpectations(t)
	aug.AssertExpectations(t)
	annotationsAPI.AssertExpectations(t)
}

func TestSaveAnnotationsInvalidAnnotationsBody(t *testing.T) {
	oldHash := randomdata.RandStringRunes(56)
	rw := new(RWMock)
	aug := new(AugmenterMock)
	annotationsAPI := new(AnnotationsAPIMock)

	h := handler.New(rw, annotationsAPI, annotations.NewCanonicalizer(annotations.NewCanonicalAnnotationSorter), aug, time.Second)
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
	body, err := io.ReadAll(resp.Body)
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

	canonicalizer := annotations.NewCanonicalizer(annotations.NewCanonicalAnnotationSorter)
	annotationsAPI := new(AnnotationsAPIMock)
	aug := &AugmenterMock{
		augment: func(ctx context.Context, depletedAnnotations []annotations.Annotation) ([]annotations.Annotation, error) {
			depletedAnnotations = canonicalizer.Canonicalize(depletedAnnotations)
			assert.Equal(t, expectedCanonicalisedAnnotationsBody.Annotations, depletedAnnotations)
			return expectedAnnotations.Annotations, nil
		},
	}

	h := handler.New(rw, annotationsAPI, canonicalizer, aug, time.Second)
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
	body, err := io.ReadAll(resp.Body)
	assert.NoError(t, err)
	assert.JSONEq(t, `{"message":"Error writing draft annotations: computer says no"}`, string(body))

	rw.AssertExpectations(t)
	aug.AssertExpectations(t)
	annotationsAPI.AssertExpectations(t)
}

func TestAnnotationsReadTimeoutGenericRW(t *testing.T) {
	rw := new(RWMock)
	rw.On("Read", mock.Anything, "83a201c6-60cd-11e7-91a7-502f7ee26895").Return(nil, "", false, &url.Error{Err: context.DeadlineExceeded})

	aug := new(AugmenterMock)
	annAPI := new(AnnotationsAPIMock)

	h := handler.New(rw, annAPI, nil, aug, time.Second)
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

	h := handler.New(rw, annAPI, nil, aug, time.Second)
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

	var e net.Error
	assert.True(t, errors.As(err, &e))
	assert.True(t, e.Timeout())
}

func TestAnnotationsWriteTimeout(t *testing.T) {
	oldHash := randomdata.RandStringRunes(56)
	rw := new(RWMock)
	rw.On("Write", mock.AnythingOfType("*context.valueCtx"), "83a201c6-60cd-11e7-91a7-502f7ee26895", &expectedCanonicalisedAnnotationsBody, oldHash).Return("", &url.Error{Err: context.DeadlineExceeded})

	canonicalizer := annotations.NewCanonicalizer(annotations.NewCanonicalAnnotationSorter)
	annotationsAPI := new(AnnotationsAPIMock)
	aug := &AugmenterMock{
		augment: func(ctx context.Context, depletedAnnotations []annotations.Annotation) ([]annotations.Annotation, error) {
			depletedAnnotations = canonicalizer.Canonicalize(depletedAnnotations)
			assert.Equal(t, expectedCanonicalisedAnnotationsBody.Annotations, depletedAnnotations)
			return expectedAnnotations.Annotations, nil
		},
	}

	h := handler.New(rw, annotationsAPI, canonicalizer, aug, time.Second)
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

	body, err := io.ReadAll(resp.Body)
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

	canonicalizer := annotations.NewCanonicalizer(annotations.NewCanonicalAnnotationSorter)

	aug := &AugmenterMock{
		augment: func(ctx context.Context, depletedAnnotations []annotations.Annotation) ([]annotations.Annotation, error) {
			depletedAnnotations = canonicalizer.Canonicalize(depletedAnnotations)
			assert.Equal(t, expectedCanonicalisedAnnotationsAfterDelete.Annotations, depletedAnnotations)
			return augmentedAnnotationsAfterDelete.Annotations, nil
		},
	}

	h := handler.New(rw, annAPI, canonicalizer, aug, time.Second)

	r := vestigo.NewRouter()
	r.Delete("/drafts/content/:uuid/annotations/:cuuid", h.DeleteAnnotation)

	req := httptest.NewRequest(
		"DELETE",
		"http://api.ft.com/drafts/content/83a201c6-60cd-11e7-91a7-502f7ee26895/annotations/9577c6d4-b09e-4552-b88f-e52745abe02b",
		nil)
	req.Header.Set(tidutils.TransactionIDHeader, testTID)
	req.Header.Set(annotations.PreviousDocumentHashHeader, oldHash)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)
	resp := w.Result()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, newHash, resp.Header.Get(annotations.DocumentHashHeader))

	rw.AssertExpectations(t)
	aug.AssertExpectations(t)
	annAPI.AssertExpectations(t)
}

func TestUnHappyDeleteAnnotationsMissingContentUUID(t *testing.T) {
	rw := new(RWMock)
	annAPI := new(AnnotationsAPIMock)
	aug := new(AugmenterMock)

	h := handler.New(rw, annAPI, nil, aug, time.Second)
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

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestUnHappyDeleteAnnotationsInvalidConceptUUID(t *testing.T) {
	rw := new(RWMock)
	annAPI := new(AnnotationsAPIMock)
	aug := new(AugmenterMock)

	h := handler.New(rw, annAPI, nil, aug, time.Second)
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

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestUnHappyDeleteAnnotationsWhenRetrievingAnnotationsFails(t *testing.T) {
	rw := new(RWMock)
	annAPI := new(AnnotationsAPIMock)
	annAPI.On("GetAllButV2", mock.Anything, "83a201c6-60cd-11e7-91a7-502f7ee26895").
		Return([]annotations.Annotation{}, errors.New("sorry something failed"))
	aug := new(AugmenterMock)

	h := handler.New(rw, annAPI, nil, aug, time.Second)
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

func TestUnHappyDeleteAnnotationsWhenNoAnnotationsFound(t *testing.T) {
	rw := new(RWMock)
	annAPI := new(AnnotationsAPIMock)

	uppErr := annotations.NewUPPError(annotations.UPPNotFoundMsg, http.StatusNotFound, nil)

	annAPI.On("GetAllButV2", mock.Anything, "83a201c6-60cd-11e7-91a7-502f7ee26895").
		Return([]annotations.Annotation{}, uppErr)
	aug := new(AugmenterMock)

	h := handler.New(rw, annAPI, nil, aug, time.Second)
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
	defer resp.Body.Close()

	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestUnHappyDeleteAnnotationsWhenWritingAnnotationsFails(t *testing.T) {
	rw := new(RWMock)
	rw.On("Write", mock.AnythingOfType("*context.valueCtx"), "83a201c6-60cd-11e7-91a7-502f7ee26895", &expectedCanonicalisedAnnotationsBody, "").Return(mock.Anything, errors.New("sorry something failed"))
	annAPI := new(AnnotationsAPIMock)
	annAPI.On("GetAllButV2", mock.Anything, "83a201c6-60cd-11e7-91a7-502f7ee26895").
		Return(expectedAnnotations.Annotations, nil)
	canonicalizer := annotations.NewCanonicalizer(annotations.NewCanonicalAnnotationSorter)

	aug := &AugmenterMock{
		augment: func(ctx context.Context, depletedAnnotations []annotations.Annotation) ([]annotations.Annotation, error) {
			depletedAnnotations = canonicalizer.Canonicalize(depletedAnnotations)
			assert.Equal(t, expectedCanonicalisedAnnotationsBody.Annotations, depletedAnnotations)
			return expectedAnnotations.Annotations, nil
		},
	}

	h := handler.New(rw, annAPI, canonicalizer, aug, time.Second)
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

func TestHappyAddAnnotation(t *testing.T) {
	rw := new(RWMock)
	annAPI := new(AnnotationsAPIMock)

	oldHash := randomdata.RandStringRunes(56)
	newHash := randomdata.RandStringRunes(56)

	rw.On("Write", mock.AnythingOfType("*context.valueCtx"), "83a201c6-60cd-11e7-91a7-502f7ee26895", &expectedCanonicalisedAnnotationsAfterAdditon, oldHash).Return(newHash, nil)
	annAPI.On("GetAllButV2", mock.Anything, "83a201c6-60cd-11e7-91a7-502f7ee26895").Return(expectedAnnotations.Annotations, nil)
	canonicalizer := annotations.NewCanonicalizer(annotations.NewCanonicalAnnotationSorter)
	aug := &AugmenterMock{
		augment: func(ctx context.Context, depletedAnnotations []annotations.Annotation) ([]annotations.Annotation, error) {
			depletedAnnotations = canonicalizer.Canonicalize(depletedAnnotations)
			assert.Equal(t, expectedCanonicalisedAnnotationsAfterAdditon.Annotations, depletedAnnotations)
			return augmentedAnnotationsAfterAddition.Annotations, nil
		},
	}

	h := handler.New(rw, annAPI, canonicalizer, aug, time.Second)
	r := vestigo.NewRouter()

	r.Post("/drafts/content/:uuid/annotations", h.AddAnnotation)

	ann := annotations.Annotation{
		Predicate: "http://www.ft.com/ontology/annotation/mentions",
		ConceptId: "http://www.ft.com/thing/100e3cc0-aecc-4458-8ebd-6b1fbc7345ed",
	}
	b, _ := json.Marshal(ann)

	req := httptest.NewRequest(
		"POST",
		"http://api.ft.com/drafts/content/83a201c6-60cd-11e7-91a7-502f7ee26895/annotations",
		bytes.NewBuffer(b))

	req.Header.Set(tidutils.TransactionIDHeader, testTID)
	req.Header.Set(annotations.PreviousDocumentHashHeader, oldHash)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)
	resp := w.Result()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, newHash, resp.Header.Get(annotations.DocumentHashHeader))

	rw.AssertExpectations(t)
	annAPI.AssertExpectations(t)
	aug.AssertExpectations(t)
}

func TestHappyAddExistingAnnotation(t *testing.T) {
	rw := new(RWMock)
	annAPI := new(AnnotationsAPIMock)

	oldHash := randomdata.RandStringRunes(56)
	newHash := randomdata.RandStringRunes(56)

	rw.On("Write", mock.AnythingOfType("*context.valueCtx"), "83a201c6-60cd-11e7-91a7-502f7ee26895", &expectedCanonicalisedAnnotationsBody, oldHash).Return(newHash, nil)
	annAPI.On("GetAllButV2", mock.Anything, "83a201c6-60cd-11e7-91a7-502f7ee26895").Return(expectedAnnotations.Annotations, nil)
	canonicalizer := annotations.NewCanonicalizer(annotations.NewCanonicalAnnotationSorter)
	aug := &AugmenterMock{
		augment: func(ctx context.Context, depletedAnnotations []annotations.Annotation) ([]annotations.Annotation, error) {
			depletedAnnotations = canonicalizer.Canonicalize(depletedAnnotations)
			assert.Equal(t, expectedCanonicalisedAnnotationsBody.Annotations, depletedAnnotations)
			return expectedAnnotations.Annotations, nil
		},
	}

	h := handler.New(rw, annAPI, canonicalizer, aug, time.Second)
	r := vestigo.NewRouter()

	r.Post("/drafts/content/:uuid/annotations", h.AddAnnotation)

	ann := annotations.Annotation{
		Predicate: "http://www.ft.com/ontology/annotation/mentions",
		ConceptId: "http://www.ft.com/thing/0a619d71-9af5-3755-90dd-f789b686c67a",
	}
	b, _ := json.Marshal(ann)

	req := httptest.NewRequest(
		"POST",
		"http://api.ft.com/drafts/content/83a201c6-60cd-11e7-91a7-502f7ee26895/annotations",
		bytes.NewBuffer(b))

	req.Header.Set(tidutils.TransactionIDHeader, testTID)
	req.Header.Set(annotations.PreviousDocumentHashHeader, oldHash)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)
	resp := w.Result()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, newHash, resp.Header.Get(annotations.DocumentHashHeader))

	rw.AssertExpectations(t)
	aug.AssertExpectations(t)
	annAPI.AssertExpectations(t)
}

func TestHappyAddAnnotationWithExistingConceptIdDifferentPredicate(t *testing.T) {
	rw := new(RWMock)
	oldHash := randomdata.RandStringRunes(56)
	newHash := randomdata.RandStringRunes(56)

	rw.On("Write", mock.AnythingOfType("*context.valueCtx"), "83a201c6-60cd-11e7-91a7-502f7ee26895", &expectedCanonicalisedAnnotationsSameConceptId, oldHash).Return(newHash, nil)
	annAPI := new(AnnotationsAPIMock)
	annAPI.On("GetAllButV2", mock.Anything, "83a201c6-60cd-11e7-91a7-502f7ee26895").Return(expectedAnnotations.Annotations, nil)
	canonicalizer := annotations.NewCanonicalizer(annotations.NewCanonicalAnnotationSorter)
	aug := &AugmenterMock{
		augment: func(ctx context.Context, depletedAnnotations []annotations.Annotation) ([]annotations.Annotation, error) {
			depletedAnnotations = canonicalizer.Canonicalize(depletedAnnotations)
			assert.Equal(t, expectedCanonicalisedAnnotationsSameConceptId.Annotations, depletedAnnotations)
			return augmentedAnnotationsSameConceptId.Annotations, nil
		},
	}

	h := handler.New(rw, annAPI, canonicalizer, aug, time.Second)
	r := vestigo.NewRouter()

	r.Post("/drafts/content/:uuid/annotations", h.AddAnnotation)

	ann := annotations.Annotation{
		Predicate: "http://www.ft.com/ontology/annotation/mentions",
		ConceptId: "http://www.ft.com/thing/838b3fbe-efbc-3cfe-b5c0-d38c046492a4",
	}
	b, _ := json.Marshal(ann)

	req := httptest.NewRequest(
		"POST",
		"http://api.ft.com/drafts/content/83a201c6-60cd-11e7-91a7-502f7ee26895/annotations",
		bytes.NewBuffer(b))

	req.Header.Set(tidutils.TransactionIDHeader, testTID)
	req.Header.Set(annotations.PreviousDocumentHashHeader, oldHash)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)
	resp := w.Result()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, newHash, resp.Header.Get(annotations.DocumentHashHeader))

	rw.AssertExpectations(t)
	aug.AssertExpectations(t)
	annAPI.AssertExpectations(t)
}

func TestUnHappyAddAnnotationInvalidContentId(t *testing.T) {
	rw := new(RWMock)
	annAPI := new(AnnotationsAPIMock)
	aug := new(AugmenterMock)

	h := handler.New(rw, annAPI, nil, aug, time.Second)
	r := vestigo.NewRouter()
	r.Post("/drafts/content/:uuid/annotations", h.AddAnnotation)

	req := httptest.NewRequest(
		"POST",
		"http://api.ft.com/drafts/content/foo/annotations",
		nil)

	req.Header.Set(tidutils.TransactionIDHeader, testTID)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)
	resp := w.Result()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestUnHappyAddAnnotationInvalidConceptIdPrefix(t *testing.T) {
	rw := new(RWMock)
	annAPI := new(AnnotationsAPIMock)
	aug := new(AugmenterMock)

	h := handler.New(rw, annAPI, nil, aug, time.Second)
	r := vestigo.NewRouter()
	r.Post("/drafts/content/:uuid/annotations", h.AddAnnotation)

	ann := annotations.Annotation{
		Predicate: "http://www.ft.com/ontology/annotation/about",
		ConceptId: "http://www.ft.com/thing//838b3fbe-efbc-3cfe-b5c0-d38c046492a4",
	}
	b, _ := json.Marshal(ann)

	req := httptest.NewRequest(
		"POST",
		"http://api.ft.com/drafts/content/83a201c6-60cd-11e7-91a7-502f7ee26895/annotations",
		bytes.NewBuffer(b))

	req.Header.Set(tidutils.TransactionIDHeader, testTID)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)
	resp := w.Result()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestUnHappyAddAnnotationEmptyConceptId(t *testing.T) {
	rw := new(RWMock)
	annAPI := new(AnnotationsAPIMock)
	aug := new(AugmenterMock)

	h := handler.New(rw, annAPI, nil, aug, time.Second)
	r := vestigo.NewRouter()
	r.Post("/drafts/content/:uuid/annotations", h.AddAnnotation)

	ann := annotations.Annotation{
		Predicate: "http://www.ft.com/ontology/annotation/about",
	}
	b, _ := json.Marshal(ann)

	req := httptest.NewRequest(
		"POST",
		"http://api.ft.com/drafts/content/83a201c6-60cd-11e7-91a7-502f7ee26895/annotations",
		bytes.NewBuffer(b))

	req.Header.Set(tidutils.TransactionIDHeader, testTID)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)
	resp := w.Result()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestUnHappyAddAnnotationInvalidConceptUuid(t *testing.T) {
	rw := new(RWMock)
	annAPI := new(AnnotationsAPIMock)
	aug := new(AugmenterMock)

	h := handler.New(rw, annAPI, nil, aug, time.Second)
	r := vestigo.NewRouter()
	r.Post("/drafts/content/:uuid/annotations", h.AddAnnotation)

	ann := annotations.Annotation{
		Predicate: "http://www.ft.com/ontology/annotation/about",
		ConceptId: "http://www.ft.com/thing//838b3fbe",
	}
	b, _ := json.Marshal(ann)

	req := httptest.NewRequest(
		"POST",
		"http://api.ft.com/drafts/content/83a201c6-60cd-11e7-91a7-502f7ee26895/annotations",
		bytes.NewBuffer(b))

	req.Header.Set(tidutils.TransactionIDHeader, testTID)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)
	resp := w.Result()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestUnHappyAddAnnotationInvalidPredicate(t *testing.T) {
	rw := new(RWMock)
	annAPI := new(AnnotationsAPIMock)
	aug := new(AugmenterMock)

	h := handler.New(rw, annAPI, nil, aug, time.Second)
	r := vestigo.NewRouter()
	r.Add("POST", "/drafts/content/:uuid/annotations", h.AddAnnotation)

	ann := annotations.Annotation{
		Predicate: "http://www.ft.com/ontology/annotation/foobar",
		ConceptId: "http://www.ft.com/thing/0a619d71-9af5-3755-90dd-f789b686c67a",
	}
	b, _ := json.Marshal(ann)

	req := httptest.NewRequest(
		"POST",
		"http://api.ft.com/drafts/content/83a201c6-60cd-11e7-91a7-502f7ee26895/annotations",
		bytes.NewBuffer(b))

	req.Header.Set(tidutils.TransactionIDHeader, testTID)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)
	resp := w.Result()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestUnhappyAddAnnotationWhenWritingAnnotationsFails(t *testing.T) {
	rw := new(RWMock)
	annAPI := new(AnnotationsAPIMock)

	rw.On("Write", mock.AnythingOfType("*context.valueCtx"), "83a201c6-60cd-11e7-91a7-502f7ee26895", &expectedCanonicalisedAnnotationsAfterAdditon, "").Return(mock.Anything, errors.New("error writing annotations"))
	annAPI.On("GetAllButV2", mock.Anything, "83a201c6-60cd-11e7-91a7-502f7ee26895").Return(expectedAnnotations.Annotations, nil)
	canonicalizer := annotations.NewCanonicalizer(annotations.NewCanonicalAnnotationSorter)
	aug := &AugmenterMock{
		augment: func(ctx context.Context, depletedAnnotations []annotations.Annotation) ([]annotations.Annotation, error) {
			depletedAnnotations = canonicalizer.Canonicalize(depletedAnnotations)
			assert.Equal(t, expectedCanonicalisedAnnotationsAfterAdditon.Annotations, depletedAnnotations)
			return augmentedAnnotationsAfterAddition.Annotations, nil
		},
	}

	h := handler.New(rw, annAPI, canonicalizer, aug, time.Second)
	r := vestigo.NewRouter()

	r.Post("/drafts/content/:uuid/annotations", h.AddAnnotation)

	ann := annotations.Annotation{
		Predicate: "http://www.ft.com/ontology/annotation/mentions",
		ConceptId: "http://www.ft.com/thing/100e3cc0-aecc-4458-8ebd-6b1fbc7345ed",
	}
	b, _ := json.Marshal(ann)

	req := httptest.NewRequest(
		"POST",
		"http://api.ft.com/drafts/content/83a201c6-60cd-11e7-91a7-502f7ee26895/annotations",
		bytes.NewBuffer(b))

	req.Header.Set(tidutils.TransactionIDHeader, testTID)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)
	resp := w.Result()
	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
}

func TestUnhappyAddAnnotationWhenGettingAnnotationsFails(t *testing.T) {
	rw := new(RWMock)
	annAPI := new(AnnotationsAPIMock)
	aug := new(AugmenterMock)

	rw.On("Write", mock.AnythingOfType("*context.valueCtx"), "83a201c6-60cd-11e7-91a7-502f7ee26895", &expectedCanonicalisedAnnotationsAfterAdditon, "").Return(mock.Anything, nil)
	annAPI.On("GetAllButV2", mock.Anything, "83a201c6-60cd-11e7-91a7-502f7ee26895").Return(expectedAnnotations.Annotations, errors.New("error getting annotations"))

	h := handler.New(rw, annAPI, annotations.NewCanonicalizer(annotations.NewCanonicalAnnotationSorter), aug, time.Second)
	r := vestigo.NewRouter()

	r.Post("/drafts/content/:uuid/annotations", h.AddAnnotation)

	ann := annotations.Annotation{
		Predicate: "http://www.ft.com/ontology/annotation/mentions",
		ConceptId: "http://www.ft.com/thing/100e3cc0-aecc-4458-8ebd-6b1fbc7345ed",
	}
	b, _ := json.Marshal(ann)

	req := httptest.NewRequest(
		"POST",
		"http://api.ft.com/drafts/content/83a201c6-60cd-11e7-91a7-502f7ee26895/annotations",
		bytes.NewBuffer(b))

	req.Header.Set(tidutils.TransactionIDHeader, testTID)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)
	resp := w.Result()
	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
}

func TestUnhappyAddAnnotationWhenNoAnnotationsFound(t *testing.T) {
	rw := new(RWMock)
	annAPI := new(AnnotationsAPIMock)
	aug := new(AugmenterMock)

	uppErr := annotations.NewUPPError(annotations.UPPNotFoundMsg, http.StatusNotFound, nil)

	rw.On("Write", mock.AnythingOfType("*context.valueCtx"), "83a201c6-60cd-11e7-91a7-502f7ee26895", &expectedCanonicalisedAnnotationsAfterAdditon, "").Return(mock.Anything, nil)
	annAPI.On("GetAllButV2", mock.Anything, "83a201c6-60cd-11e7-91a7-502f7ee26895").Return(expectedAnnotations.Annotations, uppErr)

	h := handler.New(rw, annAPI, annotations.NewCanonicalizer(annotations.NewCanonicalAnnotationSorter), aug, time.Second)
	r := vestigo.NewRouter()

	r.Post("/drafts/content/:uuid/annotations", h.AddAnnotation)

	ann := annotations.Annotation{
		Predicate: "http://www.ft.com/ontology/annotation/mentions",
		ConceptId: "http://www.ft.com/thing/100e3cc0-aecc-4458-8ebd-6b1fbc7345ed",
	}
	b, _ := json.Marshal(ann)

	req := httptest.NewRequest(
		"POST",
		"http://api.ft.com/drafts/content/83a201c6-60cd-11e7-91a7-502f7ee26895/annotations",
		bytes.NewBuffer(b))

	req.Header.Set(tidutils.TransactionIDHeader, testTID)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)
	resp := w.Result()
	defer resp.Body.Close()

	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestHappyReplaceAnnotation(t *testing.T) {
	rw := new(RWMock)
	annAPI := new(AnnotationsAPIMock)

	oldHash := randomdata.RandStringRunes(56)
	newHash := randomdata.RandStringRunes(56)

	rw.On("Write", mock.AnythingOfType("*context.valueCtx"), "83a201c6-60cd-11e7-91a7-502f7ee26895", &expectedCanonicalisedAnnotationsAfterReplace, oldHash).Return(newHash, nil)
	annAPI.On("GetAllButV2", mock.Anything, "83a201c6-60cd-11e7-91a7-502f7ee26895").Return(expectedAnnotations.Annotations, nil)
	canonicalizer := annotations.NewCanonicalizer(annotations.NewCanonicalAnnotationSorter)
	aug := &AugmenterMock{
		augment: func(ctx context.Context, depletedAnnotations []annotations.Annotation) ([]annotations.Annotation, error) {
			depletedAnnotations = canonicalizer.Canonicalize(depletedAnnotations)
			assert.Equal(t, expectedCanonicalisedAnnotationsAfterReplace.Annotations, depletedAnnotations)
			return augmentedAnnotationsAfterReplace.Annotations, nil
		},
	}

	h := handler.New(rw, annAPI, canonicalizer, aug, time.Second)
	r := vestigo.NewRouter()

	r.Patch("/drafts/content/:uuid/annotations/:cuuid", h.ReplaceAnnotation)

	ann := annotations.Annotation{
		ConceptId: "http://www.ft.com/thing/100e3cc0-aecc-4458-8ebd-6b1fbc7345ed",
	}
	b, _ := json.Marshal(ann)

	req := httptest.NewRequest(
		"PATCH",
		"http://api.ft.com/drafts/content/83a201c6-60cd-11e7-91a7-502f7ee26895/annotations/9577c6d4-b09e-4552-b88f-e52745abe02b",
		bytes.NewBuffer(b))

	req.Header.Set(tidutils.TransactionIDHeader, testTID)
	req.Header.Set(annotations.PreviousDocumentHashHeader, oldHash)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)
	resp := w.Result()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, newHash, resp.Header.Get(annotations.DocumentHashHeader))
}

func TestHappyReplaceAnnotationWithPredicate(t *testing.T) {
	rw := new(RWMock)
	annAPI := new(AnnotationsAPIMock)

	oldHash := randomdata.RandStringRunes(56)
	newHash := randomdata.RandStringRunes(56)

	const contentID = "83a201c6-60cd-11e7-91a7-502f7ee26895"
	fromAnnotationAPI := []annotations.Annotation{
		{
			Predicate: "http://www.ft.com/ontology/annotation/mentions",
			ConceptId: "http://www.ft.com/thing/0a619d71-9af5-3755-90dd-f789b686c67a",
			ApiUrl:    "http://api.ft.com/people/0a619d71-9af5-3755-90dd-f789b686c67a",
			Type:      "http://www.ft.com/ontology/person/Person",
			PrefLabel: "Barack H. Obama",
		},
		{
			Predicate: "http://www.ft.com/ontology/annotation/about",
			ConceptId: "http://www.ft.com/thing/9577c6d4-b09e-4552-b88f-e52745abe02b",
			ApiUrl:    "http://api.ft.com/concepts/9577c6d4-b09e-4552-b88f-e52745abe02b",
			Type:      "http://www.ft.com/ontology/Topic",
			PrefLabel: "US interest rates",
		},
	}
	augmentedAfterReplace := []annotations.Annotation{
		{
			Predicate: "http://www.ft.com/ontology/annotation/mentions",
			ConceptId: "http://www.ft.com/thing/0a619d71-9af5-3755-90dd-f789b686c67a",
			ApiUrl:    "http://api.ft.com/people/0a619d71-9af5-3755-90dd-f789b686c67a",
			Type:      "http://www.ft.com/ontology/person/Person",
			PrefLabel: "Barack H. Obama",
		},
		{
			Predicate: "http://www.ft.com/ontology/hasBrand",
			ConceptId: "http://www.ft.com/thing/100e3cc0-aecc-4458-8ebd-6b1fbc7345ed",
			ApiUrl:    "http://api.ft.com/concepts/100e3cc0-aecc-4458-8ebd-6b1fbc7345ed",
			Type:      "http://www.ft.com/ontology/product/Brand",
			PrefLabel: "Random Brand",
		},
	}
	afterReplace := []annotations.Annotation{
		{
			Predicate: "http://www.ft.com/ontology/annotation/mentions",
			ConceptId: "http://www.ft.com/thing/0a619d71-9af5-3755-90dd-f789b686c67a",
		},
		{
			Predicate: "http://www.ft.com/ontology/hasBrand",
			ConceptId: "http://www.ft.com/thing/100e3cc0-aecc-4458-8ebd-6b1fbc7345ed",
		},
	}

	rw.On("Write", mock.AnythingOfType("*context.valueCtx"), contentID, &annotations.Annotations{Annotations: afterReplace}, oldHash).Return(newHash, nil)
	annAPI.On("GetAllButV2", mock.Anything, contentID).Return(fromAnnotationAPI, nil)

	canonicalizer := annotations.NewCanonicalizer(annotations.NewCanonicalAnnotationSorter)
	aug := &AugmenterMock{
		augment: func(ctx context.Context, depletedAnnotations []annotations.Annotation) ([]annotations.Annotation, error) {
			depletedAnnotations = canonicalizer.Canonicalize(depletedAnnotations)
			assert.Equal(t, afterReplace, depletedAnnotations)
			return augmentedAfterReplace, nil
		},
	}

	h := handler.New(rw, annAPI, canonicalizer, aug, time.Second)
	r := vestigo.NewRouter()

	r.Patch("/drafts/content/:uuid/annotations/:cuuid", h.ReplaceAnnotation)

	ann := annotations.Annotation{
		ConceptId: "http://www.ft.com/thing/100e3cc0-aecc-4458-8ebd-6b1fbc7345ed",
		Predicate: "http://www.ft.com/ontology/hasBrand",
	}
	b, _ := json.Marshal(ann)

	req := httptest.NewRequest(
		"PATCH",
		"/drafts/content/83a201c6-60cd-11e7-91a7-502f7ee26895/annotations/9577c6d4-b09e-4552-b88f-e52745abe02b",
		bytes.NewBuffer(b))

	req.Header.Set(tidutils.TransactionIDHeader, testTID)
	req.Header.Set(annotations.PreviousDocumentHashHeader, oldHash)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)
	resp := w.Result()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, newHash, resp.Header.Get(annotations.DocumentHashHeader))
}

func TestHappyReplaceExistingAnnotation(t *testing.T) {
	rw := new(RWMock)
	annAPI := new(AnnotationsAPIMock)

	oldHash := randomdata.RandStringRunes(56)
	newHash := randomdata.RandStringRunes(56)

	rw.On("Write", mock.AnythingOfType("*context.valueCtx"), "83a201c6-60cd-11e7-91a7-502f7ee26895", &expectedAnnotationsReplaceExisting, oldHash).Return(newHash, nil)
	annAPI.On("GetAllButV2", mock.Anything, "83a201c6-60cd-11e7-91a7-502f7ee26895").Return(expectedAnnotationsReplace.Annotations, nil)
	canonicalizer := annotations.NewCanonicalizer(annotations.NewCanonicalAnnotationSorter)
	aug := &AugmenterMock{
		augment: func(ctx context.Context, depletedAnnotations []annotations.Annotation) ([]annotations.Annotation, error) {
			depletedAnnotations = canonicalizer.Canonicalize(depletedAnnotations)
			assert.Equal(t, expectedAnnotationsReplaceExisting.Annotations, depletedAnnotations)
			return expectedAnnotationsReplace.Annotations, nil
		},
	}

	h := handler.New(rw, annAPI, canonicalizer, aug, time.Second)
	r := vestigo.NewRouter()
	r.Patch("/drafts/content/:uuid/annotations/:cuuid", h.ReplaceAnnotation)

	ann := annotations.Annotation{
		ConceptId: "http://www.ft.com/thing/0a619d71-9af5-3755-90dd-f789b686c67a",
	}
	b, _ := json.Marshal(ann)

	req := httptest.NewRequest(
		"PATCH",
		"http://api.ft.com/drafts/content/83a201c6-60cd-11e7-91a7-502f7ee26895/annotations/0a619d71-9af5-3755-90dd-f789b686c67a",
		bytes.NewBuffer(b))

	req.Header.Set(tidutils.TransactionIDHeader, testTID)
	req.Header.Set(annotations.PreviousDocumentHashHeader, oldHash)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)
	resp := w.Result()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, newHash, resp.Header.Get(annotations.DocumentHashHeader))

	rw.AssertExpectations(t)
	aug.AssertExpectations(t)
	annAPI.AssertExpectations(t)
}

func TestUnHappyReplaceAnnotationsInvalidContentUUID(t *testing.T) {
	rw := new(RWMock)
	annAPI := new(AnnotationsAPIMock)
	aug := new(AugmenterMock)

	h := handler.New(rw, annAPI, nil, aug, time.Second)
	r := vestigo.NewRouter()
	r.Patch("/drafts/content/:uuid/annotations/:cuuid", h.ReplaceAnnotation)

	req := httptest.NewRequest(
		"PATCH",
		"http://api.ft.com/drafts/content/foo/annotations/eccb0da2-54f3-4f9f-bafa-fcec10e1758c",
		nil)

	req.Header.Set(tidutils.TransactionIDHeader, testTID)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)
	resp := w.Result()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestUnHappyReplaceAnnotationInvalidConceptIdInURI(t *testing.T) {
	rw := new(RWMock)
	annAPI := new(AnnotationsAPIMock)
	aug := new(AugmenterMock)

	h := handler.New(rw, annAPI, nil, aug, time.Second)
	r := vestigo.NewRouter()
	r.Patch("/drafts/content/:uuid/annotations/:cuuid", h.ReplaceAnnotation)

	ann := annotations.Annotation{
		ConceptId: "http://www.ft.com/thing/9577c6d4-b09e-4552-b88f-e52745abe02b",
	}
	b, _ := json.Marshal(ann)

	req := httptest.NewRequest(
		"PATCH",
		"http://api.ft.com/drafts/content/83a201c6-60cd-11e7-91a7-502f7ee26895/annotations/bar",
		bytes.NewBuffer(b))

	req.Header.Set(tidutils.TransactionIDHeader, testTID)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)
	resp := w.Result()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestUnHappyReplaceAnnotationEmptyBody(t *testing.T) {
	rw := new(RWMock)
	annAPI := new(AnnotationsAPIMock)
	aug := new(AugmenterMock)

	h := handler.New(rw, annAPI, nil, aug, time.Second)
	r := vestigo.NewRouter()
	r.Patch("/drafts/content/:uuid/annotations/:cuuid", h.ReplaceAnnotation)

	req := httptest.NewRequest(
		"PATCH",
		"http://api.ft.com/drafts/content/83a201c6-60cd-11e7-91a7-502f7ee26895/annotations/9577c6d4-b09e-4552-b88f-e52745abe02b",
		nil)

	req.Header.Set(tidutils.TransactionIDHeader, testTID)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)
	resp := w.Result()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestUnHappyReplaceAnnotationInvalidConceptIdInBody(t *testing.T) {
	rw := new(RWMock)
	annAPI := new(AnnotationsAPIMock)
	aug := new(AugmenterMock)

	h := handler.New(rw, annAPI, nil, aug, time.Second)
	r := vestigo.NewRouter()
	r.Patch("/drafts/content/:uuid/annotations/:cuuid", h.ReplaceAnnotation)

	ann := annotations.Annotation{
		ConceptId: "foobar",
	}
	b, _ := json.Marshal(ann)

	req := httptest.NewRequest(
		"PATCH",
		"http://api.ft.com/drafts/content/83a201c6-60cd-11e7-91a7-502f7ee26895/annotations/9577c6d4-b09e-4552-b88f-e52745abe02b",
		bytes.NewBuffer(b))

	req.Header.Set(tidutils.TransactionIDHeader, testTID)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)
	resp := w.Result()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestUnHappyReplaceAnnotationInvalidPredicate(t *testing.T) {
	rw := new(RWMock)
	annAPI := new(AnnotationsAPIMock)
	aug := new(AugmenterMock)

	h := handler.New(rw, annAPI, nil, aug, time.Second)
	r := vestigo.NewRouter()
	r.Patch("/drafts/content/:uuid/annotations/:cuuid", h.ReplaceAnnotation)

	ann := annotations.Annotation{
		ConceptId: "http://www.ft.com/thing/9577c6d4-b09e-4552-b88f-e52745abe02b",
		Predicate: "foo",
	}
	b, _ := json.Marshal(ann)

	req := httptest.NewRequest(
		"PATCH",
		"/drafts/content/83a201c6-60cd-11e7-91a7-502f7ee26895/annotations/9577c6d4-b09e-4552-b88f-e52745abe02b",
		bytes.NewBuffer(b))

	req.Header.Set(tidutils.TransactionIDHeader, testTID)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)
	resp := w.Result()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestUnhappyReplaceAnnotationWhenWritingAnnotationsFails(t *testing.T) {
	rw := new(RWMock)
	annAPI := new(AnnotationsAPIMock)

	rw.On("Write", mock.AnythingOfType("*context.valueCtx"), "83a201c6-60cd-11e7-91a7-502f7ee26895", &expectedCanonicalisedAnnotationsAfterReplace, "").Return(mock.Anything, errors.New("error writing annotations"))
	annAPI.On("GetAllButV2", mock.Anything, "83a201c6-60cd-11e7-91a7-502f7ee26895").Return(expectedAnnotations.Annotations, nil)
	canonicalizer := annotations.NewCanonicalizer(annotations.NewCanonicalAnnotationSorter)
	aug := &AugmenterMock{
		augment: func(ctx context.Context, depletedAnnotations []annotations.Annotation) ([]annotations.Annotation, error) {
			depletedAnnotations = canonicalizer.Canonicalize(depletedAnnotations)
			assert.Equal(t, expectedCanonicalisedAnnotationsAfterReplace.Annotations, depletedAnnotations)
			return augmentedAnnotationsAfterReplace.Annotations, nil
		},
	}

	h := handler.New(rw, annAPI, canonicalizer, aug, time.Second)
	r := vestigo.NewRouter()
	r.Patch("/drafts/content/:uuid/annotations/:cuuid", h.ReplaceAnnotation)

	ann := annotations.Annotation{
		ConceptId: "http://www.ft.com/thing/100e3cc0-aecc-4458-8ebd-6b1fbc7345ed",
	}
	b, _ := json.Marshal(ann)

	req := httptest.NewRequest(
		"PATCH",
		"http://api.ft.com/drafts/content/83a201c6-60cd-11e7-91a7-502f7ee26895/annotations/9577c6d4-b09e-4552-b88f-e52745abe02b",
		bytes.NewBuffer(b))

	req.Header.Set(tidutils.TransactionIDHeader, testTID)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)
	resp := w.Result()
	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
}

func TestUnhappyReplaceAnnotationWhenGettingAnnotationsFails(t *testing.T) {
	rw := new(RWMock)
	annAPI := new(AnnotationsAPIMock)
	aug := new(AugmenterMock)

	rw.On("Write", mock.AnythingOfType("*context.valueCtx"), "83a201c6-60cd-11e7-91a7-502f7ee26895", &expectedCanonicalisedAnnotationsAfterAdditon, "").Return(mock.Anything, nil)
	annAPI.On("GetAllButV2", mock.Anything, "83a201c6-60cd-11e7-91a7-502f7ee26895").Return(expectedAnnotations.Annotations, errors.New("error getting annotations"))

	h := handler.New(rw, annAPI, annotations.NewCanonicalizer(annotations.NewCanonicalAnnotationSorter), aug, time.Second)
	r := vestigo.NewRouter()
	r.Patch("/drafts/content/:uuid/annotations/:cuuid", h.ReplaceAnnotation)

	ann := annotations.Annotation{
		ConceptId: "http://www.ft.com/thing/100e3cc0-aecc-4458-8ebd-6b1fbc7345ed",
	}
	b, _ := json.Marshal(ann)

	req := httptest.NewRequest(
		"PATCH",
		"http://api.ft.com/drafts/content/83a201c6-60cd-11e7-91a7-502f7ee26895/annotations/9577c6d4-b09e-4552-b88f-e52745abe02b",
		bytes.NewBuffer(b))

	req.Header.Set(tidutils.TransactionIDHeader, testTID)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)
	resp := w.Result()
	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
}

func TestUnhappyReplaceAnnotationWhenNoAnnotationsFound(t *testing.T) {
	rw := new(RWMock)
	annAPI := new(AnnotationsAPIMock)
	aug := new(AugmenterMock)

	uppErr := annotations.NewUPPError(annotations.UPPNotFoundMsg, http.StatusNotFound, nil)

	rw.On("Write", mock.AnythingOfType("*context.valueCtx"), "83a201c6-60cd-11e7-91a7-502f7ee26895", &expectedCanonicalisedAnnotationsAfterAdditon, "").Return(mock.Anything, nil)
	annAPI.On("GetAllButV2", mock.Anything, "83a201c6-60cd-11e7-91a7-502f7ee26895").Return(expectedAnnotations.Annotations, uppErr)

	h := handler.New(rw, annAPI, annotations.NewCanonicalizer(annotations.NewCanonicalAnnotationSorter), aug, time.Second)
	r := vestigo.NewRouter()
	r.Patch("/drafts/content/:uuid/annotations/:cuuid", h.ReplaceAnnotation)

	ann := annotations.Annotation{
		ConceptId: "http://www.ft.com/thing/100e3cc0-aecc-4458-8ebd-6b1fbc7345ed",
	}
	b, _ := json.Marshal(ann)

	req := httptest.NewRequest(
		"PATCH",
		"http://api.ft.com/drafts/content/83a201c6-60cd-11e7-91a7-502f7ee26895/annotations/9577c6d4-b09e-4552-b88f-e52745abe02b",
		bytes.NewBuffer(b))

	req.Header.Set(tidutils.TransactionIDHeader, testTID)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)
	resp := w.Result()
	defer resp.Body.Close()

	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

type AugmenterMock struct {
	mock.Mock
	augment func(ctx context.Context, depletedAnnotations []annotations.Annotation) ([]annotations.Annotation, error)
}

func (m *AugmenterMock) AugmentAnnotations(ctx context.Context, depletedAnnotations []annotations.Annotation) ([]annotations.Annotation, error) {
	if m.augment != nil {
		return m.augment(ctx, depletedAnnotations)
	}
	args := m.Called(ctx, depletedAnnotations)
	return args.Get(0).([]annotations.Annotation), args.Error(1)
}

type RWMock struct {
	mock.Mock
	read     func(ctx context.Context, contentUUID string) (*annotations.Annotations, string, bool, error)
	write    func(ctx context.Context, contentUUID string, a *annotations.Annotations, hash string) (string, error)
	endpoint func() string
	gtg      func() error
}

func (m *RWMock) Read(ctx context.Context, contentUUID string) (*annotations.Annotations, string, bool, error) {
	if m.read != nil {
		return m.read(ctx, contentUUID)
	}

	args := m.Called(ctx, contentUUID)

	var ann *annotations.Annotations
	if v := args.Get(0); v != nil {
		ann = v.(*annotations.Annotations)
	}

	return ann, args.String(1), args.Bool(2), args.Error(3)
}

func (m *RWMock) Write(ctx context.Context, contentUUID string, a *annotations.Annotations, hash string) (string, error) {
	if m.write != nil {
		return m.write(ctx, contentUUID, a, hash)
	}
	args := m.Called(ctx, contentUUID, a, hash)
	return args.String(0), args.Error(1)
}

func (m *RWMock) Endpoint() string {
	if m.endpoint != nil {
		return m.endpoint()
	}
	args := m.Called()
	return args.String(0)
}

func (m *RWMock) GTG() error {
	if m.gtg != nil {
		return m.gtg()
	}
	args := m.Called()
	return args.Error(0)
}

type AnnotationsAPIMock struct {
	mock.Mock
	getAll      func(ctx context.Context, contentUUID string) ([]annotations.Annotation, error)
	getAllButV2 func(ctx context.Context, contentUUID string) ([]annotations.Annotation, error)
	endpoint    func() string
	gtg         func() error
}

func (m *AnnotationsAPIMock) GetAll(ctx context.Context, contentUUID string) ([]annotations.Annotation, error) {
	if m.getAll != nil {
		return m.getAll(ctx, contentUUID)
	}
	args := m.Called(ctx, contentUUID)
	return args.Get(0).([]annotations.Annotation), args.Error(1)
}

func (m *AnnotationsAPIMock) GetAllButV2(ctx context.Context, contentUUID string) ([]annotations.Annotation, error) {
	if m.getAllButV2 != nil {
		return m.getAllButV2(ctx, contentUUID)
	}
	args := m.Called(ctx, contentUUID)
	return args.Get(0).([]annotations.Annotation), args.Error(1)
}

func (m *AnnotationsAPIMock) Endpoint() string {
	if m.endpoint != nil {
		return m.endpoint()
	}
	args := m.Called()
	return args.String(0)
}

func (m *AnnotationsAPIMock) GTG() error {
	if m.gtg != nil {
		return m.gtg()
	}
	args := m.Called()
	return args.Error(0)
}
