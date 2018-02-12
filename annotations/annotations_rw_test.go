package annotations

import (
	"context"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"

	tidUtils "github.com/Financial-Times/transactionid-utils-go"
	"github.com/Pallinder/go-randomdata"
	log "github.com/sirupsen/logrus"
	logTest "github.com/sirupsen/logrus/hooks/test"
	"github.com/stretchr/testify/assert"
)

const testContentUUID = "db4daee0-2b84-465a-addb-fc8938a608db"

const testRWBody = `{
    "annotations":[
    	{
		    "predicate": "http://www.ft.com/ontology/annotation/mentions",
	    	"id": "http://www.ft.com/thing/0a619d71-9af5-3755-90dd-f789b686c67a"
    	},
	    {
	    	"predicate": "http://www.ft.com/ontology/annotation/hasAuthor",
    		"id": "http://www.ft.com/thing/838b3fbe-efbc-3cfe-b5c0-d38c046492a4"
	    }
    ]
}`

var expectedCanonicalizedAnnotations = Annotations{
	Annotations: []Annotation{
		{
			Predicate: "http://www.ft.com/ontology/annotation/mentions",
			ConceptId: "http://www.ft.com/thing/0a619d71-9af5-3755-90dd-f789b686c67a",
		},
		{
			Predicate: "http://www.ft.com/ontology/annotation/hasAuthor",
			ConceptId: "http://www.ft.com/thing/838b3fbe-efbc-3cfe-b5c0-d38c046492a4",
		},
	},
}

func TestHappyRead(t *testing.T) {
	tid := tidUtils.NewTransactionID()
	expectedHash := randomdata.RandStringRunes(56)
	s := newAnnotationsRWServerMock(t, http.MethodGet, http.StatusOK, testRWBody, "", expectedHash, tid)
	defer s.Close()

	rw := NewRW(s.URL)
	ctx := tidUtils.TransactionAwareContext(context.Background(), tid)
	actualAnnotations, actualHash, found, err := rw.Read(ctx, testContentUUID)
	assert.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, expectedCanonicalizedAnnotations, *actualAnnotations)
	assert.Equal(t, expectedHash, actualHash)
}

func TestReadAnnotationsNotFound(t *testing.T) {
	tid := tidUtils.NewTransactionID()
	s := newAnnotationsRWServerMock(t, http.MethodGet, http.StatusNotFound, "", "", "", tid)
	defer s.Close()

	rw := NewRW(s.URL)
	ctx := tidUtils.TransactionAwareContext(context.Background(), tid)
	_, _, found, err := rw.Read(ctx, testContentUUID)
	assert.NoError(t, err)
	assert.False(t, found)
}

func TestRead500Error(t *testing.T) {
	tid := tidUtils.NewTransactionID()
	s := newAnnotationsRWServerMock(t, http.MethodGet, http.StatusInternalServerError, "", "", "", tid)
	defer s.Close()

	rw := NewRW(s.URL)
	ctx := tidUtils.TransactionAwareContext(context.Background(), tid)
	_, _, found, err := rw.Read(ctx, testContentUUID)
	assert.EqualError(t, err, "annotations RW returned an unexpected HTTP status code in read operation: 500")
	assert.False(t, found)
}

func TestReadHTTPRequestError(t *testing.T) {
	tid := tidUtils.NewTransactionID()

	rw := NewRW(":#")
	ctx := tidUtils.TransactionAwareContext(context.Background(), tid)
	_, _, found, err := rw.Read(ctx, testContentUUID)
	assert.EqualError(t, err, "parse :: missing protocol scheme")
	assert.False(t, found)
}

func TestReadHTTPCallError(t *testing.T) {
	tid := tidUtils.NewTransactionID()

	rw := NewRW("")
	ctx := tidUtils.TransactionAwareContext(context.Background(), tid)
	_, _, found, err := rw.Read(ctx, testContentUUID)
	assert.EqualError(t, err, "Get /drafts/content/db4daee0-2b84-465a-addb-fc8938a608db/annotations: unsupported protocol scheme \"\"")
	assert.False(t, found)
}

