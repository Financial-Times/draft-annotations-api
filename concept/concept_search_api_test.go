package concept

import (
	"context"
	"encoding/json"
	"fmt"
	tidUtils "github.com/Financial-Times/transactionid-utils-go"
	"github.com/Pallinder/go-randomdata"
	"github.com/satori/go.uuid"
	log "github.com/sirupsen/logrus"
	logTest "github.com/sirupsen/logrus/hooks/test"
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
	hook := logTest.NewGlobal()
	batchSize := 20
	expectedConcepts := generateConcepts(10)
	apiKey := randomdata.RandStringRunes(10)

	s := newMockedHappySearchService(t, apiKey, batchSize, "", expectedConcepts)
	defer s.Close()

	csAPI := NewSearchAPI(s.URL, apiKey, batchSize)

	actualConcepts, err := csAPI.SearchConcepts(context.Background(), extractIDs(expectedConcepts))
	assert.NoError(t, err)
	assert.Equal(t, expectedConcepts, actualConcepts)

	var tid string
	for i, e := range hook.AllEntries() {
		if i == 0 {
			assert.Equal(t, log.WarnLevel, e.Level)
			assert.Equal(t, "Transaction ID error for requests of concepts to concept search API: Generated a new transaction ID", e.Message)
			tid = e.Data[tidUtils.TransactionIDKey].(string)
			assert.NotEmpty(t, tid)
		} else {
			assert.Equal(t, tid, e.Data[tidUtils.TransactionIDKey])
		}
	}
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

	s := newMockedUnhappySearchService(http.StatusOK, "}-a-wrong-json-payload-{")
	defer s.Close()

	csAPI := NewSearchAPI(s.URL, apiKey, batchSize)

	ctx := tidUtils.TransactionAwareContext(context.Background(), tidUtils.NewTransactionID())
	_, err := csAPI.SearchConcepts(ctx, []string{"an-id"})
	assert.EqualError(t, err, "invalid character '}' looking for beginning of value")
}

func TestHappyGTG(t *testing.T) {
	batchSize := 20
	expectedConcepts := map[string]Concept{ftBrandUUID: generateConcept(ftBrandUUID)}
	apiKey := randomdata.RandStringRunes(10)

	s := newMockedHappySearchService(t, apiKey, batchSize, "", expectedConcepts)
	defer s.Close()

	csAPI := NewSearchAPI(s.URL, apiKey, batchSize)

	err := csAPI.GTG()
	assert.NoError(t, err)
}

func TestUnhappyGTG(t *testing.T) {
	batchSize := 20
	apiKey := randomdata.RandStringRunes(10)

	s := newMockedUnhappySearchService(http.StatusServiceUnavailable, "I am not happy")
	defer s.Close()

	csAPI := NewSearchAPI(s.URL, apiKey, batchSize)

	err := csAPI.GTG()
	assert.EqualError(t, err, "concept search API returned a non-200 HTTP status code: 503")
}

func generateConcepts(n int) map[string]Concept {
	concepts := make(map[string]Concept)
	for i := 0; i < n; i++ {
		id := uuid.NewV4().String()
		concepts[id] = generateConcept(id)
	}
	return concepts
}

func generateConcept(id string) Concept {
	return Concept{
		Id:         id,
		ApiUrl:     "https://api.ft.com/things/" + id,
		Type:       randomdata.SillyName(),
		PrefLabel:  randomdata.SillyName(),
		IsFTAuthor: randomdata.Boolean(),
	}
}

func extractIDs(concepts map[string]Concept) []string {
	var ids []string
	for id := range concepts {
		ids = append(ids, id)
	}
	return ids
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

	actualApiKey := r.Header.Get(apiKeyHeader)
	assert.Equal(h.t, h.apiKey, actualApiKey)

	actualTID := r.Header.Get(tidUtils.TransactionIDHeader)
	if h.tid != "" {
		assert.Equal(h.t, h.tid, actualTID)
	} else {
		assert.NotEmpty(h.t, actualTID)
	}

	var concepts []Concept

	for _, id := range values["ids"] {
		c, found := h.expectedConcepts[id]
		assert.True(h.t, found)
		concepts = append(concepts, c)
	}

	b, err := json.Marshal(SearchResult{concepts})
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
