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
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Financial-Times/cm-annotations-ontology/validator"
	"github.com/Financial-Times/go-logger/v2"
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/require"

	"github.com/Financial-Times/draft-annotations-api/annotations"
	"github.com/Financial-Times/draft-annotations-api/handler"
	"github.com/Financial-Times/go-ft-http/fthttp"
	tidutils "github.com/Financial-Times/transactionid-utils-go"
	randomdata "github.com/Pallinder/go-randomdata"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

const (
	testTID               = "test_tid"
	testBasicAuthUsername = "username"
	testBasicAuthPassword = "password"
)

var testClient = fthttp.NewClientWithDefaultTimeout("PAC", "draft-annotations-api")

func TestHappyFetchFromAnnotationsRW(t *testing.T) {
	_ = os.Setenv("JSON_SCHEMAS_PATH", "../schemas")
	_ = os.Setenv("JSON_SCHEMAS_API_CONFIG_PATH", "../config/schemas-api-config.json")
	_ = os.Setenv("JSON_SCHEMA_NAME", "draft-annotations-pac-add.json;draft-annotations-pac-replace.json;draft-annotations-pac-write.json;draft-annotations-sv-add.json;draft-annotations-sv-replace.json;draft-annotations-sv-write.json")
	hash := randomdata.RandStringRunes(56)

	rw := new(RWMock)
	rw.On("Read", mock.Anything, "83a201c6-60cd-11e7-91a7-502f7ee26895").Return(expectedAnnotations, hash, true, nil)
	aug := new(AugmenterMock)
	aug.On("AugmentAnnotations", mock.Anything, expectedAnnotations["annotations"]).Return(expectedAnnotations["annotations"], nil)
	annAPI := new(AnnotationsAPIMock)

	log := logger.NewUPPLogger("draft-annotations-api", "INFO")
	v := validator.NewSchemaValidator(log).GetJSONValidator()
	h := handler.New(rw, annAPI, nil, aug, v, time.Second, log)
	r := mux.NewRouter()
	r.HandleFunc("/draft-annotations/content/{uuid}/annotations", h.ReadAnnotations).Methods(http.MethodGet)

	req := httptest.NewRequest(http.MethodGet, "http://api.ft.com/draft-annotations/content/83a201c6-60cd-11e7-91a7-502f7ee26895/annotations", nil)
	req.Header.Set(tidutils.TransactionIDHeader, testTID)
	req.Header.Set(annotations.OriginSystemIDHeader, annotations.PACOriginSystemID)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)
	resp := w.Result()
	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(resp.Body)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	actual := make(map[string]interface{})
	err := json.NewDecoder(resp.Body).Decode(&actual)
	assert.NoError(t, err)

	assert.Equal(t, expectedAnnotations, actual)
	assert.Equal(t, hash, resp.Header.Get(annotations.DocumentHashHeader))

	rw.AssertExpectations(t)
	aug.AssertExpectations(t)
	annAPI.AssertExpectations(t)
}

func TestUnhappyFetchFromAnnotationsRWUnauthorized(t *testing.T) {
	_ = os.Setenv("JSON_SCHEMAS_PATH", "../schemas")
	_ = os.Setenv("JSON_SCHEMAS_API_CONFIG_PATH", "../config/schemas-api-config.json")
	_ = os.Setenv("JSON_SCHEMA_NAME", "draft-annotations-pac-add.json;draft-annotations-pac-replace.json;draft-annotations-pac-write.json;draft-annotations-sv-add.json;draft-annotations-sv-replace.json;draft-annotations-sv-write.json")
	hash := randomdata.RandStringRunes(56)

	rw := new(RWMock)
	rw.On("Read", mock.Anything, "83a201c6-60cd-11e7-91a7-502f7ee26895").Return(expectedAnnotationsRead, hash, true, nil)
	aug := new(AugmenterMock)
	aug.On("AugmentAnnotations", mock.Anything, expectedAnnotationsRead["annotations"]).Return(expectedAnnotationsRead["annotations"], nil)
	annAPI := new(AnnotationsAPIMock)

	log := logger.NewUPPLogger("draft-annotations-api", "INFO")
	v := validator.NewSchemaValidator(log).GetJSONValidator()
	h := handler.New(rw, annAPI, nil, aug, v, time.Second, log)
	r := mux.NewRouter()
	r.HandleFunc("/draft-annotations/content/{uuid}/annotations", h.ReadAnnotations).Methods(http.MethodGet)

	req := httptest.NewRequest(http.MethodGet, "http://api.ft.com/draft-annotations/content/83a201c6-60cd-11e7-91a7-502f7ee26895/annotations", nil)
	req.Header.Set("Access-From", "API Gateway")
	req.Header.Set(tidutils.TransactionIDHeader, testTID)
	req.Header.Set(annotations.OriginSystemIDHeader, annotations.PACOriginSystemID)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)
	resp := w.Result()
	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(resp.Body)
	assert.Equal(t, http.StatusForbidden, resp.StatusCode)

	actual := make(map[string]interface{})
	err := json.NewDecoder(resp.Body).Decode(&actual)
	assert.Error(t, err)

	rw.AssertExpectations(t)
	aug.AssertExpectations(t)
	annAPI.AssertExpectations(t)
}

func TestReadHasBrandAnnotation(t *testing.T) {
	_ = os.Setenv("JSON_SCHEMAS_PATH", "../schemas")
	_ = os.Setenv("JSON_SCHEMAS_API_CONFIG_PATH", "../config/schemas-api-config.json")
	_ = os.Setenv("JSON_SCHEMA_NAME", "draft-annotations-pac-add.json;draft-annotations-pac-replace.json;draft-annotations-pac-write.json;draft-annotations-sv-add.json;draft-annotations-sv-replace.json;draft-annotations-sv-write.json")
	tests := map[string]struct {
		readAnnotations     []interface{}
		expectedAnnotations []interface{}
		sendHasBrand        bool
	}{
		"show hasBrand annotations": {
			readAnnotations: []interface{}{
				map[string]interface{}{
					"predicate": "http://www.ft.com/ontology/hasBrand",
					"id":        "http://www.ft.com/thing/87645070-7d8a-492e-9695-bf61ac2b4d18",
					"type":      "http://www.ft.com/ontology/product/Brand",
				},
			},
			expectedAnnotations: []interface{}{
				map[string]interface{}{
					"predicate": "http://www.ft.com/ontology/hasBrand",
					"id":        "http://www.ft.com/thing/87645070-7d8a-492e-9695-bf61ac2b4d18",
					"type":      "http://www.ft.com/ontology/product/Brand",
				},
			},
			sendHasBrand: true,
		},
		"hide hasBrand annotations": {
			readAnnotations: []interface{}{
				map[string]interface{}{
					"predicate": "http://www.ft.com/ontology/hasBrand",
					"id":        "http://www.ft.com/thing/87645070-7d8a-492e-9695-bf61ac2b4d18",
					"type":      "http://www.ft.com/ontology/product/Brand",
				},
			},
			expectedAnnotations: []interface{}{
				map[string]interface{}{
					"predicate": "http://www.ft.com/ontology/classification/isClassifiedBy",
					"id":        "http://www.ft.com/thing/87645070-7d8a-492e-9695-bf61ac2b4d18",
					"type":      "http://www.ft.com/ontology/product/Brand",
				},
			},
			sendHasBrand: false,
		},
	}

	rw := &RWMock{}
	aug := &AugmenterMock{}
	annAPI := &AnnotationsAPIMock{}
	log := logger.NewUPPLogger("draft-annotations-api", "INFO")
	v := validator.NewSchemaValidator(log).GetJSONValidator()
	h := handler.New(rw, annAPI, nil, aug, v, time.Second, log)
	r := mux.NewRouter()
	r.HandleFunc("/draft-annotations/content/{uuid}/annotations", h.ReadAnnotations).Methods(http.MethodGet)

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			hash := randomdata.RandStringRunes(56)
			rw.read = func(_ context.Context, _ string) (map[string]interface{}, string, bool, error) {
				return map[string]interface{}{"annotations": test.readAnnotations}, hash, true, nil
			}
			aug.augment = func(_ context.Context, _ []interface{}) ([]interface{}, error) {
				return test.readAnnotations, nil
			}

			req := httptest.NewRequest(http.MethodGet, "/draft-annotations/content/83a201c6-60cd-11e7-91a7-502f7ee26895/annotations", nil)
			q := req.URL.Query()
			q.Add("sendHasBrand", strconv.FormatBool(test.sendHasBrand))
			req.URL.RawQuery = q.Encode()
			req.Header.Set(tidutils.TransactionIDHeader, testTID)
			req.Header.Set(annotations.OriginSystemIDHeader, annotations.PACOriginSystemID)
			w := httptest.NewRecorder()

			r.ServeHTTP(w, req)
			resp := w.Result()
			defer func(Body io.ReadCloser) {
				_ = Body.Close()
			}(resp.Body)
			assert.Equal(t, http.StatusOK, resp.StatusCode)

			actual := make(map[string]interface{})
			err := json.NewDecoder(resp.Body).Decode(&actual)
			assert.NoError(t, err)

			assert.Equal(t, map[string]interface{}{"annotations": test.expectedAnnotations}, actual)
			assert.Equal(t, hash, resp.Header.Get(annotations.DocumentHashHeader))
		})
	}
}

func TestAddAnnotation(t *testing.T) {
	_ = os.Setenv("JSON_SCHEMAS_PATH", "../schemas")
	_ = os.Setenv("JSON_SCHEMAS_API_CONFIG_PATH", "../config/schemas-api-config.json")
	_ = os.Setenv("JSON_SCHEMA_NAME", "draft-annotations-pac-add.json;draft-annotations-pac-replace.json;draft-annotations-pac-write.json;draft-annotations-sv-add.json;draft-annotations-sv-replace.json;draft-annotations-sv-write.json")
	rw := &RWMock{}
	annAPI := &AnnotationsAPIMock{}
	aug := &AugmenterMock{}

	log := logger.NewUPPLogger("draft-annotations-api", "INFO")
	v := validator.NewSchemaValidator(log).GetJSONValidator()
	h := handler.New(rw, annAPI, annotations.NewCanonicalizer(annotations.NewCanonicalAnnotationSorter), aug, v, time.Second, log)
	router := mux.NewRouter()
	router.HandleFunc("/draft-annotations/content/{uuid}/annotations", h.AddAnnotation).Methods(http.MethodPost)

	oldHash := randomdata.RandStringRunes(56)
	newHash := randomdata.RandStringRunes(56)

	tests := map[string]struct {
		saved             []interface{}
		augmented         []interface{}
		added             map[string]interface{}
		publication       []interface{}
		requestStatusCode int
	}{
		"success - accept hasBrand annotation": {
			augmented: []interface{}{
				map[string]interface{}{
					"predicate": "http://www.ft.com/ontology/hasBrand",
					"id":        "http://www.ft.com/thing/100e3cc0-aecc-4458-8ebd-6b1fbc7345ed",
					"apiUrl":    "http://api.ft.com/concepts/100e3cc0-aecc-4458-8ebd-6b1fbc7345ed",
					"type":      "http://www.ft.com/ontology/product/Brand",
					"prefLabel": "Temp brand",
				},
			},
			saved: []interface{}{
				map[string]interface{}{
					"predicate": "http://www.ft.com/ontology/hasBrand",
					"id":        "http://www.ft.com/thing/100e3cc0-aecc-4458-8ebd-6b1fbc7345ed",
				},
			},
			added: map[string]interface{}{
				"annotation": map[string]interface{}{
					"predicate": "http://www.ft.com/ontology/hasBrand",
					"id":        "http://www.ft.com/thing/100e3cc0-aecc-4458-8ebd-6b1fbc7345ed",
				},
				"publication": []interface{}{"88fdde6c-2aa4-4f78-af02-9f680097cfd6"},
			},
			publication:       []interface{}{"88fdde6c-2aa4-4f78-af02-9f680097cfd6"},
			requestStatusCode: http.StatusOK,
		},
		"success - switch isClassifiedBy to hasBrand annotation": {
			augmented: []interface{}{
				map[string]interface{}{
					"predicate": "http://www.ft.com/ontology/hasBrand",
					"id":        "http://www.ft.com/thing/100e3cc0-aecc-4458-8ebd-6b1fbc7345ed",
					"apiUrl":    "http://api.ft.com/concepts/100e3cc0-aecc-4458-8ebd-6b1fbc7345ed",
					"type":      "http://www.ft.com/ontology/product/Brand",
					"prefLabel": "Temp brand",
				},
			},
			saved: []interface{}{
				map[string]interface{}{
					"predicate": "http://www.ft.com/ontology/hasBrand",
					"id":        "http://www.ft.com/thing/100e3cc0-aecc-4458-8ebd-6b1fbc7345ed",
				},
			},
			added: map[string]interface{}{
				"annotation": map[string]interface{}{
					"predicate": "http://www.ft.com/ontology/classification/isClassifiedBy",
					"id":        "http://www.ft.com/thing/100e3cc0-aecc-4458-8ebd-6b1fbc7345ed",
				},
				"publication": []interface{}{"88fdde6c-2aa4-4f78-af02-9f680097cfd6"},
			},
			publication:       []interface{}{"88fdde6c-2aa4-4f78-af02-9f680097cfd6"},
			requestStatusCode: http.StatusOK,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			calledGetAll := false
			rw.write = func(_ context.Context, _ string, a map[string]interface{}, hash string) (string, error) {
				assert.Equal(t, map[string]interface{}{"annotations": test.saved, "publication": test.publication}, a)
				assert.Equal(t, oldHash, hash)
				return newHash, nil
			}
			annAPI.getAllButV2 = func(_ context.Context, _ string) ([]interface{}, error) {
				calledGetAll = true
				return []interface{}{}, nil
			}
			aug.augment = func(_ context.Context, depletedAnnotations []interface{}) ([]interface{}, error) {
				expect := []interface{}{test.added["annotation"]}
				assert.Equal(t, expect, depletedAnnotations)
				return test.augmented, nil
			}

			b, _ := json.Marshal(test.added)

			req := httptest.NewRequest(
				http.MethodPost,
				"http://api.ft.com/draft-annotations/content/83a201c6-60cd-11e7-91a7-502f7ee26895/annotations",
				bytes.NewBuffer(b))

			req.Header.Set(tidutils.TransactionIDHeader, testTID)
			req.Header.Set(annotations.PreviousDocumentHashHeader, oldHash)
			req.Header.Set(annotations.OriginSystemIDHeader, annotations.PACOriginSystemID)
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			resp := w.Result()
			defer func(Body io.ReadCloser) {
				_ = Body.Close()
			}(resp.Body)
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
	_ = os.Setenv("JSON_SCHEMAS_PATH", "../schemas")
	_ = os.Setenv("JSON_SCHEMAS_API_CONFIG_PATH", "../config/schemas-api-config.json")
	_ = os.Setenv("JSON_SCHEMA_NAME", "draft-annotations-pac-add.json;draft-annotations-pac-replace.json;draft-annotations-pac-write.json;draft-annotations-sv-add.json;draft-annotations-sv-replace.json;draft-annotations-sv-write.json")
	rw := &RWMock{}
	annAPI := &AnnotationsAPIMock{}
	aug := &AugmenterMock{}

	log := logger.NewUPPLogger("draft-annotations-api", "INFO")
	v := validator.NewSchemaValidator(log).GetJSONValidator()
	h := handler.New(rw, annAPI, annotations.NewCanonicalizer(annotations.NewCanonicalAnnotationSorter), aug, v, time.Second, log)
	router := mux.NewRouter()
	router.HandleFunc("/draft-annotations/content/{uuid}/annotations", h.WriteAnnotations).Methods(http.MethodPut)

	oldHash := randomdata.RandStringRunes(56)
	newHash := randomdata.RandStringRunes(56)

	tests := map[string]struct {
		written           []interface{}
		augmented         []interface{}
		saved             []interface{}
		publication       []interface{}
		requestStatusCode int
	}{
		"success - accept hasBrand annotation": {
			written: []interface{}{
				map[string]interface{}{
					"predicate": "http://www.ft.com/ontology/hasBrand",
					"id":        "http://www.ft.com/thing/100e3cc0-aecc-4458-8ebd-6b1fbc7345ed",
				},
			},
			augmented: []interface{}{
				map[string]interface{}{
					"predicate": "http://www.ft.com/ontology/hasBrand",
					"id":        "http://www.ft.com/thing/100e3cc0-aecc-4458-8ebd-6b1fbc7345ed",
					"apiUrl":    "http://api.ft.com/concepts/100e3cc0-aecc-4458-8ebd-6b1fbc7345ed",
					"type":      "http://www.ft.com/ontology/product/Brand",
					"prefLabel": "Temp brand",
				},
			},
			saved: []interface{}{
				map[string]interface{}{
					"predicate": "http://www.ft.com/ontology/hasBrand",
					"id":        "http://www.ft.com/thing/100e3cc0-aecc-4458-8ebd-6b1fbc7345ed",
				},
			},
			publication:       []interface{}{"88fdde6c-2aa4-4f78-af02-9f680097cfd6"},
			requestStatusCode: http.StatusOK,
		},
		"success - switch isClassifiedBy to hasBrand annotation": {
			written: []interface{}{
				map[string]interface{}{
					"predicate": "http://www.ft.com/ontology/classification/isClassifiedBy",
					"id":        "http://www.ft.com/thing/100e3cc0-aecc-4458-8ebd-6b1fbc7345ed",
				},
			},
			augmented: []interface{}{
				map[string]interface{}{
					"predicate": "http://www.ft.com/ontology/hasBrand",
					"id":        "http://www.ft.com/thing/100e3cc0-aecc-4458-8ebd-6b1fbc7345ed",
					"apiUrl":    "http://api.ft.com/concepts/100e3cc0-aecc-4458-8ebd-6b1fbc7345ed",
					"type":      "http://www.ft.com/ontology/product/Brand",
					"prefLabel": "Temp brand",
				},
			},
			saved: []interface{}{
				map[string]interface{}{
					"predicate": "http://www.ft.com/ontology/hasBrand",
					"id":        "http://www.ft.com/thing/100e3cc0-aecc-4458-8ebd-6b1fbc7345ed",
				},
			},
			publication:       []interface{}{"88fdde6c-2aa4-4f78-af02-9f680097cfd6"},
			requestStatusCode: http.StatusOK,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			rw.write = func(_ context.Context, _ string, a map[string]interface{}, hash string) (string, error) {
				assert.Equal(t, map[string]interface{}{"annotations": test.saved, "publication": test.publication}, a)
				assert.Equal(t, oldHash, hash)
				return newHash, nil
			}
			aug.augment = func(_ context.Context, depletedAnnotations []interface{}) ([]interface{}, error) {
				assert.Equal(t, test.written, depletedAnnotations)
				return test.augmented, nil
			}

			b, _ := json.Marshal(map[string]interface{}{"annotations": test.written, "publication": test.publication})

			req := httptest.NewRequest(
				http.MethodPut,
				"http://api.ft.com/draft-annotations/content/83a201c6-60cd-11e7-91a7-502f7ee26895/annotations",
				bytes.NewBuffer(b))

			req.Header.Set(tidutils.TransactionIDHeader, testTID)
			req.Header.Set(annotations.PreviousDocumentHashHeader, oldHash)
			req.Header.Set(annotations.OriginSystemIDHeader, annotations.PACOriginSystemID)
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			resp := w.Result()
			defer func(Body io.ReadCloser) {
				_ = Body.Close()
			}(resp.Body)
			assert.Equal(t, test.requestStatusCode, resp.StatusCode)

			if test.requestStatusCode != http.StatusOK {
				return
			}
			assert.Equal(t, newHash, resp.Header.Get(annotations.DocumentHashHeader))
		})
	}
}

func TestReplaceHasBrandAnnotation(t *testing.T) {
	_ = os.Setenv("JSON_SCHEMAS_PATH", "../schemas")
	_ = os.Setenv("JSON_SCHEMAS_API_CONFIG_PATH", "../config/schemas-api-config.json")
	_ = os.Setenv("JSON_SCHEMA_NAME", "draft-annotations-pac-add.json;draft-annotations-pac-replace.json;draft-annotations-pac-write.json;draft-annotations-sv-add.json;draft-annotations-sv-replace.json;draft-annotations-sv-write.json")
	rw := &RWMock{}
	annAPI := &AnnotationsAPIMock{}
	aug := &AugmenterMock{}
	canonicalizer := annotations.NewCanonicalizer(annotations.NewCanonicalAnnotationSorter)

	log := logger.NewUPPLogger("draft-annotations-api", "INFO")
	v := validator.NewSchemaValidator(log).GetJSONValidator()
	h := handler.New(rw, annAPI, canonicalizer, aug, v, time.Second, log)
	router := mux.NewRouter()
	router.HandleFunc("/draft-annotations/content/{uuid}/annotations/{cuuid}", h.ReplaceAnnotation).Methods(http.MethodPatch)

	oldHash := randomdata.RandStringRunes(56)
	newHash := randomdata.RandStringRunes(56)

	tests := map[string]struct {
		fromUpp           []interface{}
		toReplace         string
		replaceWith       map[string]interface{}
		afterReplace      []interface{}
		augmented         []interface{}
		toStore           []interface{}
		publication       []interface{}
		requestStatusCode int
	}{
		"success - accept hasBrand annotation": {
			fromUpp: []interface{}{
				map[string]interface{}{
					"predicate": "http://www.ft.com/ontology/annotation/mentions",
					"id":        "http://www.ft.com/thing/0a619d71-9af5-3755-90dd-f789b686c67a",
					"apiUrl":    "http://api.ft.com/people/0a619d71-9af5-3755-90dd-f789b686c67a",
					"type":      "http://www.ft.com/ontology/person/Person",
					"prefLabel": "Barack H. Obama",
				},
				map[string]interface{}{
					"predicate": "http://www.ft.com/ontology/annotation/about",
					"id":        "http://www.ft.com/thing/9577c6d4-b09e-4552-b88f-e52745abe02b",
					"apiUrl":    "http://api.ft.com/concepts/9577c6d4-b09e-4552-b88f-e52745abe02b",
					"type":      "http://www.ft.com/ontology/Topic",
					"prefLabel": "US interest rates",
				},
			},
			toReplace: "9577c6d4-b09e-4552-b88f-e52745abe02b",
			replaceWith: map[string]interface{}{
				"annotation": map[string]interface{}{
					"predicate": "http://www.ft.com/ontology/hasBrand",
					"id":        "http://www.ft.com/thing/100e3cc0-aecc-4458-8ebd-6b1fbc7345ed",
				},
				"publication": []interface{}{"88fdde6c-2aa4-4f78-af02-9f680097cfd6"},
			},
			afterReplace: []interface{}{
				map[string]interface{}{
					"predicate": "http://www.ft.com/ontology/annotation/mentions",
					"id":        "http://www.ft.com/thing/0a619d71-9af5-3755-90dd-f789b686c67a",
				},
				map[string]interface{}{
					"predicate": "http://www.ft.com/ontology/hasBrand",
					"id":        "http://www.ft.com/thing/100e3cc0-aecc-4458-8ebd-6b1fbc7345ed",
				},
			},
			augmented: []interface{}{
				map[string]interface{}{
					"predicate": "http://www.ft.com/ontology/annotation/mentions",
					"id":        "http://www.ft.com/thing/0a619d71-9af5-3755-90dd-f789b686c67a",
					"apiUrl":    "http://api.ft.com/people/0a619d71-9af5-3755-90dd-f789b686c67a",
					"type":      "http://www.ft.com/ontology/person/Person",
					"prefLabel": "Barack H. Obama",
				},
				map[string]interface{}{
					"predicate": "http://www.ft.com/ontology/hasBrand",
					"id":        "http://www.ft.com/thing/100e3cc0-aecc-4458-8ebd-6b1fbc7345ed",
					"apiUrl":    "http://api.ft.com/concepts/100e3cc0-aecc-4458-8ebd-6b1fbc7345ed",
					"type":      "http://www.ft.com/ontology/product/Brand",
					"prefLabel": "Random brand",
				},
			},
			toStore: []interface{}{
				map[string]interface{}{
					"predicate": "http://www.ft.com/ontology/annotation/mentions",
					"id":        "http://www.ft.com/thing/0a619d71-9af5-3755-90dd-f789b686c67a",
				},
				map[string]interface{}{
					"predicate": "http://www.ft.com/ontology/hasBrand",
					"id":        "http://www.ft.com/thing/100e3cc0-aecc-4458-8ebd-6b1fbc7345ed",
				},
			},
			publication:       []interface{}{"88fdde6c-2aa4-4f78-af02-9f680097cfd6"},
			requestStatusCode: http.StatusOK,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			rw.write = func(_ context.Context, _ string, a map[string]interface{}, hash string) (string, error) {
				assert.Equal(t, map[string]interface{}{"annotations": test.toStore, "publication": test.publication}, a)
				assert.Equal(t, oldHash, hash)
				return newHash, nil
			}
			getAllCalled := false
			annAPI.getAllButV2 = func(_ context.Context, _ string) ([]interface{}, error) {
				getAllCalled = true
				return test.fromUpp, nil
			}
			aug.augment = func(_ context.Context, depletedAnnotations []interface{}) ([]interface{}, error) {
				depletedAnnotations = canonicalizer.Canonicalize(depletedAnnotations)
				assert.Equal(t, test.afterReplace, depletedAnnotations)
				return test.augmented, nil
			}

			b, _ := json.Marshal(test.replaceWith)
			target := "http://api.ft.com/draft-annotations/content/83a201c6-60cd-11e7-91a7-502f7ee26895/annotations/" + test.toReplace
			req := httptest.NewRequest(
				http.MethodPatch,
				target,
				bytes.NewBuffer(b))

			req.Header.Set(tidutils.TransactionIDHeader, testTID)
			req.Header.Set(annotations.PreviousDocumentHashHeader, oldHash)
			req.Header.Set(annotations.OriginSystemIDHeader, annotations.PACOriginSystemID)
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			resp := w.Result()
			defer func(Body io.ReadCloser) {
				_ = Body.Close()
			}(resp.Body)
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
	_ = os.Setenv("JSON_SCHEMAS_PATH", "../schemas")
	_ = os.Setenv("JSON_SCHEMAS_API_CONFIG_PATH", "../config/schemas-api-config.json")
	_ = os.Setenv("JSON_SCHEMA_NAME", "draft-annotations-pac-add.json;draft-annotations-pac-replace.json;draft-annotations-pac-write.json;draft-annotations-sv-add.json;draft-annotations-sv-replace.json;draft-annotations-sv-write.json")
	rw := new(RWMock)
	rw.On("Read", mock.Anything, "83a201c6-60cd-11e7-91a7-502f7ee26895").Return(nil, "", false, errors.New("computer says no"))
	aug := new(AugmenterMock)
	annAPI := new(AnnotationsAPIMock)

	log := logger.NewUPPLogger("draft-annotations-api", "INFO")
	v := validator.NewSchemaValidator(log).GetJSONValidator()
	h := handler.New(rw, annAPI, nil, aug, v, time.Second, log)
	r := mux.NewRouter()
	r.HandleFunc("/draft-annotations/content/{uuid}/annotations", h.ReadAnnotations).Methods(http.MethodGet)

	req := httptest.NewRequest(http.MethodGet, "http://api.ft.com/draft-annotations/content/83a201c6-60cd-11e7-91a7-502f7ee26895/annotations", nil)
	req.Header.Set(tidutils.TransactionIDHeader, testTID)
	req.Header.Set(annotations.OriginSystemIDHeader, annotations.PACOriginSystemID)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)
	resp := w.Result()
	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(resp.Body)
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
	_ = os.Setenv("JSON_SCHEMAS_PATH", "../schemas")
	_ = os.Setenv("JSON_SCHEMAS_API_CONFIG_PATH", "../config/schemas-api-config.json")
	_ = os.Setenv("JSON_SCHEMA_NAME", "draft-annotations-pac-add.json;draft-annotations-pac-replace.json;draft-annotations-pac-write.json;draft-annotations-sv-add.json;draft-annotations-sv-replace.json;draft-annotations-sv-write.json")
	rw := new(RWMock)
	rw.On("Read", mock.Anything, "83a201c6-60cd-11e7-91a7-502f7ee26895").Return(expectedAnnotations, "", true, nil)
	aug := new(AugmenterMock)
	aug.On("AugmentAnnotations", mock.Anything, expectedAnnotations["annotations"]).Return(expectedAnnotations["annotations"], errors.New("computer says no"))
	annAPI := new(AnnotationsAPIMock)

	log := logger.NewUPPLogger("draft-annotations-api", "INFO")
	v := validator.NewSchemaValidator(log).GetJSONValidator()
	h := handler.New(rw, annAPI, nil, aug, v, time.Second, log)
	r := mux.NewRouter()
	r.HandleFunc("/draft-annotations/content/{uuid}/annotations", h.ReadAnnotations).Methods(http.MethodGet)

	req := httptest.NewRequest(http.MethodGet, "http://api.ft.com/draft-annotations/content/83a201c6-60cd-11e7-91a7-502f7ee26895/annotations", nil)
	req.Header.Set(tidutils.TransactionIDHeader, testTID)
	req.Header.Set(annotations.OriginSystemIDHeader, annotations.PACOriginSystemID)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)
	resp := w.Result()
	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(resp.Body)
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
	_ = os.Setenv("JSON_SCHEMAS_PATH", "../schemas")
	_ = os.Setenv("JSON_SCHEMAS_API_CONFIG_PATH", "../config/schemas-api-config.json")
	_ = os.Setenv("JSON_SCHEMA_NAME", "draft-annotations-pac-add.json;draft-annotations-pac-replace.json;draft-annotations-pac-write.json;draft-annotations-sv-add.json;draft-annotations-sv-replace.json;draft-annotations-sv-write.json")
	aug := new(AugmenterMock)
	aug.On("AugmentAnnotations", mock.Anything, expectedAnnotations["annotations"]).Return(expectedAnnotations["annotations"], nil)

	rw := new(RWMock)

	rw.On("Read", mock.Anything, "83a201c6-60cd-11e7-91a7-502f7ee26895").Return(nil, "", false, nil)

	annotationsAPIServerMock := newAnnotationsAPIServerMock(t, http.StatusOK, annotationsAPIBody)
	defer annotationsAPIServerMock.Close()

	log := logger.NewUPPLogger("draft-annotations-api", "INFO")
	annotationsAPI := annotations.NewUPPAnnotationsAPI(testClient, annotationsAPIServerMock.URL+"/content/%v/annotations", testBasicAuthUsername, testBasicAuthPassword, log)
	assert.Equal(t, annotationsAPIServerMock.URL+"/content/%v/annotations", annotationsAPI.Endpoint())

	v := validator.NewSchemaValidator(log).GetJSONValidator()
	h := handler.New(rw, annotationsAPI, nil, aug, v, time.Second, log)
	r := mux.NewRouter()
	r.HandleFunc("/draft-annotations/content/{uuid}/annotations", h.ReadAnnotations).Methods(http.MethodGet)

	req := httptest.NewRequest(http.MethodGet, "http://api.ft.com/draft-annotations/content/83a201c6-60cd-11e7-91a7-502f7ee26895/annotations", nil)
	req.Header.Set(tidutils.TransactionIDHeader, testTID)
	req.Header.Set(annotations.OriginSystemIDHeader, annotations.PACOriginSystemID)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)
	resp := w.Result()
	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(resp.Body)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	actual := make(map[string]interface{})
	err := json.NewDecoder(resp.Body).Decode(&actual)
	assert.NoError(t, err)

	assert.Equal(t, expectedAnnotations, actual)
	assert.Empty(t, resp.Header.Get(annotations.DocumentHashHeader))

	rw.AssertExpectations(t)
	aug.AssertExpectations(t)
}

