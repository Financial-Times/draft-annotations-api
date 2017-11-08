package concept

import (
	"context"
	"encoding/json"
	"fmt"
	tidUtils "github.com/Financial-Times/transactionid-utils-go"
	"github.com/Pallinder/go-randomdata"
	"github.com/satori/go.uuid"
	"github.com/stretchr/testify/assert"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSearchConceptsSingleBatch(t *testing.T) {
	batchSize := 20
	tid := tidUtils.NewTransactionID()
	expectedConcepts := generateConcepts(10)
	apiKey := randomdata.RandStringRunes(10)

	s := newMockedHappySearchService(t, apiKey, batchSize, tid, expectedConcepts)
	defer s.Close()

	csAPI := NewSearchAPI(s.URL, apiKey, batchSize)

	ctx := tidUtils.TransactionAwareContext(context.Background(), tid)
	actualConcepts, err := csAPI.SearchConcepts(ctx, extractIDs(expectedConcepts))
	assert.NoError(t, err)
	assert.Equal(t, expectedConcepts, actualConcepts)
}

func TestSearchConceptsMultipleBatches(t *testing.T) {
	batchSize := 4
	tid := tidUtils.NewTransactionID()
	expectedConcepts := generateConcepts(10)
	apiKey := randomdata.RandStringRunes(10)

	s := newMockedHappySearchService(t, apiKey, batchSize, tid, expectedConcepts)
	defer s.Close()

	csAPI := NewSearchAPI(s.URL, apiKey, batchSize)

	ctx := tidUtils.TransactionAwareContext(context.Background(), tid)
	actualConcepts, err := csAPI.SearchConcepts(ctx, extractIDs(expectedConcepts))
	assert.NoError(t, err)
	assert.Equal(t, expectedConcepts, actualConcepts)
}

func TestSearchConceptsMissingTID(t *testing.T) {
	batchSize := 20
	expectedConcepts := generateConcepts(10)
	apiKey := randomdata.RandStringRunes(10)

	s := newMockedHappySearchService(t, apiKey, batchSize, "", expectedConcepts)
	defer s.Close()

	csAPI := NewSearchAPI(s.URL, apiKey, batchSize)

	actualConcepts, err := csAPI.SearchConcepts(context.Background(), extractIDs(expectedConcepts))
	assert.NoError(t, err)
	assert.Equal(t, expectedConcepts, actualConcepts)
}

func TestSearchConceptsBuildingHTTPRequestError(t *testing.T) {
	batchSize := 20

	apiKey := randomdata.RandStringRunes(10)

	csAPI := NewSearchAPI(":#invalid endpoint", apiKey, batchSize)

	ctx := tidUtils.TransactionAwareContext(context.Background(), tidUtils.NewTransactionID())
	_, err := csAPI.SearchConcepts(ctx, []string{"an-id"})
	assert.EqualError(t, err, "parse :: missing protocol scheme")
}

func TestSearchConceptsHTTPCallError(t *testing.T) {
	batchSize := 20

	apiKey := randomdata.RandStringRunes(10)

	csAPI := NewSearchAPI("", apiKey, batchSize)

	ctx := tidUtils.TransactionAwareContext(context.Background(), tidUtils.NewTransactionID())
	_, err := csAPI.SearchConcepts(ctx, []string{"an-id"})
	assert.EqualError(t, err, "Get ?ids=an-id: unsupported protocol scheme \"\"")
}

func TestSearchConceptsNon200HTTPStatus(t *testing.T) {
	batchSize := 20
	apiKey := randomdata.RandStringRunes(10)

	s := newMockedUnhappySearchService(http.StatusServiceUnavailable, "I am not happy")
	defer s.Close()

	csAPI := NewSearchAPI(s.URL, apiKey, batchSize)

	ctx := tidUtils.TransactionAwareContext(context.Background(), tidUtils.NewTransactionID())
	_, err := csAPI.SearchConcepts(ctx, []string{"an-id"})
	assert.EqualError(t, err, "concept search API returned a non-200 HTTP status code: 503")
}

func TestSearchConceptsUnmarshallingPayloadError(t *testing.T) {
	batchSize := 20
	apiKey := randomdata.RandStringRunes(10)

	s := newMockedUnhappySearchService(http.StatusOK, "What is this?")
	defer s.Close()

	csAPI := NewSearchAPI(s.URL, apiKey, batchSize)

	ctx := tidUtils.TransactionAwareContext(context.Background(), tidUtils.NewTransactionID())
	_, err := csAPI.SearchConcepts(ctx, []string{"an-id"})
	assert.EqualError(t, err, "invalid character 'W' looking for beginning of value")
}

func generateConcepts(n int) map[string]Concept {
	concepts := make(map[string]Concept)
	for i := 0; i < n; i++ {
		id := uuid.NewV4().String()
		concepts[id] = Concept{
			Id:         id,
			ApiUrl:     "https://api.ft.com/things/" + id,
			Types:      generateTypes(),
			PrefLabel:  randomdata.SillyName(),
			IsFTAuthor: randomdata.Boolean(),
		}
	}
	return concepts
}

func extractIDs(concepts map[string]Concept) []string {
	ids := []string{}
	for id := range concepts {
		ids = append(ids, id)
	}
	return ids
}

func generateTypes() []string {
	n := randomdata.Number(1, 5)
	types := make([]string, n)
	for i := 0; i < n; i++ {
		types[i] = randomdata.SillyName()
	}
	return types
}

func newMockedHappySearchService(t *testing.T, apiKey string, batchSize int, tid string, expectedConcepts map[string]Concept) *httptest.Server {

	h := &mockedSearchServiceHandler{
		t:                t,
		apiKey:           apiKey,
		batchSize:        batchSize,
		expectedConcepts: expectedConcepts,
		tid:              tid,
	}

	ts := httptest.NewServer(h)

	return ts
}

type mockedSearchServiceHandler struct {
	t                *testing.T
	apiKey           string
	expectedConcepts map[string]Concept
	batchSize        int
	tid              string
}

func (h *mockedSearchServiceHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	values := r.URL.Query()
	assert.Len(h.t, values, 1)
	assert.True(h.t, len(values["ids"]) <= h.batchSize)

	actualApiKey := r.Header.Get(upp.ApiKeyHeader)
	assert.Equal(h.t, h.apiKey, actualApiKey)

	actualTID := r.Header.Get(tidUtils.TransactionIDHeader)
	if h.tid != "" {
		assert.Equal(h.t, h.tid, actualTID)
	} else {
		assert.NotEmpty(h.t, actualTID)
	}

	result := []Concept{}

	for _, id := range values["ids"] {
		c, found := h.expectedConcepts[id]
		assert.True(h.t, found)
		result = append(result, c)
	}

	b, err := json.Marshal(result)
	assert.NoError(h.t, err)
	w.Write(b)
}

func newMockedUnhappySearchService(status int, msg string) *httptest.Server {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(status)
		fmt.Fprint(w, msg)
	}))
	return ts
}
