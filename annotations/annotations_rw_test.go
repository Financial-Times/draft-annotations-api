package annotations

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	tidUtils "github.com/Financial-Times/transactionid-utils-go"
	log "github.com/sirupsen/logrus"
	logTest "github.com/sirupsen/logrus/hooks/test"
	"github.com/stretchr/testify/assert"
)

const testContentUUID = "db4daee0-2b84-465a-addb-fc8938a608db"

const testRWBody = `[
	{
		"predicate": "http://www.ft.com/ontology/annotation/mentions",
		"id": "http://api.ft.com/things/0a619d71-9af5-3755-90dd-f789b686c67a"
	},
	{
		"predicate": "http://www.ft.com/ontology/annotation/hasAuthor",
		"id": "http://api.ft.com/things/838b3fbe-efbc-3cfe-b5c0-d38c046492a4"
	}
]`

var expectedRWAnnotations = []*Annotation{
	{
		Predicate: "http://www.ft.com/ontology/annotation/mentions",
		ConceptId: "http://api.ft.com/things/0a619d71-9af5-3755-90dd-f789b686c67a",
	},
	{
		Predicate: "http://www.ft.com/ontology/annotation/hasAuthor",
		ConceptId: "http://api.ft.com/things/838b3fbe-efbc-3cfe-b5c0-d38c046492a4",
	},
}

func TestHappyRead(t *testing.T) {
	tid := tidUtils.NewTransactionID()
	s := newAnnotationsRWServerMock(t, http.StatusOK, testRWBody, tid)
	defer s.Close()

	rw := NewRW(s.URL)
	ctx := tidUtils.TransactionAwareContext(context.Background(), tid)
	actualAnnotations, found, err := rw.Read(ctx, testContentUUID)
	assert.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, expectedRWAnnotations, actualAnnotations)
}

func TestReadAnnotationsNotFound(t *testing.T) {
	tid := tidUtils.NewTransactionID()
	s := newAnnotationsRWServerMock(t, http.StatusNotFound, "", tid)
	defer s.Close()

	rw := NewRW(s.URL)
	ctx := tidUtils.TransactionAwareContext(context.Background(), tid)
	_, found, err := rw.Read(ctx, testContentUUID)
	assert.NoError(t, err)
	assert.False(t, found)
}

func TestRead500Error(t *testing.T) {
	tid := tidUtils.NewTransactionID()
	s := newAnnotationsRWServerMock(t, http.StatusInternalServerError, "", tid)
	defer s.Close()

	rw := NewRW(s.URL)
	ctx := tidUtils.TransactionAwareContext(context.Background(), tid)
	_, found, err := rw.Read(ctx, testContentUUID)
	assert.EqualError(t, err, "annotations RW returned an unexpected HTTP status code in read operation: 500")
	assert.False(t, found)
}

func TestReadHTTPRequestError(t *testing.T) {
	tid := tidUtils.NewTransactionID()

	rw := NewRW(":#")
	ctx := tidUtils.TransactionAwareContext(context.Background(), tid)
	_, found, err := rw.Read(ctx, testContentUUID)
	assert.EqualError(t, err, "parse :: missing protocol scheme")
	assert.False(t, found)
}

func TestReadHTTPCallError(t *testing.T) {
	tid := tidUtils.NewTransactionID()

	rw := NewRW("")
	ctx := tidUtils.TransactionAwareContext(context.Background(), tid)
	_, found, err := rw.Read(ctx, testContentUUID)
	assert.EqualError(t, err, "Get /drafts/content/db4daee0-2b84-465a-addb-fc8938a608db/annotations: unsupported protocol scheme \"\"")
	assert.False(t, found)
}

func TestReadInvalidBodyError(t *testing.T) {
	tid := tidUtils.NewTransactionID()
	s := newAnnotationsRWServerMock(t, http.StatusOK, "{invalid-body}", tid)
	defer s.Close()

	rw := NewRW(s.URL)
	ctx := tidUtils.TransactionAwareContext(context.Background(), tid)
	_, found, err := rw.Read(ctx, testContentUUID)
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

func newAnnotationsRWServerMock(t *testing.T, status int, body string, tid string) *httptest.Server {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "GET", r.Method)
		assert.Equal(t, "/drafts/content/"+testContentUUID+"/annotations", r.URL.Path)
		if tid == "" {
			assert.NotEmpty(t, r.Header.Get(tidUtils.TransactionIDHeader))
		} else {
			assert.Equal(t, tid, r.Header.Get(tidUtils.TransactionIDHeader))
		}
		w.WriteHeader(status)
		w.Write([]byte(body))
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