func TestFetchFromAnnotationsAPI404(t *testing.T) {
	_ = os.Setenv("JSON_SCHEMAS_PATH", "../schemas")
	_ = os.Setenv("JSON_SCHEMAS_API_CONFIG_PATH", "../config/schemas-api-config.json")
	_ = os.Setenv("JSON_SCHEMA_NAME", "draft-annotations-pac-add.json;draft-annotations-pac-replace.json;draft-annotations-pac-write.json;draft-annotations-sv-add.json;draft-annotations-sv-replace.json;draft-annotations-sv-write.json")
	aug := new(AugmenterMock)
	rw := new(RWMock)
	rw.On("Read", mock.Anything, "83a201c6-60cd-11e7-91a7-502f7ee26895").Return(nil, "", false, nil)

	annotationsAPIServerMock := newAnnotationsAPIServerMock(t, http.StatusNotFound, "not found")
	defer annotationsAPIServerMock.Close()

	log := logger.NewUPPLogger("draft-annotations-api", "INFO")
	v := validator.NewSchemaValidator(log).GetJSONValidator()
	annotationsAPI := annotations.NewUPPAnnotationsAPI(testClient, annotationsAPIServerMock.URL+"/content/%v/annotations", testBasicAuthUsername, testBasicAuthPassword, log)
	h := handler.New(rw, annotationsAPI, nil, aug, v, time.Second, log)
	r := mux.NewRouter()
	r.HandleFunc("/draft-annotations/content/{uuid}/annotations", h.ReadAnnotations).Methods(http.MethodGet)

	req := httptest.NewRequest(http.MethodGet, "http://api.ft.com/draft-annotations/content/83a201c6-60cd-11e7-91a7-502f7ee26895/annotations", nil)
	req.Header.Set(tidutils.TransactionIDHeader, testTID)
	req.Header.Set(annotations.OriginSystemIDHeader, annotations.PACOriginSystemID)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)
	resp := w.Result()
	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(resp.Body)
	body, err := io.ReadAll(resp.Body)

	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	assert.NoError(t, err)
	assert.Equal(t, "not found", string(body))

	rw.AssertExpectations(t)
	aug.AssertExpectations(t)
}

func TestFetchFromAnnotationsAPI404NoAnnoPostMapping(t *testing.T) {
	_ = os.Setenv("JSON_SCHEMAS_PATH", "../schemas")
	_ = os.Setenv("JSON_SCHEMAS_API_CONFIG_PATH", "../config/schemas-api-config.json")
	_ = os.Setenv("JSON_SCHEMA_NAME", "draft-annotations-pac-add.json;draft-annotations-pac-replace.json;draft-annotations-pac-write.json;draft-annotations-sv-add.json;draft-annotations-sv-replace.json;draft-annotations-sv-write.json")
	rw := new(RWMock)
	rw.On("Read", mock.Anything, "83a201c6-60cd-11e7-91a7-502f7ee26895").Return(nil, "", false, nil)
	aug := new(AugmenterMock)

	annotationsAPIServerMock := newAnnotationsAPIServerMock(t, http.StatusOK, bannedAnnotationsAPIBody)
	defer annotationsAPIServerMock.Close()

	log := logger.NewUPPLogger("draft-annotations-api", "INFO")
	v := validator.NewSchemaValidator(log).GetJSONValidator()
	annotationsAPI := annotations.NewUPPAnnotationsAPI(testClient, annotationsAPIServerMock.URL+"/content/%v/annotations", testBasicAuthUsername, testBasicAuthPassword, log)
	h := handler.New(rw, annotationsAPI, nil, aug, v, time.Second, log)
	r := mux.NewRouter()
	r.HandleFunc("/draft-annotations/content/{uuid}/annotations", h.ReadAnnotations).Methods(http.MethodGet)

	req := httptest.NewRequest(http.MethodGet, "http://api.ft.com/draft-annotations/content/83a201c6-60cd-11e7-91a7-502f7ee26895/annotations", nil)
	req.Header.Set(tidutils.TransactionIDHeader, testTID)
	req.Header.Set(annotations.OriginSystemIDHeader, annotations.PACOriginSystemID)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)
	resp := w.Result()
	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(resp.Body)
	body, err := io.ReadAll(resp.Body)

	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	assert.NoError(t, err)
	assert.Equal(t, "{\"message\":\"No annotations found\"}", string(body))

	rw.AssertExpectations(t)
	aug.AssertExpectations(t)
}

func TestFetchFromAnnotationsAPI500(t *testing.T) {
	_ = os.Setenv("JSON_SCHEMAS_PATH", "../schemas")
	_ = os.Setenv("JSON_SCHEMAS_API_CONFIG_PATH", "../config/schemas-api-config.json")
	_ = os.Setenv("JSON_SCHEMA_NAME", "draft-annotations-pac-add.json;draft-annotations-pac-replace.json;draft-annotations-pac-write.json;draft-annotations-sv-add.json;draft-annotations-sv-replace.json;draft-annotations-sv-write.json")
	rw := new(RWMock)
	rw.On("Read", mock.Anything, "83a201c6-60cd-11e7-91a7-502f7ee26895").Return(nil, "", false, nil)
	aug := new(AugmenterMock)
	annotationsAPIServerMock := newAnnotationsAPIServerMock(t, http.StatusInternalServerError, "fire!")
	defer annotationsAPIServerMock.Close()

	log := logger.NewUPPLogger("draft-annotations-api", "INFO")
	v := validator.NewSchemaValidator(log).GetJSONValidator()
	annotationsAPI := annotations.NewUPPAnnotationsAPI(testClient, annotationsAPIServerMock.URL+"/content/%v/annotations", testBasicAuthUsername, testBasicAuthPassword, log)
	h := handler.New(rw, annotationsAPI, nil, aug, v, time.Second, log)
	r := mux.NewRouter()
	r.HandleFunc("/draft-annotations/content/{uuid}/annotations", h.ReadAnnotations).Methods(http.MethodGet)

	req := httptest.NewRequest(http.MethodGet, "http://api.ft.com/draft-annotations/content/83a201c6-60cd-11e7-91a7-502f7ee26895/annotations", nil)
	req.Header.Set(tidutils.TransactionIDHeader, testTID)
	req.Header.Set(annotations.OriginSystemIDHeader, annotations.PACOriginSystemID)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)
	resp := w.Result()
	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(resp.Body)
	body, err := io.ReadAll(resp.Body)

	assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)
	assert.NoError(t, err)
	assert.Equal(t, `{"message":"Service unavailable"}`, string(body))

	rw.AssertExpectations(t)
	aug.AssertExpectations(t)
}

func TestFetchFromAnnotationsAPIWithInvalidURL(t *testing.T) {
	_ = os.Setenv("JSON_SCHEMAS_PATH", "../schemas")
	_ = os.Setenv("JSON_SCHEMAS_API_CONFIG_PATH", "../config/schemas-api-config.json")
	_ = os.Setenv("JSON_SCHEMA_NAME", "draft-annotations-pac-add.json;draft-annotations-pac-replace.json;draft-annotations-pac-write.json;draft-annotations-sv-add.json;draft-annotations-sv-replace.json;draft-annotations-sv-write.json")
	rw := new(RWMock)
	rw.On("Read", mock.Anything, "83a201c6-60cd-11e7-91a7-502f7ee26895").Return(nil, "", false, nil)
	aug := new(AugmenterMock)

	log := logger.NewUPPLogger("draft-annotations-api", "INFO")
	v := validator.NewSchemaValidator(log).GetJSONValidator()
	annotationsAPI := annotations.NewUPPAnnotationsAPI(testClient, ":#", testBasicAuthUsername, testBasicAuthPassword, log)
	h := handler.New(rw, annotationsAPI, nil, aug, v, time.Second, log)
	r := mux.NewRouter()
	r.HandleFunc("/draft-annotations/content/{uuid}/annotations", h.ReadAnnotations).Methods(http.MethodGet)

	req := httptest.NewRequest(http.MethodGet, "http://api.ft.com/draft-annotations/content/83a201c6-60cd-11e7-91a7-502f7ee26895/annotations", nil)
	req.Header.Set(tidutils.TransactionIDHeader, testTID)
	req.Header.Set(annotations.OriginSystemIDHeader, annotations.PACOriginSystemID)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)
	resp := w.Result()
	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(resp.Body)
	body, err := io.ReadAll(resp.Body)

	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
	assert.NoError(t, err)
	assert.Contains(t, string(body), "Failed to read annotations")
	rw.AssertExpectations(t)
	aug.AssertExpectations(t)
}

func TestFetchFromAnnotationsAPIWithConnectionError(t *testing.T) {
	_ = os.Setenv("JSON_SCHEMAS_PATH", "../schemas")
	_ = os.Setenv("JSON_SCHEMAS_API_CONFIG_PATH", "../config/schemas-api-config.json")
	_ = os.Setenv("JSON_SCHEMA_NAME", "draft-annotations-pac-add.json;draft-annotations-pac-replace.json;draft-annotations-pac-write.json;draft-annotations-sv-add.json;draft-annotations-sv-replace.json;draft-annotations-sv-write.json")
	rw := new(RWMock)
	rw.On("Read", mock.Anything, "83a201c6-60cd-11e7-91a7-502f7ee26895").Return(nil, "", false, nil)
	aug := new(AugmenterMock)
	annotationsAPIServerMock := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {}))
	annotationsAPIServerMock.Close()

	log := logger.NewUPPLogger("draft-annotations-api", "INFO")
	v := validator.NewSchemaValidator(log).GetJSONValidator()
	annotationsAPI := annotations.NewUPPAnnotationsAPI(testClient, annotationsAPIServerMock.URL, testBasicAuthUsername, testBasicAuthPassword, log)
	h := handler.New(rw, annotationsAPI, nil, aug, v, time.Second, log)
	r := mux.NewRouter()
	r.HandleFunc("/draft-annotations/content/{uuid}/annotations", h.ReadAnnotations).Methods(http.MethodGet)

	req := httptest.NewRequest(http.MethodGet, "http://api.ft.com/draft-annotations/content/83a201c6-60cd-11e7-91a7-502f7ee26895/annotations", nil)
	req.Header.Set(tidutils.TransactionIDHeader, testTID)
	req.Header.Set(annotations.OriginSystemIDHeader, annotations.PACOriginSystemID)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)
	resp := w.Result()
	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(resp.Body)
	_, err := io.ReadAll(resp.Body)

	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
	assert.NoError(t, err)

	rw.AssertExpectations(t)
	aug.AssertExpectations(t)
}