func TestReadInvalidBodyError(t *testing.T) {
	tid := tidUtils.NewTransactionID()
	s := newAnnotationsRWServerMock(t, http.MethodGet, http.StatusOK, "{invalid-body}", "", "", tid)
	defer s.Close()

	rw := NewRW(s.URL)
	ctx := tidUtils.TransactionAwareContext(context.Background(), tid)
	_, _, found, err := rw.Read(ctx, testContentUUID)
	assert.EqualError(t, err, "invalid character 'i' looking for beginning of object key string")
	assert.False(t, found)
}

func TestReadMissingTID(t *testing.T) {
	hook := logTest.NewGlobal()

	rw := NewRW("")
	rw.Read(context.Background(), testContentUUID)
	var tid string
	for i, e := range hook.AllEntries() {
		if i == 0 {
			assert.Equal(t, log.WarnLevel, e.Level)
			assert.Equal(t, "Transaction ID error in getting annotations from RW with concept data: Generated a new transaction ID", e.Message)
			tid = e.Data[tidUtils.TransactionIDKey].(string)
			assert.NotEmpty(t, tid)
		} else {
			assert.Equal(t, tid, e.Data[tidUtils.TransactionIDKey])
		}
	}
}

func TestHappyWriteStatusCreate(t *testing.T) {
	tid := tidUtils.NewTransactionID()
	oldHash := randomdata.RandStringRunes(56)
	newHash := randomdata.RandStringRunes(56)
	s := newAnnotationsRWServerMock(t, http.MethodPut, http.StatusCreated, testRWBody, oldHash, newHash, tid)
	defer s.Close()

	rw := NewRW(s.URL)
	ctx := tidUtils.TransactionAwareContext(context.Background(), tid)
	actualNewHash, err := rw.Write(ctx, testContentUUID, &expectedCanonicalizedAnnotations, oldHash)
	assert.NoError(t, err)
	assert.Equal(t, newHash, actualNewHash)
}

func TestHappyWriteStatusOK(t *testing.T) {
	tid := tidUtils.NewTransactionID()
	oldHash := randomdata.RandStringRunes(56)
	newHash := randomdata.RandStringRunes(56)
	s := newAnnotationsRWServerMock(t, http.MethodPut, http.StatusOK, testRWBody, oldHash, newHash, tid)
	defer s.Close()

	rw := NewRW(s.URL)
	ctx := tidUtils.TransactionAwareContext(context.Background(), tid)
	actualNewHash, err := rw.Write(ctx, testContentUUID, &expectedCanonicalizedAnnotations, oldHash)
	assert.NoError(t, err)
	assert.Equal(t, newHash, actualNewHash)
}

func TestUnhappyWriteStatus500(t *testing.T) {
	tid := tidUtils.NewTransactionID()
	oldHash := randomdata.RandStringRunes(56)
	s := newAnnotationsRWServerMock(t, http.MethodPut, http.StatusInternalServerError, testRWBody, oldHash, "", tid)
	defer s.Close()

	rw := NewRW(s.URL)
	ctx := tidUtils.TransactionAwareContext(context.Background(), tid)
	_, err := rw.Write(ctx, testContentUUID, &expectedCanonicalizedAnnotations, oldHash)
	assert.EqualError(t, err, "annotations RW returned an unexpected HTTP status code in write operation: 500")
}

func TestWriteHTTPRequestError(t *testing.T) {
	tid := tidUtils.NewTransactionID()
	oldHash := randomdata.RandStringRunes(56)
	rw := NewRW(":#")
	ctx := tidUtils.TransactionAwareContext(context.Background(), tid)
	_, err := rw.Write(ctx, testContentUUID, &expectedCanonicalizedAnnotations, oldHash)
	assert.EqualError(t, err, "parse :: missing protocol scheme")
}