func newAnnotationsAPIServerMock(t *testing.T, status int, body string) *httptest.Server {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		username, password, ok := r.BasicAuth()
		if !ok {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		if username != testBasicAuthUsername || password != testBasicAuthPassword {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		assert.Equal(t, testTID, r.Header.Get(tidutils.TransactionIDHeader))
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
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

var expectedAnnotations = map[string]interface{}{
	"annotations": []interface{}{
		map[string]interface{}{
			"predicate": "http://www.ft.com/ontology/annotation/mentions",
			"id":        "http://www.ft.com/thing/0a619d71-9af5-3755-90dd-f789b686c67a",
			"apiUrl":    "http://api.ft.com/people/0a619d71-9af5-3755-90dd-f789b686c67a",
			"type":      "http://www.ft.com/ontology/person/Person",
			"prefLabel": "Barack H. Obama",
		},
		map[string]interface{}{
			"predicate": "http://www.ft.com/ontology/annotation/hasAuthor",
			"id":        "http://www.ft.com/thing/838b3fbe-efbc-3cfe-b5c0-d38c046492a4",
			"apiUrl":    "http://api.ft.com/people/838b3fbe-efbc-3cfe-b5c0-d38c046492a4",
			"type":      "http://www.ft.com/ontology/person/Person",
			"prefLabel": "David J Lynch",
		},
		map[string]interface{}{
			"predicate": "http://www.ft.com/ontology/annotation/about",
			"id":        "http://www.ft.com/thing/9577c6d4-b09e-4552-b88f-e52745abe02b",
			"apiUrl":    "http://api.ft.com/concepts/9577c6d4-b09e-4552-b88f-e52745abe02b",
			"type":      "http://www.ft.com/ontology/Topic",
			"prefLabel": "US interest rates",
		},
		map[string]interface{}{
			"predicate": "http://www.ft.com/ontology/hasDisplayTag",
			"id":        "http://www.ft.com/thing/9577c6d4-b09e-4552-b88f-e52745abe02b",
			"apiUrl":    "http://api.ft.com/concepts/9577c6d4-b09e-4552-b88f-e52745abe02b",
			"type":      "http://www.ft.com/ontology/Topic",
			"prefLabel": "US interest rates",
		},
	},
}

var expectedAnnotationsRead = map[string]interface{}{
	"annotations": []interface{}{
		map[string]interface{}{
			"predicate": "http://www.ft.com/ontology/annotation/mentions",
			"id":        "http://www.ft.com/thing/0a619d71-9af5-3755-90dd-f789b686c67a",
			"apiUrl":    "http://api.ft.com/people/0a619d71-9af5-3755-90dd-f789b686c67a",
			"type":      "http://www.ft.com/ontology/person/Person",
			"prefLabel": "Barack H. Obama",
		},
		map[string]interface{}{
			"predicate": "http://www.ft.com/ontology/annotation/hasAuthor",
			"id":        "http://www.ft.com/thing/838b3fbe-efbc-3cfe-b5c0-d38c046492a4",
			"apiUrl":    "http://api.ft.com/people/838b3fbe-efbc-3cfe-b5c0-d38c046492a4",
			"type":      "http://www.ft.com/ontology/person/Person",
			"prefLabel": "David J Lynch",
		},
		map[string]interface{}{
			"predicate": "http://www.ft.com/ontology/annotation/about",
			"id":        "http://www.ft.com/thing/9577c6d4-b09e-4552-b88f-e52745abe02b",
			"apiUrl":    "http://api.ft.com/concepts/9577c6d4-b09e-4552-b88f-e52745abe02b",
			"type":      "http://www.ft.com/ontology/Topic",
			"prefLabel": "US interest rates",
		},
		map[string]interface{}{
			"predicate": "http://www.ft.com/ontology/hasDisplayTag",
			"id":        "http://www.ft.com/thing/9577c6d4-b09e-4552-b88f-e52745abe02b",
			"apiUrl":    "http://api.ft.com/concepts/9577c6d4-b09e-4552-b88f-e52745abe02b",
			"type":      "http://www.ft.com/ontology/Topic",
			"prefLabel": "US interest rates",
		},
	},
	"publication": []interface{}{"8e6c705e-1132-42a2-8db0-c295e29e8658"},
}

var expectedAnnotationsWithPublication = map[string]interface{}{
	"annotations": []interface{}{
		map[string]interface{}{
			"predicate": "http://www.ft.com/ontology/annotation/mentions",
			"id":        "http://www.ft.com/thing/0a619d71-9af5-3755-90dd-f789b686c67a",
			"apiUrl":    "http://api.ft.com/people/0a619d71-9af5-3755-90dd-f789b686c67a",
			"type":      "http://www.ft.com/ontology/person/Person",
			"prefLabel": "Barack H. Obama",
		},
		map[string]interface{}{
			"predicate": "http://www.ft.com/ontology/annotation/hasAuthor",
			"id":        "http://www.ft.com/thing/838b3fbe-efbc-3cfe-b5c0-d38c046492a4",
			"apiUrl":    "http://api.ft.com/people/838b3fbe-efbc-3cfe-b5c0-d38c046492a4",
			"type":      "http://www.ft.com/ontology/person/Person",
			"prefLabel": "David J Lynch",
		},
		map[string]interface{}{
			"predicate": "http://www.ft.com/ontology/annotation/about",
			"id":        "http://www.ft.com/thing/9577c6d4-b09e-4552-b88f-e52745abe02b",
			"apiUrl":    "http://api.ft.com/concepts/9577c6d4-b09e-4552-b88f-e52745abe02b",
			"type":      "http://www.ft.com/ontology/Topic",
			"prefLabel": "US interest rates",
		},
		map[string]interface{}{
			"predicate": "http://www.ft.com/ontology/hasDisplayTag",
			"id":        "http://www.ft.com/thing/9577c6d4-b09e-4552-b88f-e52745abe02b",
			"apiUrl":    "http://api.ft.com/concepts/9577c6d4-b09e-4552-b88f-e52745abe02b",
			"type":      "http://www.ft.com/ontology/Topic",
			"prefLabel": "US interest rates",
		},
	},
	"publication": []interface{}{"88fdde6c-2aa4-4f78-af02-9f680097cfd6"},
}

var augmentedAnnotationsAfterAddition = map[string]interface{}{
	"annotations": []interface{}{
		map[string]interface{}{
			"predicate": "http://www.ft.com/ontology/annotation/mentions",
			"id":        "http://www.ft.com/thing/0a619d71-9af5-3755-90dd-f789b686c67a",
			"apiUrl":    "http://api.ft.com/people/0a619d71-9af5-3755-90dd-f789b686c67a",
			"type":      "http://www.ft.com/ontology/person/Person",
			"prefLabel": "Barack H. Obama",
		},
		map[string]interface{}{
			"predicate": "http://www.ft.com/ontology/annotation/hasAuthor",
			"id":        "http://www.ft.com/thing/838b3fbe-efbc-3cfe-b5c0-d38c046492a4",
			"apiUrl":    "http://api.ft.com/people/838b3fbe-efbc-3cfe-b5c0-d38c046492a4",
			"type":      "http://www.ft.com/ontology/person/Person",
			"prefLabel": "David J Lynch",
		},
		map[string]interface{}{
			"predicate": "http://www.ft.com/ontology/annotation/about",
			"id":        "http://www.ft.com/thing/9577c6d4-b09e-4552-b88f-e52745abe02b",
			"apiUrl":    "http://api.ft.com/concepts/9577c6d4-b09e-4552-b88f-e52745abe02b",
			"type":      "http://www.ft.com/ontology/Topic",
			"prefLabel": "US interest rates",
		},
		map[string]interface{}{
			"predicate": "http://www.ft.com/ontology/hasDisplayTag",
			"id":        "http://www.ft.com/thing/9577c6d4-b09e-4552-b88f-e52745abe02b",
			"apiUrl":    "http://api.ft.com/concepts/9577c6d4-b09e-4552-b88f-e52745abe02b",
			"type":      "http://www.ft.com/ontology/Topic",
			"prefLabel": "US interest rates",
		},
		map[string]interface{}{
			"predicate": "http://www.ft.com/ontology/annotation/mentions",
			"id":        "http://www.ft.com/thing/100e3cc0-aecc-4458-8ebd-6b1fbc7345ed",
			"apiUrl":    "http://api.ft.com/organisations/100e3cc0-aecc-4458-8ebd-6b1fbc7345ed",
			"type":      "http://www.ft.com/ontology/organisation/Organisation",
			"prefLabel": "Office for National Statistics UK",
		},
	},
}

var expectedAnnotationsReplace = map[string]interface{}{
	"annotations": []interface{}{
		map[string]interface{}{
			"predicate": "http://www.ft.com/ontology/annotation/mentions",
			"id":        "http://www.ft.com/thing/0a619d71-9af5-3755-90dd-f789b686c67a",
			"apiUrl":    "http://api.ft.com/people/0a619d71-9af5-3755-90dd-f789b686c67a",
			"type":      "http://www.ft.com/ontology/person/Person",
			"prefLabel": "Barack H. Obama",
		},
	},
}

var expectedAnnotationsReplaceExisting = map[string]interface{}{
	"annotations": []interface{}{
		map[string]interface{}{
			"predicate": "http://www.ft.com/ontology/annotation/mentions",
			"id":        "http://www.ft.com/thing/0a619d71-9af5-3755-90dd-f789b686c67a",
		},
	},
	"publication": []interface{}{"88fdde6c-2aa4-4f78-af02-9f680097cfd6"},
}

var expectedCanonicalisedAnnotationsBody = map[string]interface{}{
	"annotations": []interface{}{
		map[string]interface{}{
			"predicate": "http://www.ft.com/ontology/annotation/about",
			"id":        "http://www.ft.com/thing/9577c6d4-b09e-4552-b88f-e52745abe02b",
		},
		map[string]interface{}{
			"predicate": "http://www.ft.com/ontology/annotation/hasAuthor",
			"id":        "http://www.ft.com/thing/838b3fbe-efbc-3cfe-b5c0-d38c046492a4",
		},
		map[string]interface{}{
			"predicate": "http://www.ft.com/ontology/annotation/mentions",
			"id":        "http://www.ft.com/thing/0a619d71-9af5-3755-90dd-f789b686c67a",
		},
		map[string]interface{}{
			"predicate": "http://www.ft.com/ontology/hasDisplayTag",
			"id":        "http://www.ft.com/thing/9577c6d4-b09e-4552-b88f-e52745abe02b",
		},
	},
	"publication": []interface{}{"88fdde6c-2aa4-4f78-af02-9f680097cfd6"},
}

var expectedCanonicalisedAnnotationsBodyWriteWithPublication = map[string]interface{}{
	"annotations": []interface{}{
		map[string]interface{}{
			"predicate": "http://www.ft.com/ontology/annotation/about",
			"id":        "http://www.ft.com/thing/9577c6d4-b09e-4552-b88f-e52745abe02b",
		},
		map[string]interface{}{
			"predicate": "http://www.ft.com/ontology/annotation/hasAuthor",
			"id":        "http://www.ft.com/thing/838b3fbe-efbc-3cfe-b5c0-d38c046492a4",
		},
		map[string]interface{}{
			"predicate": "http://www.ft.com/ontology/annotation/mentions",
			"id":        "http://www.ft.com/thing/0a619d71-9af5-3755-90dd-f789b686c67a",
		},
		map[string]interface{}{
			"predicate": "http://www.ft.com/ontology/hasDisplayTag",
			"id":        "http://www.ft.com/thing/9577c6d4-b09e-4552-b88f-e52745abe02b",
		},
	},
	"publication": []interface{}{"88fdde6c-2aa4-4f78-af02-9f680097cfd6"},
}

var expectedCanonicalisedAnnotationsBodyWrite = map[string]interface{}{
	"annotations": []interface{}{
		map[string]interface{}{
			"predicate": "http://www.ft.com/ontology/annotation/about",
			"id":        "http://www.ft.com/thing/9577c6d4-b09e-4552-b88f-e52745abe02b",
		},
		map[string]interface{}{
			"predicate": "http://www.ft.com/ontology/annotation/hasAuthor",
			"id":        "http://www.ft.com/thing/838b3fbe-efbc-3cfe-b5c0-d38c046492a4",
		},
		map[string]interface{}{
			"predicate": "http://www.ft.com/ontology/annotation/mentions",
			"id":        "http://www.ft.com/thing/0a619d71-9af5-3755-90dd-f789b686c67a",
		},
		map[string]interface{}{
			"predicate": "http://www.ft.com/ontology/hasDisplayTag",
			"id":        "http://www.ft.com/thing/9577c6d4-b09e-4552-b88f-e52745abe02b",
		},
	},
}

var expectedCanonicalisedAnnotationsAfterDelete = map[string]interface{}{
	"annotations": []interface{}{
		map[string]interface{}{
			"predicate": "http://www.ft.com/ontology/annotation/hasAuthor",
			"id":        "http://www.ft.com/thing/838b3fbe-efbc-3cfe-b5c0-d38c046492a4",
		},
		map[string]interface{}{
			"predicate": "http://www.ft.com/ontology/annotation/mentions",
			"id":        "http://www.ft.com/thing/0a619d71-9af5-3755-90dd-f789b686c67a",
		},
	},
}

var augmentedAnnotationsAfterDelete = map[string]interface{}{
	"annotations": []interface{}{
		map[string]interface{}{
			"predicate": "http://www.ft.com/ontology/annotation/hasAuthor",
			"id":        "http://www.ft.com/thing/838b3fbe-efbc-3cfe-b5c0-d38c046492a4",
			"apiUrl":    "http://api.ft.com/people/838b3fbe-efbc-3cfe-b5c0-d38c046492a4",
			"type":      "http://www.ft.com/ontology/person/Person",
			"prefLabel": "David J Lynch",
		},
		map[string]interface{}{
			"predicate": "http://www.ft.com/ontology/annotation/mentions",
			"id":        "http://www.ft.com/thing/0a619d71-9af5-3755-90dd-f789b686c67a",
			"apiUrl":    "http://api.ft.com/people/0a619d71-9af5-3755-90dd-f789b686c67a",
			"type":      "http://www.ft.com/ontology/person/Person",
			"prefLabel": "Barack H. Obama",
		},
	},
}

var expectedCanonicalisedAnnotationsAfterAdditon = map[string]interface{}{
	"annotations": []interface{}{
		map[string]interface{}{
			"predicate": "http://www.ft.com/ontology/annotation/about",
			"id":        "http://www.ft.com/thing/9577c6d4-b09e-4552-b88f-e52745abe02b",
		},
		map[string]interface{}{
			"predicate": "http://www.ft.com/ontology/annotation/hasAuthor",
			"id":        "http://www.ft.com/thing/838b3fbe-efbc-3cfe-b5c0-d38c046492a4",
		},
		map[string]interface{}{
			"predicate": "http://www.ft.com/ontology/annotation/mentions",
			"id":        "http://www.ft.com/thing/0a619d71-9af5-3755-90dd-f789b686c67a",
		},
		map[string]interface{}{
			"predicate": "http://www.ft.com/ontology/annotation/mentions",
			"id":        "http://www.ft.com/thing/100e3cc0-aecc-4458-8ebd-6b1fbc7345ed",
		},
		map[string]interface{}{
			"predicate": "http://www.ft.com/ontology/hasDisplayTag",
			"id":        "http://www.ft.com/thing/9577c6d4-b09e-4552-b88f-e52745abe02b",
		},
	},
	"publication": []interface{}{"88fdde6c-2aa4-4f78-af02-9f680097cfd6"},
}

var expectedCanonicalisedAnnotationsAfterReplace = map[string]interface{}{
	"annotations": []interface{}{
		map[string]interface{}{
			"predicate": "http://www.ft.com/ontology/annotation/about",
			"id":        "http://www.ft.com/thing/100e3cc0-aecc-4458-8ebd-6b1fbc7345ed",
		},
		map[string]interface{}{
			"predicate": "http://www.ft.com/ontology/annotation/hasAuthor",
			"id":        "http://www.ft.com/thing/838b3fbe-efbc-3cfe-b5c0-d38c046492a4",
		},
		map[string]interface{}{
			"predicate": "http://www.ft.com/ontology/annotation/mentions",
			"id":        "http://www.ft.com/thing/0a619d71-9af5-3755-90dd-f789b686c67a",
		},
		map[string]interface{}{
			"predicate": "http://www.ft.com/ontology/hasDisplayTag",
			"id":        "http://www.ft.com/thing/100e3cc0-aecc-4458-8ebd-6b1fbc7345ed",
		},
	},
	"publication": []interface{}{"88fdde6c-2aa4-4f78-af02-9f680097cfd6"},
}

var augmentedAnnotationsAfterReplace = map[string]interface{}{
	"annotations": []interface{}{
		map[string]interface{}{
			"predicate": "http://www.ft.com/ontology/annotation/mentions",
			"id":        "http://www.ft.com/thing/0a619d71-9af5-3755-90dd-f789b686c67a",
			"apiUrl":    "http://api.ft.com/people/0a619d71-9af5-3755-90dd-f789b686c67a",
			"type":      "http://www.ft.com/ontology/person/Person",
			"prefLabel": "Barack H. Obama",
		},
		map[string]interface{}{
			"predicate": "http://www.ft.com/ontology/annotation/hasAuthor",
			"id":        "http://www.ft.com/thing/838b3fbe-efbc-3cfe-b5c0-d38c046492a4",
			"apiUrl":    "http://api.ft.com/people/838b3fbe-efbc-3cfe-b5c0-d38c046492a4",
			"type":      "http://www.ft.com/ontology/person/Person",
			"prefLabel": "David J Lynch",
		},
		map[string]interface{}{
			"predicate": "http://www.ft.com/ontology/annotation/about",
			"id":        "http://www.ft.com/thing/100e3cc0-aecc-4458-8ebd-6b1fbc7345ed",
			"apiUrl":    "http://api.ft.com/organisations/100e3cc0-aecc-4458-8ebd-6b1fbc7345ed",
			"type":      "http://www.ft.com/ontology/organisation/Organisation",
			"prefLabel": "Office for National Statistics UK",
		},
		map[string]interface{}{
			"predicate": "http://www.ft.com/ontology/hasDisplayTag",
			"id":        "http://www.ft.com/thing/100e3cc0-aecc-4458-8ebd-6b1fbc7345ed",
			"apiUrl":    "http://api.ft.com/organisations/100e3cc0-aecc-4458-8ebd-6b1fbc7345ed",
			"type":      "http://www.ft.com/ontology/organisation/Organisation",
			"prefLabel": "Office for National Statistics UK",
		},
	},
}

var expectedCanonicalisedAnnotationsSameConceptID = map[string]interface{}{
	"annotations": []interface{}{
		map[string]interface{}{
			"predicate": "http://www.ft.com/ontology/annotation/about",
			"id":        "http://www.ft.com/thing/9577c6d4-b09e-4552-b88f-e52745abe02b",
		},
		map[string]interface{}{
			"predicate": "http://www.ft.com/ontology/annotation/hasAuthor",
			"id":        "http://www.ft.com/thing/838b3fbe-efbc-3cfe-b5c0-d38c046492a4",
		},
		map[string]interface{}{
			"predicate": "http://www.ft.com/ontology/annotation/mentions",
			"id":        "http://www.ft.com/thing/0a619d71-9af5-3755-90dd-f789b686c67a",
		},
		map[string]interface{}{
			"predicate": "http://www.ft.com/ontology/annotation/mentions",
			"id":        "http://www.ft.com/thing/838b3fbe-efbc-3cfe-b5c0-d38c046492a4",
		},
		map[string]interface{}{
			"predicate": "http://www.ft.com/ontology/hasDisplayTag",
			"id":        "http://www.ft.com/thing/9577c6d4-b09e-4552-b88f-e52745abe02b",
		},
	},
	"publication": []interface{}{"88fdde6c-2aa4-4f78-af02-9f680097cfd6"},
}

var augmentedAnnotationsSameConceptID = map[string]interface{}{
	"annotations": []interface{}{
		map[string]interface{}{
			"predicate": "http://www.ft.com/ontology/annotation/mentions",
			"id":        "http://www.ft.com/thing/0a619d71-9af5-3755-90dd-f789b686c67a",
			"apiUrl":    "http://api.ft.com/people/0a619d71-9af5-3755-90dd-f789b686c67a",
			"type":      "http://www.ft.com/ontology/person/Person",
			"prefLabel": "Barack H. Obama",
		},
		map[string]interface{}{
			"predicate": "http://www.ft.com/ontology/annotation/hasAuthor",
			"id":        "http://www.ft.com/thing/838b3fbe-efbc-3cfe-b5c0-d38c046492a4",
			"apiUrl":    "http://api.ft.com/people/838b3fbe-efbc-3cfe-b5c0-d38c046492a4",
			"type":      "http://www.ft.com/ontology/person/Person",
			"prefLabel": "David J Lynch",
		},
		map[string]interface{}{
			"predicate": "http://www.ft.com/ontology/annotation/mentions",
			"id":        "http://www.ft.com/thing/838b3fbe-efbc-3cfe-b5c0-d38c046492a4",
			"apiUrl":    "http://api.ft.com/people/838b3fbe-efbc-3cfe-b5c0-d38c046492a4",
			"type":      "http://www.ft.com/ontology/person/Person",
			"prefLabel": "David J Lynch",
		},
		map[string]interface{}{
			"predicate": "http://www.ft.com/ontology/annotation/about",
			"id":        "http://www.ft.com/thing/9577c6d4-b09e-4552-b88f-e52745abe02b",
			"apiUrl":    "http://api.ft.com/concepts/9577c6d4-b09e-4552-b88f-e52745abe02b",
			"type":      "http://www.ft.com/ontology/Topic",
			"prefLabel": "US interest rates",
		},
		map[string]interface{}{
			"predicate": "http://www.ft.com/ontology/hasDisplayTag",
			"id":        "http://www.ft.com/thing/9577c6d4-b09e-4552-b88f-e52745abe02b",
			"apiUrl":    "http://api.ft.com/concepts/9577c6d4-b09e-4552-b88f-e52745abe02b",
			"type":      "http://www.ft.com/ontology/Topic",
			"prefLabel": "US interest rates",
		},
	},
}

func TestSaveAnnotations(t *testing.T) {
	_ = os.Setenv("JSON_SCHEMAS_PATH", "../schemas")
	_ = os.Setenv("JSON_SCHEMAS_API_CONFIG_PATH", "../config/schemas-api-config.json")
	_ = os.Setenv("JSON_SCHEMA_NAME", "draft-annotations-pac-add.json;draft-annotations-pac-replace.json;draft-annotations-pac-write.json;draft-annotations-sv-add.json;draft-annotations-sv-replace.json;draft-annotations-sv-write.json")
	oldHash := randomdata.RandStringRunes(56)
	newHash := randomdata.RandStringRunes(56)
	rw := new(RWMock)
	rw.On("Write", mock.Anything, "83a201c6-60cd-11e7-91a7-502f7ee26895", expectedCanonicalisedAnnotationsBodyWriteWithPublication, oldHash).Return(newHash, nil)

	canonicalizer := annotations.NewCanonicalizer(annotations.NewCanonicalAnnotationSorter)
	annotationsAPI := new(AnnotationsAPIMock)
	aug := &AugmenterMock{
		augment: func(_ context.Context, depletedAnnotations []interface{}) ([]interface{}, error) {
			depletedAnnotations = canonicalizer.Canonicalize(depletedAnnotations)
			assert.Equal(t, expectedCanonicalisedAnnotationsBody["annotations"], depletedAnnotations)
			return expectedAnnotationsWithPublication["annotations"].([]interface{}), nil
		},
	}

	log := logger.NewUPPLogger("draft-annotations-api", "INFO")
	v := validator.NewSchemaValidator(log).GetJSONValidator()
	h := handler.New(rw, annotationsAPI, canonicalizer, aug, v, time.Second, log)
	r := mux.NewRouter()
	r.HandleFunc("/draft-annotations/content/{uuid}/annotations", h.WriteAnnotations).Methods(http.MethodPut)

	entity := bytes.Buffer{}
	err := json.NewEncoder(&entity).Encode(&expectedAnnotationsWithPublication)
	if err != nil {
		t.Fatalf("failed to encode annotations: %v", err)
	}

	req := httptest.NewRequest(
		http.MethodPut,
		"http://api.ft.com/draft-annotations/content/83a201c6-60cd-11e7-91a7-502f7ee26895/annotations",
		&entity)

	req.Header.Set(tidutils.TransactionIDHeader, testTID)
	req.Header.Set(annotations.PreviousDocumentHashHeader, oldHash)
	req.Header.Set(annotations.OriginSystemIDHeader, annotations.PACOriginSystemID)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)
	resp := w.Result()
	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(resp.Body)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	actual := make(map[string]interface{})
	err = json.NewDecoder(resp.Body).Decode(&actual)
	assert.NoError(t, err)

	assert.Equal(t, expectedCanonicalisedAnnotationsBodyWriteWithPublication, actual)
	assert.Equal(t, newHash, resp.Header.Get(annotations.DocumentHashHeader))

	rw.AssertExpectations(t)
	aug.AssertExpectations(t)
	annotationsAPI.AssertExpectations(t)
}

func TestSaveAnnotationsInvalidContentUUID(t *testing.T) {
	_ = os.Setenv("JSON_SCHEMAS_PATH", "../schemas")
	_ = os.Setenv("JSON_SCHEMAS_API_CONFIG_PATH", "../config/schemas-api-config.json")
	_ = os.Setenv("JSON_SCHEMA_NAME", "draft-annotations-pac-add.json;draft-annotations-pac-replace.json;draft-annotations-pac-write.json;draft-annotations-sv-add.json;draft-annotations-sv-replace.json;draft-annotations-sv-write.json")
	oldHash := randomdata.RandStringRunes(56)
	rw := new(RWMock)
	aug := new(AugmenterMock)
	annotationsAPI := new(AnnotationsAPIMock)

	log := logger.NewUPPLogger("draft-annotations-api", "INFO")
	v := validator.NewSchemaValidator(log).GetJSONValidator()
	h := handler.New(rw, annotationsAPI, annotations.NewCanonicalizer(annotations.NewCanonicalAnnotationSorter), aug, v, time.Second, log)
	r := mux.NewRouter()
	r.HandleFunc("/draft-annotations/content/{uuid}/annotations", h.WriteAnnotations).Methods(http.MethodPut)

	req := httptest.NewRequest(
		http.MethodPut,
		"http://api.ft.com/draft-annotations/content/not-a-valid-uuid/annotations",
		strings.NewReader(expectedAnnotationsBody))

	req.Header.Set(tidutils.TransactionIDHeader, testTID)
	req.Header.Set(annotations.PreviousDocumentHashHeader, oldHash)
	req.Header.Set(annotations.OriginSystemIDHeader, annotations.PACOriginSystemID)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)
	resp := w.Result()
	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(resp.Body)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	body, err := io.ReadAll(resp.Body)
	assert.NoError(t, err)
	assert.JSONEq(t, fmt.Sprintf(`{"message":"Invalid content UUID: invalid UUID length: %d"}`, len("not-a-valid-uuid")), string(body))

	rw.AssertExpectations(t)
	aug.AssertExpectations(t)
	annotationsAPI.AssertExpectations(t)
}

func TestSaveAnnotationsInvalidAnnotationsBody(t *testing.T) {
	_ = os.Setenv("JSON_SCHEMAS_PATH", "../schemas")
	_ = os.Setenv("JSON_SCHEMAS_API_CONFIG_PATH", "../config/schemas-api-config.json")
	_ = os.Setenv("JSON_SCHEMA_NAME", "draft-annotations-pac-add.json;draft-annotations-pac-replace.json;draft-annotations-pac-write.json;draft-annotations-sv-add.json;draft-annotations-sv-replace.json;draft-annotations-sv-write.json")
	oldHash := randomdata.RandStringRunes(56)
	rw := new(RWMock)
	aug := new(AugmenterMock)
	annotationsAPI := new(AnnotationsAPIMock)

	log := logger.NewUPPLogger("draft-annotations-api", "INFO")
	v := validator.NewSchemaValidator(log).GetJSONValidator()
	h := handler.New(rw, annotationsAPI, annotations.NewCanonicalizer(annotations.NewCanonicalAnnotationSorter), aug, v, time.Second, log)
	r := mux.NewRouter()
	r.HandleFunc("/draft-annotations/content/{uuid}/annotations", h.WriteAnnotations).Methods(http.MethodPut)

	req := httptest.NewRequest(
		http.MethodPut,
		"http://api.ft.com/draft-annotations/content/83a201c6-60cd-11e7-91a7-502f7ee26895/annotations",
		strings.NewReader(`{invalid-json}`))

	req.Header.Set(tidutils.TransactionIDHeader, testTID)
	req.Header.Set(annotations.PreviousDocumentHashHeader, oldHash)
	req.Header.Set(annotations.OriginSystemIDHeader, annotations.PACOriginSystemID)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)
	resp := w.Result()
	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(resp.Body)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	body, err := io.ReadAll(resp.Body)
	assert.NoError(t, err)
	assert.JSONEq(t, `{"message":"Unable to unmarshal annotations body: invalid character 'i' looking for beginning of object key string"}`, string(body))

	rw.AssertExpectations(t)
	aug.AssertExpectations(t)
	annotationsAPI.AssertExpectations(t)
}

func TestSaveAnnotationsErrorFromRW(t *testing.T) {
	_ = os.Setenv("JSON_SCHEMAS_PATH", "../schemas")
	_ = os.Setenv("JSON_SCHEMAS_API_CONFIG_PATH", "../config/schemas-api-config.json")
	_ = os.Setenv("JSON_SCHEMA_NAME", "draft-annotations-pac-add.json;draft-annotations-pac-replace.json;draft-annotations-pac-write.json;draft-annotations-sv-add.json;draft-annotations-sv-replace.json;draft-annotations-sv-write.json")
	oldHash := randomdata.RandStringRunes(56)
	rw := new(RWMock)
	rw.On("Write", mock.Anything, "83a201c6-60cd-11e7-91a7-502f7ee26895", expectedCanonicalisedAnnotationsBodyWriteWithPublication, oldHash).Return("", errors.New("computer says no"))

	canonicalizer := annotations.NewCanonicalizer(annotations.NewCanonicalAnnotationSorter)
	annotationsAPI := new(AnnotationsAPIMock)
	aug := &AugmenterMock{
		augment: func(_ context.Context, depletedAnnotations []interface{}) ([]interface{}, error) {
			depletedAnnotations = canonicalizer.Canonicalize(depletedAnnotations)
			assert.Equal(t, expectedCanonicalisedAnnotationsBody["annotations"], depletedAnnotations)
			return expectedAnnotationsWithPublication["annotations"].([]interface{}), nil
		},
	}

	log := logger.NewUPPLogger("draft-annotations-api", "INFO")
	v := validator.NewSchemaValidator(log).GetJSONValidator()
	h := handler.New(rw, annotationsAPI, canonicalizer, aug, v, time.Second, log)
	r := mux.NewRouter()
	r.HandleFunc("/draft-annotations/content/{uuid}/annotations", h.WriteAnnotations).Methods(http.MethodPut)

	entity := bytes.Buffer{}
	err := json.NewEncoder(&entity).Encode(&expectedAnnotationsWithPublication)
	if err != nil {
		t.Fatalf("failed to encode annotations: %v", err)
	}

	req := httptest.NewRequest(
		http.MethodPut,
		"http://api.ft.com/draft-annotations/content/83a201c6-60cd-11e7-91a7-502f7ee26895/annotations",
		&entity)

	req.Header.Set(tidutils.TransactionIDHeader, testTID)
	req.Header.Set(annotations.PreviousDocumentHashHeader, oldHash)
	req.Header.Set(annotations.OriginSystemIDHeader, annotations.PACOriginSystemID)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)
	resp := w.Result()
	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(resp.Body)
	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
	body, err := io.ReadAll(resp.Body)
	assert.NoError(t, err)
	assert.JSONEq(t, `{"message":"Error writing draft annotations: computer says no"}`, string(body))

	rw.AssertExpectations(t)
	aug.AssertExpectations(t)
	annotationsAPI.AssertExpectations(t)
}

func TestAnnotationsReadTimeoutGenericRW(t *testing.T) {
	_ = os.Setenv("JSON_SCHEMAS_PATH", "../schemas")
	_ = os.Setenv("JSON_SCHEMAS_API_CONFIG_PATH", "../config/schemas-api-config.json")
	_ = os.Setenv("JSON_SCHEMA_NAME", "draft-annotations-pac-add.json;draft-annotations-pac-replace.json;draft-annotations-pac-write.json;draft-annotations-sv-add.json;draft-annotations-sv-replace.json;draft-annotations-sv-write.json")
	rw := new(RWMock)
	rw.On("Read", mock.Anything, "83a201c6-60cd-11e7-91a7-502f7ee26895").Return(nil, "", false, &url.Error{Err: context.DeadlineExceeded})

	aug := new(AugmenterMock)
	annAPI := new(AnnotationsAPIMock)

	log := logger.NewUPPLogger("draft-annotations-api", "INFO")
	v := validator.NewSchemaValidator(log).GetJSONValidator()
	h := handler.New(rw, annAPI, nil, aug, v, time.Second, log)
	r := mux.NewRouter()
	r.HandleFunc("/draft-annotations/content/{uuid}/annotations", h.ReadAnnotations).Methods(http.MethodGet)

	req := httptest.NewRequest(http.MethodGet, "http://api.ft.com/draft-annotations/content/83a201c6-60cd-11e7-91a7-502f7ee26895/annotations", nil)
	req.Header.Set(tidutils.TransactionIDHeader, testTID)
	req.Header.Set(annotations.OriginSystemIDHeader, annotations.PACOriginSystemID)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)
	resp := w.Result()
	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(resp.Body)
	assert.Equal(t, http.StatusGatewayTimeout, resp.StatusCode)
	assert.JSONEq(t, `{"message":"Timeout while reading annotations"}`, w.Body.String())

	rw.AssertExpectations(t)
	aug.AssertExpectations(t)
	annAPI.AssertExpectations(t)
}

func TestAnnotationsReadTimeoutUPP(t *testing.T) {
	_ = os.Setenv("JSON_SCHEMAS_PATH", "../schemas")
	_ = os.Setenv("JSON_SCHEMAS_API_CONFIG_PATH", "../config/schemas-api-config.json")
	_ = os.Setenv("JSON_SCHEMA_NAME", "draft-annotations-pac-add.json;draft-annotations-pac-replace.json;draft-annotations-pac-write.json;draft-annotations-sv-add.json;draft-annotations-sv-replace.json;draft-annotations-sv-write.json")
	rw := new(RWMock)
	rw.On("Read", mock.Anything, "83a201c6-60cd-11e7-91a7-502f7ee26895").Return(nil, "", false, nil)

	aug := new(AugmenterMock)
	annAPI := new(AnnotationsAPIMock)
	annAPI.On("GetAll", mock.Anything, "83a201c6-60cd-11e7-91a7-502f7ee26895").Return([]interface{}{}, &url.Error{Err: context.DeadlineExceeded})

	log := logger.NewUPPLogger("draft-annotations-api", "INFO")
	v := validator.NewSchemaValidator(log).GetJSONValidator()
	h := handler.New(rw, annAPI, nil, aug, v, time.Second, log)
	r := mux.NewRouter()
	r.HandleFunc("/draft-annotations/content/{uuid}/annotations", h.ReadAnnotations).Methods(http.MethodGet)

	req := httptest.NewRequest(http.MethodGet, "http://api.ft.com/draft-annotations/content/83a201c6-60cd-11e7-91a7-502f7ee26895/annotations", nil)
	req.Header.Set(tidutils.TransactionIDHeader, testTID)
	req.Header.Set(annotations.OriginSystemIDHeader, annotations.PACOriginSystemID)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)
	resp := w.Result()
	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(resp.Body)
	assert.Equal(t, http.StatusGatewayTimeout, resp.StatusCode)
	assert.JSONEq(t, `{"message":"Timeout while reading annotations"}`, w.Body.String())

	rw.AssertExpectations(t)
	aug.AssertExpectations(t)
	annAPI.AssertExpectations(t)
}

func TestIsTimeoutErr(t *testing.T) {
	_ = os.Setenv("JSON_SCHEMAS_PATH", "../schemas")
	_ = os.Setenv("JSON_SCHEMAS_API_CONFIG_PATH", "../config/schemas-api-config.json")
	_ = os.Setenv("JSON_SCHEMA_NAME", "draft-annotations-pac-add.json;draft-annotations-pac-replace.json;draft-annotations-pac-write.json;draft-annotations-sv-add.json;draft-annotations-sv-replace.json;draft-annotations-sv-write.json")
	r := mux.NewRouter()
	r.HandleFunc("/", func(_ http.ResponseWriter, _ *http.Request) {
		time.Sleep(500 * time.Millisecond)
	}).Methods(http.MethodGet)

	s := httptest.NewServer(r)

	req, _ := http.NewRequest(http.MethodGet, s.URL+"/", nil)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := http.DefaultClient.Do(req.WithContext(ctx))

	var e net.Error
	assert.True(t, errors.As(err, &e))
	assert.True(t, e.Timeout())
}

func TestAnnotationsWriteTimeout(t *testing.T) {
	_ = os.Setenv("JSON_SCHEMAS_PATH", "../schemas")
	_ = os.Setenv("JSON_SCHEMAS_API_CONFIG_PATH", "../config/schemas-api-config.json")
	_ = os.Setenv("JSON_SCHEMA_NAME", "draft-annotations-pac-add.json;draft-annotations-pac-replace.json;draft-annotations-pac-write.json;draft-annotations-sv-add.json;draft-annotations-sv-replace.json;draft-annotations-sv-write.json")
	oldHash := randomdata.RandStringRunes(56)
	rw := new(RWMock)
	rw.On("Write", mock.Anything, "83a201c6-60cd-11e7-91a7-502f7ee26895", expectedCanonicalisedAnnotationsBodyWriteWithPublication, oldHash).Return("", &url.Error{Err: context.DeadlineExceeded})

	canonicalizer := annotations.NewCanonicalizer(annotations.NewCanonicalAnnotationSorter)
	annotationsAPI := new(AnnotationsAPIMock)
	aug := &AugmenterMock{
		augment: func(_ context.Context, depletedAnnotations []interface{}) ([]interface{}, error) {
			depletedAnnotations = canonicalizer.Canonicalize(depletedAnnotations)
			assert.Equal(t, expectedCanonicalisedAnnotationsBody["annotations"], depletedAnnotations)
			return expectedAnnotationsWithPublication["annotations"].([]interface{}), nil
		},
	}

	log := logger.NewUPPLogger("draft-annotations-api", "INFO")
	v := validator.NewSchemaValidator(log).GetJSONValidator()
	h := handler.New(rw, annotationsAPI, canonicalizer, aug, v, time.Second, log)
	r := mux.NewRouter()
	r.HandleFunc("/draft-annotations/content/{uuid}/annotations", h.WriteAnnotations).Methods(http.MethodPut)

	entity := bytes.Buffer{}
	err := json.NewEncoder(&entity).Encode(&expectedAnnotationsWithPublication)
	if err != nil {
		t.Fatalf("failed to encode annotations: %v", err)
	}

	req := httptest.NewRequest(
		http.MethodPut,
		"http://api.ft.com/draft-annotations/content/83a201c6-60cd-11e7-91a7-502f7ee26895/annotations",
		&entity)

	req.Header.Set(tidutils.TransactionIDHeader, testTID)
	req.Header.Set(annotations.PreviousDocumentHashHeader, oldHash)
	req.Header.Set(annotations.OriginSystemIDHeader, annotations.PACOriginSystemID)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	resp := w.Result()
	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(resp.Body)
	assert.Equal(t, http.StatusGatewayTimeout, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	assert.NoError(t, err)
	assert.JSONEq(t, `{"message":"Timeout while waiting to write draft annotations"}`, string(body))

	rw.AssertExpectations(t)
	aug.AssertExpectations(t)
	annotationsAPI.AssertExpectations(t)
}

func TestHappyDeleteAnnotations(t *testing.T) {
	_ = os.Setenv("JSON_SCHEMAS_PATH", "../schemas")
	_ = os.Setenv("JSON_SCHEMAS_API_CONFIG_PATH", "../config/schemas-api-config.json")
	_ = os.Setenv("JSON_SCHEMA_NAME", "draft-annotations-pac-add.json;draft-annotations-pac-replace.json;draft-annotations-pac-write.json;draft-annotations-sv-add.json;draft-annotations-sv-replace.json;draft-annotations-sv-write.json")
	rw := new(RWMock)
	oldHash := randomdata.RandStringRunes(56)
	newHash := randomdata.RandStringRunes(56)
	rw.On("Write", mock.Anything, "83a201c6-60cd-11e7-91a7-502f7ee26895",
		expectedCanonicalisedAnnotationsAfterDelete, oldHash).Return(newHash, nil)
	annAPI := new(AnnotationsAPIMock)
	annAPI.On("GetAllButV2", mock.Anything, "83a201c6-60cd-11e7-91a7-502f7ee26895").
		Return(expectedAnnotations["annotations"], nil)

	canonicalizer := annotations.NewCanonicalizer(annotations.NewCanonicalAnnotationSorter)

	aug := &AugmenterMock{
		augment: func(_ context.Context, depletedAnnotations []interface{}) ([]interface{}, error) {
			depletedAnnotations = canonicalizer.Canonicalize(depletedAnnotations)
			assert.Equal(t, expectedCanonicalisedAnnotationsAfterDelete["annotations"], depletedAnnotations)
			return augmentedAnnotationsAfterDelete["annotations"].([]interface{}), nil
		},
	}

	log := logger.NewUPPLogger("draft-annotations-api", "INFO")
	v := validator.NewSchemaValidator(log).GetJSONValidator()
	h := handler.New(rw, annAPI, canonicalizer, aug, v, time.Second, log)

	r := mux.NewRouter()
	r.HandleFunc("/draft-annotations/content/{uuid}/annotations/{cuuid}", h.DeleteAnnotation).Methods(http.MethodDelete)

	req := httptest.NewRequest(
		http.MethodDelete,
		"http://api.ft.com/draft-annotations/content/83a201c6-60cd-11e7-91a7-502f7ee26895/annotations/9577c6d4-b09e-4552-b88f-e52745abe02b",
		nil)
	req.Header.Set(tidutils.TransactionIDHeader, testTID)
	req.Header.Set(annotations.PreviousDocumentHashHeader, oldHash)
	req.Header.Set(annotations.OriginSystemIDHeader, annotations.PACOriginSystemID)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)
	resp := w.Result()
	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(resp.Body)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, newHash, resp.Header.Get(annotations.DocumentHashHeader))

	rw.AssertExpectations(t)
	aug.AssertExpectations(t)
	annAPI.AssertExpectations(t)
}