func TestWriteHTTPCallError(t *testing.T) {
	tid := tidUtils.NewTransactionID()
	oldHash := randomdata.RandStringRunes(56)
	rw := NewRW("")
	ctx := tidUtils.TransactionAwareContext(context.Background(), tid)
	_, err := rw.Write(ctx, testContentUUID, &expectedCanonicalizedAnnotations, oldHash)
	assert.EqualError(t, err, "Put /drafts/content/db4daee0-2b84-465a-addb-fc8938a608db/annotations: unsupported protocol scheme \"\"")
}

func TestWriteMissingTID(t *testing.T) {
	hook := logTest.NewGlobal()
	oldHash := randomdata.RandStringRunes(56)
	rw := NewRW("")
	rw.Write(context.Background(), testContentUUID, &expectedCanonicalizedAnnotations, oldHash)
	var tid string
	for i, e := range hook.AllEntries() {
		if i == 0 {
			assert.Equal(t, log.WarnLevel, e.Level)
			assert.Equal(t, "Transaction ID error in writing annotations to RW with concept data: Generated a new transaction ID", e.Message)
			tid = e.Data[tidUtils.TransactionIDKey].(string)
			assert.NotEmpty(t, tid)
		} else {
			assert.Equal(t, tid, e.Data[tidUtils.TransactionIDKey])
		}
	}
}

func newAnnotationsRWServerMock(t *testing.T, method string, status int, body string, hashIn string, hashOut string, tid string) *httptest.Server {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, method, r.Method)
		assert.Equal(t, "/drafts/content/"+testContentUUID+"/annotations", r.URL.Path)
		if tid == "" {
			assert.NotEmpty(t, r.Header.Get(tidUtils.TransactionIDHeader))
		} else {
			assert.Equal(t, tid, r.Header.Get(tidUtils.TransactionIDHeader))
		}

		assert.Equal(t, "PAC draft-annotations-api", r.Header.Get("User-Agent"))

		w.Header().Set(DocumentHashHeader, hashOut)
		w.WriteHeader(status)

		switch r.Method {
		case http.MethodGet:
			w.Write([]byte(body))
		case http.MethodPut:
			assert.Equal(t, hashIn, r.Header.Get(PreviousDocumentHashHeader))
			rBody, _ := ioutil.ReadAll(r.Body)
			assert.JSONEq(t, body, string(rBody))
		}
	}))
	return ts
}

func TestRWHappyGTG(t *testing.T) {
	s := newAnnotationsRWGTGServerMock(t, http.StatusOK, "")
	defer s.Close()
	rw := NewRW(s.URL)
	err := rw.GTG()
	assert.NoError(t, err)
}

func TestRWHTTPRequestErrorGTG(t *testing.T) {
	rw := NewRW(":#")
	err := rw.GTG()
	assert.EqualError(t, err, "gtg HTTP request error: parse :: missing protocol scheme")
}

func TestRWHTTPCallErrorGTG(t *testing.T) {
	rw := NewRW("")
	err := rw.GTG()
	assert.EqualError(t, err, "gtg HTTP call error: Get /__gtg: unsupported protocol scheme \"\"")
}

func TestRW503GTG(t *testing.T) {
	s := newAnnotationsRWGTGServerMock(t, http.StatusServiceUnavailable, "service unavailable")
	defer s.Close()
	rw := NewRW(s.URL)
	err := rw.GTG()
	assert.EqualError(t, err, "gtg returned unexpected status 503: service unavailable")
}

func newAnnotationsRWGTGServerMock(t *testing.T, status int, body string) *httptest.Server {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "GET", r.Method)
		assert.Equal(t, "/__gtg", r.URL.Path)
		w.WriteHeader(status)
		w.Write([]byte(body))
	}))
	return ts
}

func TestRWEndpoint(t *testing.T) {
	testEndpoint := "http://an-endpoint.com:8080"
	rw := NewRW(testEndpoint)
	assert.Equal(t, testEndpoint, rw.Endpoint())
}