func TestUnHappyDeleteAnnotationsMissingContentUUID(t *testing.T) {
	_ = os.Setenv("JSON_SCHEMAS_PATH", "../schemas")
	_ = os.Setenv("JSON_SCHEMAS_API_CONFIG_PATH", "../config/schemas-api-config.json")
	_ = os.Setenv("JSON_SCHEMA_NAME", "draft-annotations-pac-add.json;draft-annotations-pac-replace.json;draft-annotations-pac-write.json;draft-annotations-sv-add.json;draft-annotations-sv-replace.json;draft-annotations-sv-write.json")
	rw := new(RWMock)
	annAPI := new(AnnotationsAPIMock)
	aug := new(AugmenterMock)

	log := logger.NewUPPLogger("draft-annotations-api", "INFO")
	v := validator.NewSchemaValidator(log).GetJSONValidator()
	h := handler.New(rw, annAPI, nil, aug, v, time.Second, log)
	r := mux.NewRouter()
	r.HandleFunc("/draft-annotations/content/{uuid}/annotations/{cuuid}", h.DeleteAnnotation).Methods(http.MethodDelete)

	req := httptest.NewRequest(
		http.MethodDelete,
		"http://api.ft.com/draft-annotations/content/foo/annotations/eccb0da2-54f3-4f9f-bafa-fcec10e1758c",
		nil)
	req.Header.Set(tidutils.TransactionIDHeader, testTID)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)
	resp := w.Result()
	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(resp.Body)

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestUnHappyDeleteAnnotationsInvalidConceptUUID(t *testing.T) {
	_ = os.Setenv("JSON_SCHEMAS_PATH", "../schemas")
	_ = os.Setenv("JSON_SCHEMAS_API_CONFIG_PATH", "../config/schemas-api-config.json")
	_ = os.Setenv("JSON_SCHEMA_NAME", "draft-annotations-pac-add.json;draft-annotations-pac-replace.json;draft-annotations-pac-write.json;draft-annotations-sv-add.json;draft-annotations-sv-replace.json;draft-annotations-sv-write.json")
	rw := new(RWMock)
	annAPI := new(AnnotationsAPIMock)
	aug := new(AugmenterMock)

	log := logger.NewUPPLogger("draft-annotations-api", "INFO")
	v := validator.NewSchemaValidator(log).GetJSONValidator()
	h := handler.New(rw, annAPI, nil, aug, v, time.Second, log)
	r := mux.NewRouter()
	r.HandleFunc("/draft-annotations/content/{uuid}/annotations/{cuuid}", h.DeleteAnnotation).Methods(http.MethodDelete)

	req := httptest.NewRequest(
		http.MethodDelete,
		"http://api.ft.com/draft-annotations/content/83a201c6-60cd-11e7-91a7-502f7ee26895/annotations/bar",
		nil)
	req.Header.Set(tidutils.TransactionIDHeader, testTID)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)
	resp := w.Result()
	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(resp.Body)

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestUnHappyDeleteAnnotationsWhenRetrievingAnnotationsFails(t *testing.T) {
	_ = os.Setenv("JSON_SCHEMAS_PATH", "../schemas")
	_ = os.Setenv("JSON_SCHEMAS_API_CONFIG_PATH", "../config/schemas-api-config.json")
	_ = os.Setenv("JSON_SCHEMA_NAME", "draft-annotations-pac-add.json;draft-annotations-pac-replace.json;draft-annotations-pac-write.json;draft-annotations-sv-add.json;draft-annotations-sv-replace.json;draft-annotations-sv-write.json")
	rw := new(RWMock)
	annAPI := new(AnnotationsAPIMock)
	annAPI.On("GetAllButV2", mock.Anything, "83a201c6-60cd-11e7-91a7-502f7ee26895").
		Return([]interface{}{}, errors.New("sorry something failed"))
	aug := new(AugmenterMock)

	log := logger.NewUPPLogger("draft-annotations-api", "INFO")
	v := validator.NewSchemaValidator(log).GetJSONValidator()
	h := handler.New(rw, annAPI, nil, aug, v, time.Second, log)
	r := mux.NewRouter()
	r.HandleFunc("/draft-annotations/content/{uuid}/annotations/{cuuid}", h.DeleteAnnotation).Methods(http.MethodDelete)

	req := httptest.NewRequest(
		http.MethodDelete,
		"http://api.ft.com/draft-annotations/content/83a201c6-60cd-11e7-91a7-502f7ee26895/annotations/eccb0da2-54f3-4f9f-bafa-fcec10e1758c",
		nil)
	req.Header.Set(tidutils.TransactionIDHeader, testTID)
	req.Header.Set(annotations.OriginSystemIDHeader, annotations.PACOriginSystemID)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)
	resp := w.Result()
	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(resp.Body)

	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
}

func TestUnHappyDeleteAnnotationsWhenNoAnnotationsFound(t *testing.T) {
	_ = os.Setenv("JSON_SCHEMAS_PATH", "../schemas")
	_ = os.Setenv("JSON_SCHEMAS_API_CONFIG_PATH", "../config/schemas-api-config.json")
	_ = os.Setenv("JSON_SCHEMA_NAME", "draft-annotations-pac-add.json;draft-annotations-pac-replace.json;draft-annotations-pac-write.json;draft-annotations-sv-add.json;draft-annotations-sv-replace.json;draft-annotations-sv-write.json")
	rw := new(RWMock)
	annAPI := new(AnnotationsAPIMock)

	uppErr := annotations.NewUPPError(annotations.UPPNotFoundMsg, http.StatusNotFound, nil)

	annAPI.On("GetAllButV2", mock.Anything, "83a201c6-60cd-11e7-91a7-502f7ee26895").
		Return([]interface{}{}, uppErr)
	aug := new(AugmenterMock)

	log := logger.NewUPPLogger("draft-annotations-api", "INFO")
	v := validator.NewSchemaValidator(log).GetJSONValidator()
	h := handler.New(rw, annAPI, nil, aug, v, time.Second, log)
	r := mux.NewRouter()
	r.HandleFunc("/draft-annotations/content/:uuid/annotations/{cuuid}", h.DeleteAnnotation).Methods(http.MethodDelete)

	req := httptest.NewRequest(
		http.MethodDelete,
		"http://api.ft.com/draft-annotations/content/83a201c6-60cd-11e7-91a7-502f7ee26895/annotations/eccb0da2-54f3-4f9f-bafa-fcec10e1758c",
		nil)
	req.Header.Set(tidutils.TransactionIDHeader, testTID)
	req.Header.Set(annotations.OriginSystemIDHeader, annotations.PACOriginSystemID)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)
	resp := w.Result()
	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(resp.Body)

	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestUnHappyDeleteAnnotationsWhenWritingAnnotationsFails(t *testing.T) {
	_ = os.Setenv("JSON_SCHEMAS_PATH", "../schemas")
	_ = os.Setenv("JSON_SCHEMAS_API_CONFIG_PATH", "../config/schemas-api-config.json")
	_ = os.Setenv("JSON_SCHEMA_NAME", "draft-annotations-pac-add.json;draft-annotations-pac-replace.json;draft-annotations-pac-write.json;draft-annotations-sv-add.json;draft-annotations-sv-replace.json;draft-annotations-sv-write.json")
	rw := new(RWMock)
	rw.On("Write", mock.Anything, "83a201c6-60cd-11e7-91a7-502f7ee26895", expectedCanonicalisedAnnotationsBodyWrite, "").Return(mock.Anything, errors.New("sorry something failed"))
	annAPI := new(AnnotationsAPIMock)
	annAPI.On("GetAllButV2", mock.Anything, "83a201c6-60cd-11e7-91a7-502f7ee26895").
		Return(expectedAnnotations["annotations"], nil)
	canonicalizer := annotations.NewCanonicalizer(annotations.NewCanonicalAnnotationSorter)

	aug := &AugmenterMock{
		augment: func(_ context.Context, depletedAnnotations []interface{}) ([]interface{}, error) {
			depletedAnnotations = canonicalizer.Canonicalize(depletedAnnotations)
			assert.Equal(t, expectedCanonicalisedAnnotationsBody["annotations"], depletedAnnotations)
			return expectedAnnotations["annotations"].([]interface{}), nil
		},
	}

	log := logger.NewUPPLogger("draft-annotations-api", "INFO")
	v := validator.NewSchemaValidator(log).GetJSONValidator()
	h := handler.New(rw, annAPI, canonicalizer, aug, v, time.Second, log)
	r := mux.NewRouter()
	r.HandleFunc("/draft-annotations/content/{uuid}/annotations/{cuuid}", h.DeleteAnnotation).Methods(http.MethodDelete)

	req := httptest.NewRequest(
		http.MethodDelete,
		"http://api.ft.com/draft-annotations/content/83a201c6-60cd-11e7-91a7-502f7ee26895/annotations/eccb0da2-54f3-4f9f-bafa-fcec10e1758c",
		nil)
	req.Header.Set(tidutils.TransactionIDHeader, testTID)
	req.Header.Set(annotations.OriginSystemIDHeader, annotations.PACOriginSystemID)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)
	resp := w.Result()
	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(resp.Body)

	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
}

func TestHappyAddAnnotation(t *testing.T) {
	_ = os.Setenv("JSON_SCHEMAS_PATH", "../schemas")
	_ = os.Setenv("JSON_SCHEMAS_API_CONFIG_PATH", "../config/schemas-api-config.json")
	_ = os.Setenv("JSON_SCHEMA_NAME", "draft-annotations-pac-add.json;draft-annotations-pac-replace.json;draft-annotations-pac-write.json;draft-annotations-sv-add.json;draft-annotations-sv-replace.json;draft-annotations-sv-write.json")
	rw := new(RWMock)
	annAPI := new(AnnotationsAPIMock)

	oldHash := randomdata.RandStringRunes(56)
	newHash := randomdata.RandStringRunes(56)

	rw.On("Write", mock.Anything, "83a201c6-60cd-11e7-91a7-502f7ee26895", expectedCanonicalisedAnnotationsAfterAdditon, oldHash).Return(newHash, nil)
	annAPI.On("GetAllButV2", mock.Anything, "83a201c6-60cd-11e7-91a7-502f7ee26895").Return(expectedAnnotations["annotations"], nil)
	canonicalizer := annotations.NewCanonicalizer(annotations.NewCanonicalAnnotationSorter)
	aug := &AugmenterMock{
		augment: func(_ context.Context, depletedAnnotations []interface{}) ([]interface{}, error) {
			depletedAnnotations = canonicalizer.Canonicalize(depletedAnnotations)
			assert.Equal(t, expectedCanonicalisedAnnotationsAfterAdditon["annotations"], depletedAnnotations)
			return augmentedAnnotationsAfterAddition["annotations"].([]interface{}), nil
		},
	}

	log := logger.NewUPPLogger("draft-annotations-api", "INFO")
	v := validator.NewSchemaValidator(log).GetJSONValidator()
	h := handler.New(rw, annAPI, canonicalizer, aug, v, time.Second, log)
	r := mux.NewRouter()

	r.HandleFunc("/draft-annotations/content/{uuid}/annotations", h.AddAnnotation).Methods(http.MethodPost)

	ann := map[string]interface{}{
		"annotation": map[string]interface{}{
			"predicate": "http://www.ft.com/ontology/annotation/mentions",
			"id":        "http://www.ft.com/thing/100e3cc0-aecc-4458-8ebd-6b1fbc7345ed",
		},
		"publication": []interface{}{"88fdde6c-2aa4-4f78-af02-9f680097cfd6"},
	}
	b, _ := json.Marshal(ann)

	req := httptest.NewRequest(
		http.MethodPost,
		"http://api.ft.com/draft-annotations/content/83a201c6-60cd-11e7-91a7-502f7ee26895/annotations",
		bytes.NewBuffer(b))

	req.Header.Set(tidutils.TransactionIDHeader, testTID)
	req.Header.Set(annotations.PreviousDocumentHashHeader, oldHash)
	req.Header.Set(annotations.OriginSystemIDHeader, annotations.PACOriginSystemID)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)
	resp := w.Result()
	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(resp.Body)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, newHash, resp.Header.Get(annotations.DocumentHashHeader))

	rw.AssertExpectations(t)
	annAPI.AssertExpectations(t)
	aug.AssertExpectations(t)
}

func TestHappyAddExistingAnnotation(t *testing.T) {
	_ = os.Setenv("JSON_SCHEMAS_PATH", "../schemas")
	_ = os.Setenv("JSON_SCHEMAS_API_CONFIG_PATH", "../config/schemas-api-config.json")
	_ = os.Setenv("JSON_SCHEMA_NAME", "draft-annotations-pac-add.json;draft-annotations-pac-replace.json;draft-annotations-pac-write.json;draft-annotations-sv-add.json;draft-annotations-sv-replace.json;draft-annotations-sv-write.json")
	rw := new(RWMock)
	annAPI := new(AnnotationsAPIMock)

	oldHash := randomdata.RandStringRunes(56)
	newHash := randomdata.RandStringRunes(56)

	rw.On("Write", mock.Anything, "83a201c6-60cd-11e7-91a7-502f7ee26895", expectedCanonicalisedAnnotationsBody, oldHash).Return(newHash, nil)
	annAPI.On("GetAllButV2", mock.Anything, "83a201c6-60cd-11e7-91a7-502f7ee26895").Return(expectedAnnotations["annotations"], nil)
	canonicalizer := annotations.NewCanonicalizer(annotations.NewCanonicalAnnotationSorter)
	aug := &AugmenterMock{
		augment: func(_ context.Context, depletedAnnotations []interface{}) ([]interface{}, error) {
			depletedAnnotations = canonicalizer.Canonicalize(depletedAnnotations)
			assert.Equal(t, expectedCanonicalisedAnnotationsBody["annotations"], depletedAnnotations)
			return expectedAnnotations["annotations"].([]interface{}), nil
		},
	}

	log := logger.NewUPPLogger("draft-annotations-api", "INFO")
	v := validator.NewSchemaValidator(log).GetJSONValidator()
	h := handler.New(rw, annAPI, canonicalizer, aug, v, time.Second, log)
	r := mux.NewRouter()

	r.HandleFunc("/draft-annotations/content/{uuid}/annotations", h.AddAnnotation).Methods(http.MethodPost)

	ann := map[string]interface{}{
		"annotation": map[string]interface{}{
			"predicate": "http://www.ft.com/ontology/annotation/mentions",
			"id":        "http://www.ft.com/thing/0a619d71-9af5-3755-90dd-f789b686c67a",
		},
		"publication": []interface{}{"88fdde6c-2aa4-4f78-af02-9f680097cfd6"},
	}
	b, _ := json.Marshal(ann)

	req := httptest.NewRequest(
		http.MethodPost,
		"http://api.ft.com/draft-annotations/content/83a201c6-60cd-11e7-91a7-502f7ee26895/annotations",
		bytes.NewBuffer(b))

	req.Header.Set(tidutils.TransactionIDHeader, testTID)
	req.Header.Set(annotations.PreviousDocumentHashHeader, oldHash)
	req.Header.Set(annotations.OriginSystemIDHeader, annotations.PACOriginSystemID)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)
	resp := w.Result()
	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(resp.Body)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, newHash, resp.Header.Get(annotations.DocumentHashHeader))

	rw.AssertExpectations(t)
	aug.AssertExpectations(t)
	annAPI.AssertExpectations(t)
}

func TestHappyAddAnnotationWithExistingConceptIdDifferentPredicate(t *testing.T) {
	_ = os.Setenv("JSON_SCHEMAS_PATH", "../schemas")
	_ = os.Setenv("JSON_SCHEMAS_API_CONFIG_PATH", "../config/schemas-api-config.json")
	_ = os.Setenv("JSON_SCHEMA_NAME", "draft-annotations-pac-add.json;draft-annotations-pac-replace.json;draft-annotations-pac-write.json;draft-annotations-sv-add.json;draft-annotations-sv-replace.json;draft-annotations-sv-write.json")
	rw := new(RWMock)
	oldHash := randomdata.RandStringRunes(56)
	newHash := randomdata.RandStringRunes(56)

	rw.On("Write", mock.Anything, "83a201c6-60cd-11e7-91a7-502f7ee26895", expectedCanonicalisedAnnotationsSameConceptID, oldHash).Return(newHash, nil)
	annAPI := new(AnnotationsAPIMock)
	annAPI.On("GetAllButV2", mock.Anything, "83a201c6-60cd-11e7-91a7-502f7ee26895").Return(expectedAnnotations["annotations"], nil)
	canonicalizer := annotations.NewCanonicalizer(annotations.NewCanonicalAnnotationSorter)
	aug := &AugmenterMock{
		augment: func(_ context.Context, depletedAnnotations []interface{}) ([]interface{}, error) {
			depletedAnnotations = canonicalizer.Canonicalize(depletedAnnotations)
			assert.Equal(t, expectedCanonicalisedAnnotationsSameConceptID["annotations"], depletedAnnotations)
			return augmentedAnnotationsSameConceptID["annotations"].([]interface{}), nil
		},
	}

	log := logger.NewUPPLogger("draft-annotations-api", "INFO")
	v := validator.NewSchemaValidator(log).GetJSONValidator()
	h := handler.New(rw, annAPI, canonicalizer, aug, v, time.Second, log)
	r := mux.NewRouter()

	r.HandleFunc("/draft-annotations/content/{uuid}/annotations", h.AddAnnotation).Methods(http.MethodPost)

	ann := map[string]interface{}{
		"annotation": map[string]interface{}{
			"predicate": "http://www.ft.com/ontology/annotation/mentions",
			"id":        "http://www.ft.com/thing/838b3fbe-efbc-3cfe-b5c0-d38c046492a4",
		},
		"publication": []interface{}{"88fdde6c-2aa4-4f78-af02-9f680097cfd6"},
	}
	b, _ := json.Marshal(ann)

	req := httptest.NewRequest(
		http.MethodPost,
		"http://api.ft.com/draft-annotations/content/83a201c6-60cd-11e7-91a7-502f7ee26895/annotations",
		bytes.NewBuffer(b))

	req.Header.Set(tidutils.TransactionIDHeader, testTID)
	req.Header.Set(annotations.PreviousDocumentHashHeader, oldHash)
	req.Header.Set(annotations.OriginSystemIDHeader, annotations.PACOriginSystemID)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)
	resp := w.Result()
	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(resp.Body)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, newHash, resp.Header.Get(annotations.DocumentHashHeader))

	rw.AssertExpectations(t)
	aug.AssertExpectations(t)
	annAPI.AssertExpectations(t)
}

func TestUnHappyAddAnnotationInvalidContentId(t *testing.T) {
	_ = os.Setenv("JSON_SCHEMAS_PATH", "../schemas")
	_ = os.Setenv("JSON_SCHEMAS_API_CONFIG_PATH", "../config/schemas-api-config.json")
	_ = os.Setenv("JSON_SCHEMA_NAME", "draft-annotations-pac-add.json;draft-annotations-pac-replace.json;draft-annotations-pac-write.json;draft-annotations-sv-add.json;draft-annotations-sv-replace.json;draft-annotations-sv-write.json")
	rw := new(RWMock)
	annAPI := new(AnnotationsAPIMock)
	aug := new(AugmenterMock)

	log := logger.NewUPPLogger("draft-annotations-api", "INFO")
	v := validator.NewSchemaValidator(log).GetJSONValidator()
	h := handler.New(rw, annAPI, nil, aug, v, time.Second, log)
	r := mux.NewRouter()
	r.HandleFunc("/draft-annotations/content/{uuid}/annotations", h.AddAnnotation).Methods(http.MethodPost)

	req := httptest.NewRequest(
		http.MethodPost,
		"http://api.ft.com/draft-annotations/content/foo/annotations",
		nil)

	req.Header.Set(tidutils.TransactionIDHeader, testTID)
	req.Header.Set(annotations.OriginSystemIDHeader, annotations.PACOriginSystemID)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)
	resp := w.Result()
	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(resp.Body)

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestUnHappyAddAnnotationInvalidConceptIdPrefix(t *testing.T) {
	_ = os.Setenv("JSON_SCHEMAS_PATH", "../schemas")
	_ = os.Setenv("JSON_SCHEMAS_API_CONFIG_PATH", "../config/schemas-api-config.json")
	_ = os.Setenv("JSON_SCHEMA_NAME", "draft-annotations-pac-add.json;draft-annotations-pac-replace.json;draft-annotations-pac-write.json;draft-annotations-sv-add.json;draft-annotations-sv-replace.json;draft-annotations-sv-write.json")
	rw := new(RWMock)
	annAPI := new(AnnotationsAPIMock)
	aug := new(AugmenterMock)

	log := logger.NewUPPLogger("draft-annotations-api", "INFO")
	v := validator.NewSchemaValidator(log).GetJSONValidator()
	h := handler.New(rw, annAPI, nil, aug, v, time.Second, log)
	r := mux.NewRouter()
	r.HandleFunc("/draft-annotations/content/{uuid}/annotations", h.AddAnnotation).Methods(http.MethodPost)

	ann := map[string]interface{}{
		"annotation": map[string]interface{}{
			"predicate": "http://www.ft.com/ontology/annotation/about",
			"id":        "http://www.ft.com/thing//838b3fbe-efbc-3cfe-b5c0-d38c046492a4",
		},
		"publication": []interface{}{"88fdde6c-2aa4-4f78-af02-9f680097cfd6"},
	}
	b, _ := json.Marshal(ann)

	req := httptest.NewRequest(
		http.MethodPost,
		"http://api.ft.com/draft-annotations/content/83a201c6-60cd-11e7-91a7-502f7ee26895/annotations",
		bytes.NewBuffer(b))

	req.Header.Set(tidutils.TransactionIDHeader, testTID)
	req.Header.Set(annotations.OriginSystemIDHeader, annotations.PACOriginSystemID)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)
	resp := w.Result()
	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(resp.Body)

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestUnHappyAddAnnotationEmptyConceptId(t *testing.T) {
	_ = os.Setenv("JSON_SCHEMAS_PATH", "../schemas")
	_ = os.Setenv("JSON_SCHEMAS_API_CONFIG_PATH", "../config/schemas-api-config.json")
	_ = os.Setenv("JSON_SCHEMA_NAME", "draft-annotations-pac-add.json;draft-annotations-pac-replace.json;draft-annotations-pac-write.json;draft-annotations-sv-add.json;draft-annotations-sv-replace.json;draft-annotations-sv-write.json")
	rw := new(RWMock)
	annAPI := new(AnnotationsAPIMock)
	aug := new(AugmenterMock)

	log := logger.NewUPPLogger("draft-annotations-api", "INFO")
	v := validator.NewSchemaValidator(log).GetJSONValidator()
	h := handler.New(rw, annAPI, nil, aug, v, time.Second, log)
	r := mux.NewRouter()
	r.HandleFunc("/draft-annotations/content/{uuid}/annotations", h.AddAnnotation).Methods(http.MethodPost)

	ann := map[string]interface{}{
		"annotation": map[string]interface{}{
			"predicate": "http://www.ft.com/ontology/annotation/about",
		},
		"publication": []interface{}{"88fdde6c-2aa4-4f78-af02-9f680097cfd6"},
	}
	b, _ := json.Marshal(ann)

	req := httptest.NewRequest(
		http.MethodPost,
		"http://api.ft.com/draft-annotations/content/83a201c6-60cd-11e7-91a7-502f7ee26895/annotations",
		bytes.NewBuffer(b))

	req.Header.Set(tidutils.TransactionIDHeader, testTID)
	req.Header.Set(annotations.OriginSystemIDHeader, annotations.PACOriginSystemID)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)
	resp := w.Result()
	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(resp.Body)

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestUnHappyAddAnnotationInvalidConceptUuid(t *testing.T) {
	_ = os.Setenv("JSON_SCHEMAS_PATH", "../schemas")
	_ = os.Setenv("JSON_SCHEMAS_API_CONFIG_PATH", "../config/schemas-api-config.json")
	_ = os.Setenv("JSON_SCHEMA_NAME", "draft-annotations-pac-add.json;draft-annotations-pac-replace.json;draft-annotations-pac-write.json;draft-annotations-sv-add.json;draft-annotations-sv-replace.json;draft-annotations-sv-write.json")
	rw := new(RWMock)
	annAPI := new(AnnotationsAPIMock)
	aug := new(AugmenterMock)

	log := logger.NewUPPLogger("draft-annotations-api", "INFO")
	v := validator.NewSchemaValidator(log).GetJSONValidator()
	h := handler.New(rw, annAPI, nil, aug, v, time.Second, log)
	r := mux.NewRouter()
	r.HandleFunc("/draft-annotations/content/{uuid}/annotations", h.AddAnnotation).Methods(http.MethodPost)

	ann := map[string]interface{}{
		"annotation": map[string]interface{}{
			"predicate": "http://www.ft.com/ontology/annotation/about",
			"id":        "http://www.ft.com/thing//838b3fbe",
		},
		"publication": []interface{}{"88fdde6c-2aa4-4f78-af02-9f680097cfd6"},
	}
	b, _ := json.Marshal(ann)

	req := httptest.NewRequest(
		http.MethodPost,
		"http://api.ft.com/draft-annotations/content/83a201c6-60cd-11e7-91a7-502f7ee26895/annotations",
		bytes.NewBuffer(b))

	req.Header.Set(tidutils.TransactionIDHeader, testTID)
	req.Header.Set(annotations.OriginSystemIDHeader, annotations.PACOriginSystemID)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)
	resp := w.Result()
	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(resp.Body)

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestUnHappyAddAnnotationInvalidPredicate(t *testing.T) {
	_ = os.Setenv("JSON_SCHEMAS_PATH", "../schemas")
	_ = os.Setenv("JSON_SCHEMAS_API_CONFIG_PATH", "../config/schemas-api-config.json")
	_ = os.Setenv("JSON_SCHEMA_NAME", "draft-annotations-pac-add.json;draft-annotations-pac-replace.json;draft-annotations-pac-write.json;draft-annotations-sv-add.json;draft-annotations-sv-replace.json;draft-annotations-sv-write.json")
	rw := new(RWMock)
	annAPI := new(AnnotationsAPIMock)
	aug := new(AugmenterMock)

	log := logger.NewUPPLogger("draft-annotations-api", "INFO")
	v := validator.NewSchemaValidator(log).GetJSONValidator()
	h := handler.New(rw, annAPI, nil, aug, v, time.Second, log)
	r := mux.NewRouter()
	r.HandleFunc("/draft-annotations/content/{uuid}/annotations", h.AddAnnotation).Methods(http.MethodPost)

	ann := map[string]interface{}{
		"annotation": map[string]interface{}{
			"predicate": "http://www.ft.com/ontology/annotation/foobar",
			"id":        "http://www.ft.com/thing/0a619d71-9af5-3755-90dd-f789b686c67a",
		},
		"publication": []interface{}{"88fdde6c-2aa4-4f78-af02-9f680097cfd6"},
	}
	b, _ := json.Marshal(ann)

	req := httptest.NewRequest(
		http.MethodPost,
		"http://api.ft.com/draft-annotations/content/83a201c6-60cd-11e7-91a7-502f7ee26895/annotations",
		bytes.NewBuffer(b))

	req.Header.Set(tidutils.TransactionIDHeader, testTID)
	req.Header.Set(annotations.OriginSystemIDHeader, annotations.PACOriginSystemID)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)
	resp := w.Result()
	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(resp.Body)

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestUnhappyAddAnnotationWhenWritingAnnotationsFails(t *testing.T) {
	_ = os.Setenv("JSON_SCHEMAS_PATH", "../schemas")
	_ = os.Setenv("JSON_SCHEMAS_API_CONFIG_PATH", "../config/schemas-api-config.json")
	_ = os.Setenv("JSON_SCHEMA_NAME", "draft-annotations-pac-add.json;draft-annotations-pac-replace.json;draft-annotations-pac-write.json;draft-annotations-sv-add.json;draft-annotations-sv-replace.json;draft-annotations-sv-write.json")
	rw := new(RWMock)
	annAPI := new(AnnotationsAPIMock)

	rw.On("Write", mock.Anything, "83a201c6-60cd-11e7-91a7-502f7ee26895", expectedCanonicalisedAnnotationsAfterAdditon, "").Return(mock.Anything, errors.New("error writing annotations"))
	annAPI.On("GetAllButV2", mock.Anything, "83a201c6-60cd-11e7-91a7-502f7ee26895").Return(expectedAnnotations["annotations"], nil)
	canonicalizer := annotations.NewCanonicalizer(annotations.NewCanonicalAnnotationSorter)
	aug := &AugmenterMock{
		augment: func(_ context.Context, depletedAnnotations []interface{}) ([]interface{}, error) {
			depletedAnnotations = canonicalizer.Canonicalize(depletedAnnotations)
			assert.Equal(t, expectedCanonicalisedAnnotationsAfterAdditon["annotations"], depletedAnnotations)
			return augmentedAnnotationsAfterAddition["annotations"].([]interface{}), nil
		},
	}

	log := logger.NewUPPLogger("draft-annotations-api", "INFO")
	v := validator.NewSchemaValidator(log).GetJSONValidator()
	h := handler.New(rw, annAPI, canonicalizer, aug, v, time.Second, log)
	r := mux.NewRouter()

	r.HandleFunc("/draft-annotations/content/{uuid}/annotations", h.AddAnnotation).Methods(http.MethodPost)

	ann := map[string]interface{}{
		"annotation": map[string]interface{}{
			"predicate": "http://www.ft.com/ontology/annotation/mentions",
			"id":        "http://www.ft.com/thing/100e3cc0-aecc-4458-8ebd-6b1fbc7345ed",
		},
		"publication": []interface{}{"88fdde6c-2aa4-4f78-af02-9f680097cfd6"},
	}
	b, _ := json.Marshal(ann)

	req := httptest.NewRequest(
		http.MethodPost,
		"http://api.ft.com/draft-annotations/content/83a201c6-60cd-11e7-91a7-502f7ee26895/annotations",
		bytes.NewBuffer(b))

	req.Header.Set(tidutils.TransactionIDHeader, testTID)
	req.Header.Set(annotations.OriginSystemIDHeader, annotations.PACOriginSystemID)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)
	resp := w.Result()
	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(resp.Body)
	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
}

func TestUnhappyAddAnnotationWhenGettingAnnotationsFails(t *testing.T) {
	_ = os.Setenv("JSON_SCHEMAS_PATH", "../schemas")
	_ = os.Setenv("JSON_SCHEMAS_API_CONFIG_PATH", "../config/schemas-api-config.json")
	_ = os.Setenv("JSON_SCHEMA_NAME", "draft-annotations-pac-add.json;draft-annotations-pac-replace.json;draft-annotations-pac-write.json;draft-annotations-sv-add.json;draft-annotations-sv-replace.json;draft-annotations-sv-write.json")
	rw := new(RWMock)
	annAPI := new(AnnotationsAPIMock)
	aug := new(AugmenterMock)

	rw.On("Write", mock.Anything, "83a201c6-60cd-11e7-91a7-502f7ee26895", expectedCanonicalisedAnnotationsAfterAdditon, "").Return(mock.Anything, nil)
	annAPI.On("GetAllButV2", mock.Anything, "83a201c6-60cd-11e7-91a7-502f7ee26895").Return(expectedAnnotations["annotations"], errors.New("error getting annotations"))

	log := logger.NewUPPLogger("draft-annotations-api", "INFO")
	v := validator.NewSchemaValidator(log).GetJSONValidator()
	h := handler.New(rw, annAPI, annotations.NewCanonicalizer(annotations.NewCanonicalAnnotationSorter), aug, v, time.Second, log)
	r := mux.NewRouter()

	r.HandleFunc("/draft-annotations/content/{uuid}/annotations", h.AddAnnotation).Methods(http.MethodPost)

	ann := map[string]interface{}{
		"annotation": map[string]interface{}{
			"predicate": "http://www.ft.com/ontology/annotation/mentions",
			"id":        "http://www.ft.com/thing/100e3cc0-aecc-4458-8ebd-6b1fbc7345ed",
		},
		"publication": []interface{}{"88fdde6c-2aa4-4f78-af02-9f680097cfd6"},
	}
	b, _ := json.Marshal(ann)

	req := httptest.NewRequest(
		http.MethodPost,
		"http://api.ft.com/draft-annotations/content/83a201c6-60cd-11e7-91a7-502f7ee26895/annotations",
		bytes.NewBuffer(b))

	req.Header.Set(tidutils.TransactionIDHeader, testTID)
	req.Header.Set(annotations.OriginSystemIDHeader, annotations.PACOriginSystemID)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)
	resp := w.Result()
	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(resp.Body)
	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
}

func TestUnhappyAddAnnotationWhenNoAnnotationsFound(t *testing.T) {
	_ = os.Setenv("JSON_SCHEMAS_PATH", "../schemas")
	_ = os.Setenv("JSON_SCHEMAS_API_CONFIG_PATH", "../config/schemas-api-config.json")
	_ = os.Setenv("JSON_SCHEMA_NAME", "draft-annotations-pac-add.json;draft-annotations-pac-replace.json;draft-annotations-pac-write.json;draft-annotations-sv-add.json;draft-annotations-sv-replace.json;draft-annotations-sv-write.json")
	rw := new(RWMock)
	annAPI := new(AnnotationsAPIMock)
	aug := new(AugmenterMock)

	uppErr := annotations.NewUPPError(annotations.UPPNotFoundMsg, http.StatusNotFound, nil)

	rw.On("Write", mock.Anything, "83a201c6-60cd-11e7-91a7-502f7ee26895", expectedCanonicalisedAnnotationsAfterAdditon, "").Return(mock.Anything, nil)
	annAPI.On("GetAllButV2", mock.Anything, "83a201c6-60cd-11e7-91a7-502f7ee26895").Return(expectedAnnotations["annotations"], uppErr)

	log := logger.NewUPPLogger("draft-annotations-api", "INFO")
	v := validator.NewSchemaValidator(log).GetJSONValidator()
	h := handler.New(rw, annAPI, annotations.NewCanonicalizer(annotations.NewCanonicalAnnotationSorter), aug, v, time.Second, log)
	r := mux.NewRouter()

	r.HandleFunc("/draft-annotations/content/{uuid}/annotations", h.AddAnnotation).Methods(http.MethodPost)

	ann := map[string]interface{}{
		"annotation": map[string]interface{}{
			"predicate": "http://www.ft.com/ontology/annotation/mentions",
			"id":        "http://www.ft.com/thing/100e3cc0-aecc-4458-8ebd-6b1fbc7345ed",
		},
		"publication": []interface{}{"88fdde6c-2aa4-4f78-af02-9f680097cfd6"},
	}
	b, _ := json.Marshal(ann)

	req := httptest.NewRequest(
		http.MethodPost,
		"http://api.ft.com/draft-annotations/content/83a201c6-60cd-11e7-91a7-502f7ee26895/annotations",
		bytes.NewBuffer(b))

	req.Header.Set(tidutils.TransactionIDHeader, testTID)
	req.Header.Set(annotations.OriginSystemIDHeader, annotations.PACOriginSystemID)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)
	resp := w.Result()
	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(resp.Body)

	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestHappyReplaceAnnotation(t *testing.T) {
	_ = os.Setenv("JSON_SCHEMAS_PATH", "../schemas")
	_ = os.Setenv("JSON_SCHEMAS_API_CONFIG_PATH", "../config/schemas-api-config.json")
	_ = os.Setenv("JSON_SCHEMA_NAME", "draft-annotations-pac-add.json;draft-annotations-pac-replace.json;draft-annotations-pac-write.json;draft-annotations-sv-add.json;draft-annotations-sv-replace.json;draft-annotations-sv-write.json")
	rw := new(RWMock)
	annAPI := new(AnnotationsAPIMock)

	oldHash := randomdata.RandStringRunes(56)
	newHash := randomdata.RandStringRunes(56)

	rw.On("Write", mock.Anything, "83a201c6-60cd-11e7-91a7-502f7ee26895", expectedCanonicalisedAnnotationsAfterReplace, oldHash).Return(newHash, nil)
	annAPI.On("GetAllButV2", mock.Anything, "83a201c6-60cd-11e7-91a7-502f7ee26895").Return(expectedAnnotations["annotations"], nil)
	canonicalizer := annotations.NewCanonicalizer(annotations.NewCanonicalAnnotationSorter)
	aug := &AugmenterMock{
		augment: func(_ context.Context, depletedAnnotations []interface{}) ([]interface{}, error) {
			depletedAnnotations = canonicalizer.Canonicalize(depletedAnnotations)
			assert.Equal(t, expectedCanonicalisedAnnotationsAfterReplace["annotations"], depletedAnnotations)
			return augmentedAnnotationsAfterReplace["annotations"].([]interface{}), nil
		},
	}

	log := logger.NewUPPLogger("draft-annotations-api", "INFO")
	v := validator.NewSchemaValidator(log).GetJSONValidator()
	h := handler.New(rw, annAPI, canonicalizer, aug, v, time.Second, log)
	r := mux.NewRouter()

	r.HandleFunc("/draft-annotations/content/{uuid}/annotations/{cuuid}", h.ReplaceAnnotation).Methods(http.MethodPatch)

	ann := map[string]interface{}{
		"annotation": map[string]interface{}{
			"id": "http://www.ft.com/thing/100e3cc0-aecc-4458-8ebd-6b1fbc7345ed",
		},
		"publication": []interface{}{"88fdde6c-2aa4-4f78-af02-9f680097cfd6"},
	}
	b, _ := json.Marshal(ann)

	req := httptest.NewRequest(
		http.MethodPatch,
		"http://api.ft.com/draft-annotations/content/83a201c6-60cd-11e7-91a7-502f7ee26895/annotations/9577c6d4-b09e-4552-b88f-e52745abe02b",
		bytes.NewBuffer(b))

	req.Header.Set(tidutils.TransactionIDHeader, testTID)
	req.Header.Set(annotations.PreviousDocumentHashHeader, oldHash)
	req.Header.Set(annotations.OriginSystemIDHeader, annotations.PACOriginSystemID)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)
	resp := w.Result()
	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(resp.Body)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, newHash, resp.Header.Get(annotations.DocumentHashHeader))
}

func TestHappyReplaceAnnotationWithPredicate(t *testing.T) {
	_ = os.Setenv("JSON_SCHEMAS_PATH", "../schemas")
	_ = os.Setenv("JSON_SCHEMAS_API_CONFIG_PATH", "../config/schemas-api-config.json")
	_ = os.Setenv("JSON_SCHEMA_NAME", "draft-annotations-pac-add.json;draft-annotations-pac-replace.json;draft-annotations-pac-write.json;draft-annotations-sv-add.json;draft-annotations-sv-replace.json;draft-annotations-sv-write.json")
	rw := new(RWMock)
	annAPI := new(AnnotationsAPIMock)

	oldHash := randomdata.RandStringRunes(56)
	newHash := randomdata.RandStringRunes(56)

	const contentID = "83a201c6-60cd-11e7-91a7-502f7ee26895"
	fromAnnotationAPI := []interface{}{
		map[string]interface{}{
			"predicate": "http://www.ft.com/ontology/annotation/mentions",
			"id":        "http://www.ft.com/thing/0a619d71-9af5-3755-90dd-f789b686c67a",
			"apiUrl":    "http://api.ft.com/people/0a619d71-9af5-3755-90dd-f789b686c67a",
			"type":      "http://www.ft.com/ontology/person/Person",
			"prefLabel": "Barack H. Obama",
		},
		map[string]interface{}{
			"predicate": "http://www.ft.com/ontology/annotation/about",
			"id":        "http://www.ft.com/thing/9577c6d4-b09e-4552-b88f-e52745abe02b",
			"apiUrl":    "http://api.ft.com/concepts/9577c6d4-b09e-4552-b88f-e52745abe02b",
			"type":      "http://www.ft.com/ontology/Topic",
			"prefLabel": "US interest rates",
		},
	}
	augmentedAfterReplace := []interface{}{
		map[string]interface{}{
			"predicate": "http://www.ft.com/ontology/annotation/mentions",
			"id":        "http://www.ft.com/thing/0a619d71-9af5-3755-90dd-f789b686c67a",
			"apiUrl":    "http://api.ft.com/people/0a619d71-9af5-3755-90dd-f789b686c67a",
			"type":      "http://www.ft.com/ontology/person/Person",
			"prefLabel": "Barack H. Obama",
		},
		map[string]interface{}{
			"predicate": "http://www.ft.com/ontology/hasBrand",
			"id":        "http://www.ft.com/thing/100e3cc0-aecc-4458-8ebd-6b1fbc7345ed",
			"apiUrl":    "http://api.ft.com/concepts/100e3cc0-aecc-4458-8ebd-6b1fbc7345ed",
			"type":      "http://www.ft.com/ontology/product/Brand",
			"prefLabel": "Random Brand",
		},
	}
	afterReplace := []interface{}{
		map[string]interface{}{
			"predicate": "http://www.ft.com/ontology/annotation/mentions",
			"id":        "http://www.ft.com/thing/0a619d71-9af5-3755-90dd-f789b686c67a",
		},
		map[string]interface{}{
			"predicate": "http://www.ft.com/ontology/hasBrand",
			"id":        "http://www.ft.com/thing/100e3cc0-aecc-4458-8ebd-6b1fbc7345ed",
		},
	}

	rw.On("Write", mock.Anything, contentID, map[string]interface{}{"annotations": afterReplace, "publication": []interface{}{"88fdde6c-2aa4-4f78-af02-9f680097cfd6"}}, oldHash).Return(newHash, nil)
	annAPI.On("GetAllButV2", mock.Anything, contentID).Return(fromAnnotationAPI, nil)

	canonicalizer := annotations.NewCanonicalizer(annotations.NewCanonicalAnnotationSorter)
	aug := &AugmenterMock{
		augment: func(_ context.Context, depletedAnnotations []interface{}) ([]interface{}, error) {
			depletedAnnotations = canonicalizer.Canonicalize(depletedAnnotations)
			assert.Equal(t, afterReplace, depletedAnnotations)
			return augmentedAfterReplace, nil
		},
	}

	log := logger.NewUPPLogger("draft-annotations-api", "INFO")
	v := validator.NewSchemaValidator(log).GetJSONValidator()
	h := handler.New(rw, annAPI, canonicalizer, aug, v, time.Second, log)
	r := mux.NewRouter()

	r.HandleFunc("/draft-annotations/content/{uuid}/annotations/{cuuid}", h.ReplaceAnnotation).Methods(http.MethodPatch)

	ann := map[string]interface{}{
		"annotation": map[string]interface{}{
			"id":        "http://www.ft.com/thing/100e3cc0-aecc-4458-8ebd-6b1fbc7345ed",
			"predicate": "http://www.ft.com/ontology/hasBrand",
		},
		"publication": []interface{}{"88fdde6c-2aa4-4f78-af02-9f680097cfd6"},
	}
	b, _ := json.Marshal(ann)

	req := httptest.NewRequest(
		http.MethodPatch,
		"/draft-annotations/content/83a201c6-60cd-11e7-91a7-502f7ee26895/annotations/9577c6d4-b09e-4552-b88f-e52745abe02b",
		bytes.NewBuffer(b))

	req.Header.Set(tidutils.TransactionIDHeader, testTID)
	req.Header.Set(annotations.PreviousDocumentHashHeader, oldHash)
	req.Header.Set(annotations.OriginSystemIDHeader, annotations.PACOriginSystemID)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)
	resp := w.Result()
	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(resp.Body)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, newHash, resp.Header.Get(annotations.DocumentHashHeader))
}

func TestHappyReplaceExistingAnnotation(t *testing.T) {
	_ = os.Setenv("JSON_SCHEMAS_PATH", "../schemas")
	_ = os.Setenv("JSON_SCHEMAS_API_CONFIG_PATH", "../config/schemas-api-config.json")
	_ = os.Setenv("JSON_SCHEMA_NAME", "draft-annotations-pac-add.json;draft-annotations-pac-replace.json;draft-annotations-pac-write.json;draft-annotations-sv-add.json;draft-annotations-sv-replace.json;draft-annotations-sv-write.json")
	rw := new(RWMock)
	annAPI := new(AnnotationsAPIMock)

	oldHash := randomdata.RandStringRunes(56)
	newHash := randomdata.RandStringRunes(56)

	rw.On("Write", mock.Anything, "83a201c6-60cd-11e7-91a7-502f7ee26895", expectedAnnotationsReplaceExisting, oldHash).Return(newHash, nil)
	annAPI.On("GetAllButV2", mock.Anything, "83a201c6-60cd-11e7-91a7-502f7ee26895").Return(expectedAnnotationsReplace["annotations"], nil)
	canonicalizer := annotations.NewCanonicalizer(annotations.NewCanonicalAnnotationSorter)
	aug := &AugmenterMock{
		augment: func(_ context.Context, depletedAnnotations []interface{}) ([]interface{}, error) {
			depletedAnnotations = canonicalizer.Canonicalize(depletedAnnotations)
			assert.Equal(t, expectedAnnotationsReplaceExisting["annotations"], depletedAnnotations)
			return expectedAnnotationsReplace["annotations"].([]interface{}), nil
		},
	}

	log := logger.NewUPPLogger("draft-annotations-api", "INFO")
	v := validator.NewSchemaValidator(log).GetJSONValidator()
	h := handler.New(rw, annAPI, canonicalizer, aug, v, time.Second, log)
	r := mux.NewRouter()
	r.HandleFunc("/draft-annotations/content/{uuid}/annotations/{cuuid}", h.ReplaceAnnotation).Methods(http.MethodPatch)

	ann := map[string]interface{}{
		"annotation": map[string]interface{}{
			"id":        "http://www.ft.com/thing/0a619d71-9af5-3755-90dd-f789b686c67a",
			"predicate": "http://www.ft.com/ontology/annotation/mentions",
		},
		"publication": []interface{}{"88fdde6c-2aa4-4f78-af02-9f680097cfd6"},
	}
	b, _ := json.Marshal(ann)

	req := httptest.NewRequest(
		http.MethodPatch,
		"http://api.ft.com/draft-annotations/content/83a201c6-60cd-11e7-91a7-502f7ee26895/annotations/0a619d71-9af5-3755-90dd-f789b686c67a",
		bytes.NewBuffer(b))

	req.Header.Set(tidutils.TransactionIDHeader, testTID)
	req.Header.Set(annotations.PreviousDocumentHashHeader, oldHash)
	req.Header.Set(annotations.OriginSystemIDHeader, annotations.PACOriginSystemID)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)
	resp := w.Result()
	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(resp.Body)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, newHash, resp.Header.Get(annotations.DocumentHashHeader))

	rw.AssertExpectations(t)
	aug.AssertExpectations(t)
	annAPI.AssertExpectations(t)
}

func TestUnHappyReplaceAnnotationsInvalidContentUUID(t *testing.T) {
	_ = os.Setenv("JSON_SCHEMAS_PATH", "../schemas")
	_ = os.Setenv("JSON_SCHEMAS_API_CONFIG_PATH", "../config/schemas-api-config.json")
	_ = os.Setenv("JSON_SCHEMA_NAME", "draft-annotations-pac-add.json;draft-annotations-pac-replace.json;draft-annotations-pac-write.json;draft-annotations-sv-add.json;draft-annotations-sv-replace.json;draft-annotations-sv-write.json")
	rw := new(RWMock)
	annAPI := new(AnnotationsAPIMock)
	aug := new(AugmenterMock)

	log := logger.NewUPPLogger("draft-annotations-api", "INFO")
	v := validator.NewSchemaValidator(log).GetJSONValidator()
	h := handler.New(rw, annAPI, nil, aug, v, time.Second, log)
	r := mux.NewRouter()
	r.HandleFunc("/draft-annotations/content/{uuid}/annotations/{cuuid}", h.ReplaceAnnotation).Methods(http.MethodPatch)

	req := httptest.NewRequest(
		http.MethodPatch,
		"http://api.ft.com/draft-annotations/content/foo/annotations/eccb0da2-54f3-4f9f-bafa-fcec10e1758c",
		nil)

	req.Header.Set(tidutils.TransactionIDHeader, testTID)
	req.Header.Set(annotations.OriginSystemIDHeader, annotations.PACOriginSystemID)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)
	resp := w.Result()
	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(resp.Body)

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestUnHappyReplaceAnnotationInvalidConceptIdInURI(t *testing.T) {
	_ = os.Setenv("JSON_SCHEMAS_PATH", "../schemas")
	_ = os.Setenv("JSON_SCHEMAS_API_CONFIG_PATH", "../config/schemas-api-config.json")
	_ = os.Setenv("JSON_SCHEMA_NAME", "draft-annotations-pac-add.json;draft-annotations-pac-replace.json;draft-annotations-pac-write.json;draft-annotations-sv-add.json;draft-annotations-sv-replace.json;draft-annotations-sv-write.json")
	rw := new(RWMock)
	annAPI := new(AnnotationsAPIMock)
	aug := new(AugmenterMock)

	log := logger.NewUPPLogger("draft-annotations-api", "INFO")
	v := validator.NewSchemaValidator(log).GetJSONValidator()
	h := handler.New(rw, annAPI, nil, aug, v, time.Second, log)
	r := mux.NewRouter()
	r.HandleFunc("/draft-annotations/content/{uuid}/annotations/{cuuid}", h.ReplaceAnnotation).Methods(http.MethodPatch)

	ann := map[string]interface{}{
		"id": "http://www.ft.com/thing/9577c6d4-b09e-4552-b88f-e52745abe02b",
	}
	b, _ := json.Marshal(ann)

	req := httptest.NewRequest(
		http.MethodPatch,
		"http://api.ft.com/draft-annotations/content/83a201c6-60cd-11e7-91a7-502f7ee26895/annotations/bar",
		bytes.NewBuffer(b))

	req.Header.Set(tidutils.TransactionIDHeader, testTID)
	req.Header.Set(annotations.OriginSystemIDHeader, annotations.PACOriginSystemID)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)
	resp := w.Result()
	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(resp.Body)

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestUnHappyReplaceAnnotationEmptyBody(t *testing.T) {
	_ = os.Setenv("JSON_SCHEMAS_PATH", "../schemas")
	_ = os.Setenv("JSON_SCHEMAS_API_CONFIG_PATH", "../config/schemas-api-config.json")
	_ = os.Setenv("JSON_SCHEMA_NAME", "draft-annotations-pac-add.json;draft-annotations-pac-replace.json;draft-annotations-pac-write.json;draft-annotations-sv-add.json;draft-annotations-sv-replace.json;draft-annotations-sv-write.json")
	rw := new(RWMock)
	annAPI := new(AnnotationsAPIMock)
	aug := new(AugmenterMock)

	log := logger.NewUPPLogger("draft-annotations-api", "INFO")
	v := validator.NewSchemaValidator(log).GetJSONValidator()
	h := handler.New(rw, annAPI, nil, aug, v, time.Second, log)
	r := mux.NewRouter()
	r.HandleFunc("/draft-annotations/content/{uuid}/annotations/{cuuid}", h.ReplaceAnnotation).Methods(http.MethodPatch)

	req := httptest.NewRequest(
		http.MethodPatch,
		"http://api.ft.com/draft-annotations/content/83a201c6-60cd-11e7-91a7-502f7ee26895/annotations/9577c6d4-b09e-4552-b88f-e52745abe02b",
		nil)

	req.Header.Set(tidutils.TransactionIDHeader, testTID)
	req.Header.Set(annotations.OriginSystemIDHeader, annotations.PACOriginSystemID)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)
	resp := w.Result()
	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(resp.Body)

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestUnHappyReplaceAnnotationInvalidConceptIdInBody(t *testing.T) {
	_ = os.Setenv("JSON_SCHEMAS_PATH", "../schemas")
	_ = os.Setenv("JSON_SCHEMAS_API_CONFIG_PATH", "../config/schemas-api-config.json")
	_ = os.Setenv("JSON_SCHEMA_NAME", "draft-annotations-pac-add.json;draft-annotations-pac-replace.json;draft-annotations-pac-write.json;draft-annotations-sv-add.json;draft-annotations-sv-replace.json;draft-annotations-sv-write.json")
	rw := new(RWMock)
	annAPI := new(AnnotationsAPIMock)
	aug := new(AugmenterMock)

	log := logger.NewUPPLogger("draft-annotations-api", "INFO")
	v := validator.NewSchemaValidator(log).GetJSONValidator()
	h := handler.New(rw, annAPI, nil, aug, v, time.Second, log)
	r := mux.NewRouter()
	r.HandleFunc("/draft-annotations/content/{uuid}/annotations/{cuuid}", h.ReplaceAnnotation).Methods(http.MethodPatch)

	ann := map[string]interface{}{
		"annotation": map[string]interface{}{
			"id": "foobar",
		},
	}
	b, _ := json.Marshal(ann)

	req := httptest.NewRequest(
		http.MethodPatch,
		"http://api.ft.com/draft-annotations/content/83a201c6-60cd-11e7-91a7-502f7ee26895/annotations/9577c6d4-b09e-4552-b88f-e52745abe02b",
		bytes.NewBuffer(b))

	req.Header.Set(tidutils.TransactionIDHeader, testTID)
	req.Header.Set(annotations.OriginSystemIDHeader, annotations.PACOriginSystemID)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)
	resp := w.Result()
	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(resp.Body)

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestUnHappyReplaceAnnotationInvalidPredicate(t *testing.T) {
	_ = os.Setenv("JSON_SCHEMAS_PATH", "../schemas")
	_ = os.Setenv("JSON_SCHEMAS_API_CONFIG_PATH", "../config/schemas-api-config.json")
	_ = os.Setenv("JSON_SCHEMA_NAME", "draft-annotations-pac-add.json;draft-annotations-pac-replace.json;draft-annotations-pac-write.json;draft-annotations-sv-add.json;draft-annotations-sv-replace.json;draft-annotations-sv-write.json")
	rw := new(RWMock)
	annAPI := new(AnnotationsAPIMock)
	aug := new(AugmenterMock)

	log := logger.NewUPPLogger("draft-annotations-api", "INFO")
	v := validator.NewSchemaValidator(log).GetJSONValidator()
	h := handler.New(rw, annAPI, nil, aug, v, time.Second, log)
	r := mux.NewRouter()
	r.HandleFunc("/draft-annotations/content/{uuid}/annotations/{cuuid}", h.ReplaceAnnotation).Methods(http.MethodPatch)

	ann := map[string]interface{}{
		"annotation": map[string]interface{}{
			"id":        "http://www.ft.com/thing/9577c6d4-b09e-4552-b88f-e52745abe02b",
			"predicate": "foo",
		},
	}
	b, _ := json.Marshal(ann)

	req := httptest.NewRequest(
		http.MethodPatch,
		"/draft-annotations/content/83a201c6-60cd-11e7-91a7-502f7ee26895/annotations/9577c6d4-b09e-4552-b88f-e52745abe02b",
		bytes.NewBuffer(b))

	req.Header.Set(tidutils.TransactionIDHeader, testTID)
	req.Header.Set(annotations.OriginSystemIDHeader, annotations.PACOriginSystemID)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)
	resp := w.Result()
	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(resp.Body)

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestUnhappyReplaceAnnotationWhenWritingAnnotationsFails(t *testing.T) {
	_ = os.Setenv("JSON_SCHEMAS_PATH", "../schemas")
	_ = os.Setenv("JSON_SCHEMAS_API_CONFIG_PATH", "../config/schemas-api-config.json")
	_ = os.Setenv("JSON_SCHEMA_NAME", "draft-annotations-pac-add.json;draft-annotations-pac-replace.json;draft-annotations-pac-write.json;draft-annotations-sv-add.json;draft-annotations-sv-replace.json;draft-annotations-sv-write.json")
	rw := new(RWMock)
	annAPI := new(AnnotationsAPIMock)

	rw.On("Write", mock.Anything, "83a201c6-60cd-11e7-91a7-502f7ee26895", expectedCanonicalisedAnnotationsAfterReplace, "").Return(mock.Anything, errors.New("error writing annotations"))
	annAPI.On("GetAllButV2", mock.Anything, "83a201c6-60cd-11e7-91a7-502f7ee26895").Return(expectedAnnotations["annotations"], nil)
	canonicalizer := annotations.NewCanonicalizer(annotations.NewCanonicalAnnotationSorter)
	aug := &AugmenterMock{
		augment: func(_ context.Context, depletedAnnotations []interface{}) ([]interface{}, error) {
			depletedAnnotations = canonicalizer.Canonicalize(depletedAnnotations)
			assert.Equal(t, expectedCanonicalisedAnnotationsAfterReplace["annotations"], depletedAnnotations)
			return augmentedAnnotationsAfterReplace["annotations"].([]interface{}), nil
		},
	}

	log := logger.NewUPPLogger("draft-annotations-api", "INFO")
	v := validator.NewSchemaValidator(log).GetJSONValidator()
	h := handler.New(rw, annAPI, canonicalizer, aug, v, time.Second, log)
	r := mux.NewRouter()
	r.HandleFunc("/draft-annotations/content/{uuid}/annotations/{cuuid}", h.ReplaceAnnotation).Methods(http.MethodPatch)

	ann := map[string]interface{}{
		"annotation": map[string]interface{}{
			"id": "http://www.ft.com/thing/100e3cc0-aecc-4458-8ebd-6b1fbc7345ed",
		},
		"publication": []interface{}{"88fdde6c-2aa4-4f78-af02-9f680097cfd6"},
	}
	b, _ := json.Marshal(ann)

	req := httptest.NewRequest(
		http.MethodPatch,
		"http://api.ft.com/draft-annotations/content/83a201c6-60cd-11e7-91a7-502f7ee26895/annotations/9577c6d4-b09e-4552-b88f-e52745abe02b",
		bytes.NewBuffer(b))

	req.Header.Set(tidutils.TransactionIDHeader, testTID)
	req.Header.Set(annotations.OriginSystemIDHeader, annotations.PACOriginSystemID)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)
	resp := w.Result()
	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(resp.Body)
	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
}

func TestUnhappyReplaceAnnotationWhenGettingAnnotationsFails(t *testing.T) {
	_ = os.Setenv("JSON_SCHEMAS_PATH", "../schemas")
	_ = os.Setenv("JSON_SCHEMAS_API_CONFIG_PATH", "../config/schemas-api-config.json")
	_ = os.Setenv("JSON_SCHEMA_NAME", "draft-annotations-pac-add.json;draft-annotations-pac-replace.json;draft-annotations-pac-write.json;draft-annotations-sv-add.json;draft-annotations-sv-replace.json;draft-annotations-sv-write.json")
	rw := new(RWMock)
	annAPI := new(AnnotationsAPIMock)
	aug := new(AugmenterMock)

	rw.On("Write", mock.Anything, "83a201c6-60cd-11e7-91a7-502f7ee26895", expectedCanonicalisedAnnotationsAfterAdditon, "").Return(mock.Anything, nil)
	annAPI.On("GetAllButV2", mock.Anything, "83a201c6-60cd-11e7-91a7-502f7ee26895").Return(expectedAnnotations["annotations"], errors.New("error getting annotations"))

	log := logger.NewUPPLogger("draft-annotations-api", "INFO")
	v := validator.NewSchemaValidator(log).GetJSONValidator()
	h := handler.New(rw, annAPI, annotations.NewCanonicalizer(annotations.NewCanonicalAnnotationSorter), aug, v, time.Second, log)
	r := mux.NewRouter()
	r.HandleFunc("/draft-annotations/content/{uuid}/annotations/{cuuid}", h.ReplaceAnnotation).Methods(http.MethodPatch)

	ann := map[string]interface{}{
		"annotation": map[string]interface{}{
			"id":        "http://www.ft.com/thing/100e3cc0-aecc-4458-8ebd-6b1fbc7345ed",
			"predicate": "http://www.ft.com/ontology/annotation/about",
		},
		"publication": []interface{}{"88fdde6c-2aa4-4f78-af02-9f680097cfd6"},
	}
	b, _ := json.Marshal(ann)

	req := httptest.NewRequest(
		http.MethodPatch,
		"http://api.ft.com/draft-annotations/content/83a201c6-60cd-11e7-91a7-502f7ee26895/annotations/9577c6d4-b09e-4552-b88f-e52745abe02b",
		bytes.NewBuffer(b))

	req.Header.Set(tidutils.TransactionIDHeader, testTID)
	req.Header.Set(annotations.OriginSystemIDHeader, annotations.PACOriginSystemID)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)
	resp := w.Result()
	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(resp.Body)
	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
}

func TestUnhappyReplaceAnnotationWhenNoAnnotationsFound(t *testing.T) {
	_ = os.Setenv("JSON_SCHEMAS_PATH", "../schemas")
	_ = os.Setenv("JSON_SCHEMAS_API_CONFIG_PATH", "../config/schemas-api-config.json")
	_ = os.Setenv("JSON_SCHEMA_NAME", "draft-annotations-pac-add.json;draft-annotations-pac-replace.json;draft-annotations-pac-write.json;draft-annotations-sv-add.json;draft-annotations-sv-replace.json;draft-annotations-sv-write.json")
	rw := new(RWMock)
	annAPI := new(AnnotationsAPIMock)
	aug := new(AugmenterMock)

	uppErr := annotations.NewUPPError(annotations.UPPNotFoundMsg, http.StatusNotFound, nil)

	rw.On("Write", mock.Anything, "83a201c6-60cd-11e7-91a7-502f7ee26895", expectedCanonicalisedAnnotationsAfterAdditon, "").Return(mock.Anything, nil)
	annAPI.On("GetAllButV2", mock.Anything, "83a201c6-60cd-11e7-91a7-502f7ee26895").Return(expectedAnnotations["annotations"], uppErr)

	log := logger.NewUPPLogger("draft-annotations-api", "INFO")
	v := validator.NewSchemaValidator(log).GetJSONValidator()
	h := handler.New(rw, annAPI, annotations.NewCanonicalizer(annotations.NewCanonicalAnnotationSorter), aug, v, time.Second, log)
	r := mux.NewRouter()
	r.HandleFunc("/draft-annotations/content/{uuid}/annotations/{cuuid}", h.ReplaceAnnotation).Methods(http.MethodPatch)

	ann := map[string]interface{}{
		"annotation": map[string]interface{}{
			"id":        "http://www.ft.com/thing/100e3cc0-aecc-4458-8ebd-6b1fbc7345ed",
			"predicate": "http://www.ft.com/ontology/annotation/about",
		},
		"publication": []interface{}{"88fdde6c-2aa4-4f78-af02-9f680097cfd6"},
	}
	b, _ := json.Marshal(ann)

	req := httptest.NewRequest(
		http.MethodPatch,
		"/draft-annotations/content/83a201c6-60cd-11e7-91a7-502f7ee26895/annotations/9577c6d4-b09e-4552-b88f-e52745abe02b",
		bytes.NewBuffer(b))

	req.Header.Set(tidutils.TransactionIDHeader, testTID)
	req.Header.Set(annotations.OriginSystemIDHeader, annotations.PACOriginSystemID)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)
	resp := w.Result()
	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(resp.Body)

	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestValidate(t *testing.T) {
	_ = os.Setenv("JSON_SCHEMAS_PATH", "../schemas")
	_ = os.Setenv("JSON_SCHEMAS_API_CONFIG_PATH", "../config/schemas-api-config.json")
	_ = os.Setenv("JSON_SCHEMA_NAME", "draft-annotations-pac-add.json;draft-annotations-pac-replace.json;draft-annotations-pac-write.json;draft-annotations-sv-add.json;draft-annotations-sv-replace.json;draft-annotations-sv-write.json")

	tests := []struct {
		name               string
		requestBody        map[string]interface{}
		header             string
		expectedStatusCode int
	}{
		{
			"Valid PAC annotations write request",
			map[string]interface{}{
				"annotations": []interface{}{
					map[string]interface{}{
						"predicate": "http://www.ft.com/ontology/annotation/mentions",
						"id":        "http://www.ft.com/thing/0a619d71-9af5-3755-90dd-f789b686c67a",
					},
				},
				"publication": []string{"88fdde6c-2aa4-4f78-af02-9f680097cfd6"},
			},
			"draft-annotations-pac-write.json",
			200,
		},
		{
			"Valid SV annotations write request",
			map[string]interface{}{
				"annotations": []interface{}{
					map[string]interface{}{
						"predicate": "http://www.ft.com/ontology/annotation/about",
						"id":        "http://www.ft.com/thing/0a619d71-9af5-3755-90dd-f789b686c67a",
					},
				},
				"publication": []string{"88fdde6c-2aa4-4f78-af02-9f680097cfd6"},
			},
			"draft-annotations-sv-write.json",
			200,
		},
		{
			"PAC annotations write request with missing publication array",
			map[string]interface{}{
				"annotations": []interface{}{
					map[string]interface{}{
						"predicate": "http://www.ft.com/ontology/annotation/mentions",
						"id":        "http://www.ft.com/thing/0a619d71-9af5-3755-90dd-f789b686c67a",
					},
				},
			},
			"draft-annotations-pac-write.json",
			400,
		},
		{
			"SV annotations write request with missing publication array",
			map[string]interface{}{
				"annotations": []interface{}{
					map[string]interface{}{
						"predicate": "http://www.ft.com/ontology/annotation/about",
						"id":        "http://www.ft.com/thing/0a619d71-9af5-3755-90dd-f789b686c67a",
					},
				},
			},
			"draft-annotations-sv-write.json",
			400,
		},
		{
			"Valid PAC annotations add request",
			map[string]interface{}{
				"annotation": map[string]interface{}{
					"predicate": "http://www.ft.com/ontology/annotation/mentions",
					"id":        "http://www.ft.com/thing/0a619d71-9af5-3755-90dd-f789b686c67a",
				},
				"publication": []string{"88fdde6c-2aa4-4f78-af02-9f680097cfd6"},
			},
			"draft-annotations-pac-add.json",
			200,
		},
		{
			"Valid SV annotations add request",
			map[string]interface{}{
				"annotation": map[string]interface{}{
					"predicate": "http://www.ft.com/ontology/annotation/about",
					"id":        "http://www.ft.com/thing/0a619d71-9af5-3755-90dd-f789b686c67a",
				},
				"publication": []string{"88fdde6c-2aa4-4f78-af02-9f680097cfd6"},
			},
			"draft-annotations-sv-add.json",
			200,
		},
		{
			"PAC annotations add request with missing publication",
			map[string]interface{}{
				"annotation": map[string]interface{}{
					"predicate": "http://www.ft.com/ontology/annotation/mentions",
					"id":        "http://www.ft.com/thing/0a619d71-9af5-3755-90dd-f789b686c67a",
				},
			},
			"draft-annotations-pac-add.json",
			400,
		},
		{
			"SV annotations add request with missing publication",
			map[string]interface{}{
				"annotation": map[string]interface{}{
					"predicate": "http://www.ft.com/ontology/annotation/about",
					"id":        "http://www.ft.com/thing/0a619d71-9af5-3755-90dd-f789b686c67a",
				},
			},
			"draft-annotations-sv-add.json",
			400,
		},
	}

	for _, tt := range tests {
		rw := new(RWMock)
		annAPI := new(AnnotationsAPIMock)
		aug := new(AugmenterMock)

		log := logger.NewUPPLogger("draft-annotations-api", "INFO")
		v := validator.NewSchemaValidator(log).GetJSONValidator()
		h := handler.New(rw, annAPI, annotations.NewCanonicalizer(annotations.NewCanonicalAnnotationSorter), aug, v, time.Second, log)

		r := mux.NewRouter()
		r.HandleFunc("/draft-annotations/validate", h.Validate).Methods(http.MethodPost)

		b, err := json.Marshal(tt.requestBody)
		require.NoError(t, err)

		req := httptest.NewRequest(
			http.MethodPost,
			"/draft-annotations/validate",
			bytes.NewBuffer(b))
		req.Header.Set(handler.SchemaNameHeader, tt.header)

		w := httptest.NewRecorder()

		r.ServeHTTP(w, req)
		resp := w.Result()
		defer func(Body io.ReadCloser) {
			_ = Body.Close()
		}(resp.Body)

		assert.Equal(t, tt.expectedStatusCode, resp.StatusCode)
	}
}

func TestListSchemas(t *testing.T) {
	_ = os.Setenv("JSON_SCHEMAS_PATH", "../schemas")
	_ = os.Setenv("JSON_SCHEMAS_API_CONFIG_PATH", "../config/schemas-api-config.json")
	_ = os.Setenv("JSON_SCHEMA_NAME", "draft-annotations-pac-add.json;draft-annotations-pac-replace.json;draft-annotations-pac-write.json;draft-annotations-sv-add.json;draft-annotations-sv-replace.json;draft-annotations-sv-write.json")

	tests := []struct {
		name            string
		expectedMessage string
	}{
		{
			"List schemas",
			`{"_links":{"application/vnd.ft-upp-annotations-pac-add.json":[{"href":"/draft-annotations/schemas/draft-annotations-pac-add.json","name":"latest-version"}],"application/vnd.ft-upp-annotations-pac-replace.json":[{"href":"/draft-annotations/schemas/draft-annotations-pac-replace.json","name":"latest-version"}],"application/vnd.ft-upp-annotations-pac-write.json":[{"href":"/draft-annotations/schemas/draft-annotations-pac-write.json","name":"latest-version"}],"application/vnd.ft-upp-annotations-sv-add.json":[{"href":"/draft-annotations/schemas/draft-annotations-sv-add.json","name":"latest-version"}],"application/vnd.ft-upp-annotations-sv-replace.json":[{"href":"/draft-annotations/schemas/draft-annotations-sv-replace.json","name":"latest-version"}],"application/vnd.ft-upp-annotations-sv-write.json":[{"href":"/draft-annotations/schemas/draft-annotations-sv-write.json","name":"latest-version"}],"self":{"href":"/draft-annotations/schemas"}}}`,
		},
	}

	for _, tt := range tests {
		log := logger.NewUPPLogger("draft-annotations-api", "INFO")
		s := validator.NewSchemaValidator(log).GetSchemaHandler()

		r := mux.NewRouter()
		r.HandleFunc("/draft-annotations/schemas", s.ListSchemas).Methods(http.MethodGet)

		req := httptest.NewRequest(
			http.MethodGet,
			"/draft-annotations/schemas",
			nil)

		w := httptest.NewRecorder()

		r.ServeHTTP(w, req)
		resp := w.Result()
		defer func(Body io.ReadCloser) {
			_ = Body.Close()
		}(resp.Body)

		assert.Equal(t, tt.expectedMessage, strings.TrimSpace(w.Body.String()))
	}
}

func TestGetSchemas(t *testing.T) {
	_ = os.Setenv("JSON_SCHEMAS_PATH", "../schemas")
	_ = os.Setenv("JSON_SCHEMAS_API_CONFIG_PATH", "../config/schemas-api-config.json")
	_ = os.Setenv("JSON_SCHEMA_NAME", "draft-annotations-pac-add.json;draft-annotations-pac-replace.json;draft-annotations-pac-write.json;draft-annotations-sv-add.json;draft-annotations-sv-replace.json;draft-annotations-sv-write.json")

	tests := []struct {
		name            string
		schemaName      string
		expectedMessage string
	}{
		{
			"Get Draft PAC Annotations Write Schema",
			"draft-annotations-pac-write.json",
			`{"$schema":"https://json-schema.org/draft/2020-12/schema","$id":"http://upp-publishing-prod.ft.com/schema/draft-annotations-pac-write+json","title":"Draft PAC Annotations Write Endpoint","type":"object","description":"Schema for Draft PAC Annotations","properties":{"annotations":{"type":"array","description":"Draft PAC annotations","items":{"$ref":"#/$defs/annotation"}},"publication":{"type":"array","description":"Indicates which titles are aware of this content","items":{"type":"string","pattern":"[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}"}}},"required":["annotations","publication"],"additionalProperties":false,"$defs":{"annotation":{"type":"object","properties":{"id":{"type":"string","pattern":".*/([0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12})$","description":"ID of the related concept"},"predicate":{"type":"string","description":"Predicate of the annotation","enum":["http://www.ft.com/ontology/annotation/mentions","http://www.ft.com/ontology/classification/isClassifiedBy","http://www.ft.com/ontology/implicitlyClassifiedBy","http://www.ft.com/ontology/annotation/about","http://www.ft.com/ontology/isPrimarilyClassifiedBy","http://www.ft.com/ontology/majorMentions","http://www.ft.com/ontology/annotation/hasAuthor","http://www.ft.com/ontology/hasContributor","http://www.ft.com/ontology/hasDisplayTag","http://www.ft.com/ontology/hasBrand"]},"apiUrl":{"type":"string","description":"API URL of the related concept"},"type":{"type":"string","description":"Type of the related concept"},"prefLabel":{"type":"string","description":"PrefLabel of the related concept"},"isFTAuthor":{"type":"boolean","description":"Indicates whether the related concept is an FT author"}},"required":["id","predicate"],"additionalProperties":false}}}`,
		},
		{
			"Get Draft SV Annotations Add Schema",
			"draft-annotations-sv-add.json",
			`{"$schema":"https://json-schema.org/draft/2020-12/schema","$id":"http://upp-publishing-prod.ft.com/schema/draft-annotations-sv-add+json","title":"Draft Sustainable Views Annotations Add Endpoint","type":"object","description":"Schema for Draft Sustainable Views Annotations","properties":{"annotation":{"type":"object","properties":{"id":{"type":"string","pattern":".*/([0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12})$","description":"ID of the related concept"},"predicate":{"type":"string","description":"Predicate of the annotation","enum":["http://www.ft.com/ontology/annotation/about","http://www.ft.com/ontology/annotation/hasAuthor","http://www.ft.com/ontology/annotation/hasReference"]},"apiUrl":{"type":"string","description":"API URL of the related concept"},"type":{"type":"string","description":"Type of the related concept"},"prefLabel":{"type":"string","description":"PrefLabel of the related concept"},"isFTAuthor":{"type":"boolean","description":"Indicates whether the related concept is an FT author"}},"required":["id","predicate"],"additionalProperties":false},"publication":{"type":"array","description":"Indicates which titles are aware of this content","items":{"type":"string","pattern":"[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}"}}},"required":["annotation","publication"],"additionalProperties":false}`,
		},
		{
			"Get Draft SV Annotations Replace Schema",
			"draft-annotations-sv-replace.json",
			`{"$schema":"https://json-schema.org/draft/2020-12/schema","$id":"http://upp-publishing-prod.ft.com/schema/draft-annotations-sv-replace+json","title":"Draft Sustainable Views Annotations Replace Endpoint","type":"object","description":"Schema for Draft Sustainable Views Annotations","properties":{"annotation":{"type":"object","properties":{"id":{"type":"string","pattern":".*/([0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12})$","description":"ID of the related concept"},"predicate":{"type":"string","description":"Predicate of the annotation","enum":["http://www.ft.com/ontology/annotation/about","http://www.ft.com/ontology/annotation/hasAuthor","http://www.ft.com/ontology/annotation/hasReference"]},"apiUrl":{"type":"string","description":"API URL of the related concept"},"type":{"type":"string","description":"Type of the related concept"},"prefLabel":{"type":"string","description":"PrefLabel of the related concept"},"isFTAuthor":{"type":"boolean","description":"Indicates whether the related concept is an FT author"}},"required":["id"],"additionalProperties":false},"publication":{"type":"array","description":"Indicates which titles are aware of this content","items":{"type":"string","pattern":"[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}"}}},"required":["annotation","publication"],"additionalProperties":false}`,
		},
	}

	for _, tt := range tests {
		log := logger.NewUPPLogger("draft-annotations-api", "INFO")
		s := validator.NewSchemaValidator(log).GetSchemaHandler()

		r := mux.NewRouter()
		r.HandleFunc("/draft-annotations/schemas/{schemaName}", s.GetSchema).Methods(http.MethodGet)

		req := httptest.NewRequest(
			http.MethodGet,
			"/draft-annotations/schemas/"+tt.schemaName,
			nil)

		w := httptest.NewRecorder()

		r.ServeHTTP(w, req)
		resp := w.Result()
		defer func(Body io.ReadCloser) {
			_ = Body.Close()
		}(resp.Body)

		body := &bytes.Buffer{}
		err := json.Compact(body, w.Body.Bytes())
		require.NoError(t, err)

		assert.Equal(t, tt.expectedMessage, strings.TrimSpace(body.String()))
	}
}

type AugmenterMock struct {
	mock.Mock
	augment func(ctx context.Context, depletedAnnotations []interface{}) ([]interface{}, error)
}

func (m *AugmenterMock) AugmentAnnotations(ctx context.Context, depletedAnnotations []interface{}) ([]interface{}, error) {
	if m.augment != nil {
		return m.augment(ctx, depletedAnnotations)
	}
	args := m.Called(ctx, depletedAnnotations)
	return args.Get(0).([]interface{}), args.Error(1)
}

type RWMock struct {
	mock.Mock
	read     func(ctx context.Context, contentUUID string) (map[string]interface{}, string, bool, error)
	write    func(ctx context.Context, contentUUID string, a map[string]interface{}, hash string) (string, error)
	endpoint func() string
	gtg      func() error
}

func (m *RWMock) Read(ctx context.Context, contentUUID string) (map[string]interface{}, string, bool, error) {
	if m.read != nil {
		return m.read(ctx, contentUUID)
	}

	args := m.Called(ctx, contentUUID)

	var ann map[string]interface{}
	if v := args.Get(0); v != nil {
		ann = v.(map[string]interface{})
	}

	return ann, args.String(1), args.Bool(2), args.Error(3)
}

func (m *RWMock) Write(ctx context.Context, contentUUID string, a map[string]interface{}, hash string) (string, error) {
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
	getAll      func(ctx context.Context, contentUUID string) ([]interface{}, error)
	getAllButV2 func(ctx context.Context, contentUUID string) ([]interface{}, error)
	endpoint    func() string
	gtg         func() error
}

func (m *AnnotationsAPIMock) GetAll(ctx context.Context, contentUUID string) ([]interface{}, error) {
	if m.getAll != nil {
		return m.getAll(ctx, contentUUID)
	}
	args := m.Called(ctx, contentUUID)
	return args.Get(0).([]interface{}), args.Error(1)
}

func (m *AnnotationsAPIMock) GetAllButV2(ctx context.Context, contentUUID string) ([]interface{}, error) {
	if m.getAllButV2 != nil {
		return m.getAllButV2(ctx, contentUUID)
	}
	args := m.Called(ctx, contentUUID)
	return args.Get(0).([]interface{}), args.Error(1)
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

func TestShowResponseBasedOnPolicy(t *testing.T) {
	type args struct {
		r          *http.Request
		result     map[string]interface{}
		policyType string
	}
	tests := []struct {
		name     string
		args     args
		expected bool
	}{
		{
			name: "no policies",
			args: args{
				r:          &http.Request{Header: http.Header{}},
				result:     expectedAnnotationsRead,
				policyType: "READ",
			},
			expected: true,
		},
		{
			name: "missing x-policy",
			args: args{
				r: &http.Request{Header: http.Header{
					"Access-From": []string{"API Gateway"},
				}},
				result:     expectedAnnotationsRead,
				policyType: "READ",
			},
			expected: false,
		},
		{
			name: "x-policy is empty string but did not match ft pink",
			args: args{
				r: &http.Request{Header: http.Header{
					"Access-From": []string{"API Gateway"},
					"X-Policy":    []string{""},
				}},
				result:     expectedAnnotationsRead,
				policyType: "READ",
			},
			expected: false,
		},
		{
			name: "x-policy is empty string allow ft pink",
			args: args{
				r: &http.Request{Header: http.Header{
					"Access-From": []string{"API Gateway"},
					"X-Policy":    []string{""},
				}},
				result:     expectedAnnotations,
				policyType: "READ",
			},
			expected: true,
		},
		{
			name: "x-policy internal_unstable allow ft pink",
			args: args{
				r: &http.Request{Header: http.Header{
					"Access-From": []string{"API Gateway"},
					"X-Policy":    []string{"INTERNAL_UNSTABLE"},
				}},
				result:     expectedAnnotations,
				policyType: "READ",
			},
			expected: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := handler.ShowResponseBasedOnPolicy(tt.args.r, tt.args.result, tt.args.policyType)
			assert.Equal(t, tt.expected, actual)
		})
	}
}
