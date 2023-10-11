package concept

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/Financial-Times/draft-annotations-api/basicauth"
	"github.com/Financial-Times/go-ft-http/fthttp"
	tidUtils "github.com/Financial-Times/transactionid-utils-go"
	"github.com/Pallinder/go-randomdata"
	"github.com/google/uuid"
	"github.com/husobee/vestigo"
	log "github.com/sirupsen/logrus"
	logTest "github.com/sirupsen/logrus/hooks/test"
	"github.com/stretchr/testify/assert"
)

var testClient = fthttp.NewClientWithDefaultTimeout("PAC", "draft-annotations-api")

func TestGetConceptsByIDsSingleBatch(t *testing.T) {
	batchSize := 20
	tid := tidUtils.NewTransactionID()
	expectedConcepts := generateConcepts(10)

	s := newMockedHappySearchService(t, basicauth.TestBasicAuthUsername, basicauth.TestBasicAuthPassword, batchSize, tid, expectedConcepts)
	defer s.Close()

	csAPI := NewReadAPI(testClient, s.URL, basicauth.TestBasicAuthUsername, basicauth.TestBasicAuthPassword, batchSize)

	ctx := tidUtils.TransactionAwareContext(context.Background(), tid)
	actualConcepts, err := csAPI.GetConceptsByIDs(ctx, extractIDs(expectedConcepts))
	assert.NoError(t, err)
	assert.Equal(t, expectedConcepts, actualConcepts)
}

func TestGetConceptsByIDsMultipleBatches(t *testing.T) {
	batchSize := 4
	tid := tidUtils.NewTransactionID()
	expectedConcepts := generateConcepts(10)

	s := newMockedHappySearchService(t, basicauth.TestBasicAuthUsername, basicauth.TestBasicAuthPassword, batchSize, tid, expectedConcepts)
	defer s.Close()

	csAPI := NewReadAPI(testClient, s.URL, basicauth.TestBasicAuthUsername, basicauth.TestBasicAuthPassword, batchSize)

	ctx := tidUtils.TransactionAwareContext(context.Background(), tid)
	actualConcepts, err := csAPI.GetConceptsByIDs(ctx, extractIDs(expectedConcepts))
	assert.NoError(t, err)
	assert.Equal(t, expectedConcepts, actualConcepts)
}

func TestGetConceptsByIDsMissingTID(t *testing.T) {
	hook := logTest.NewGlobal()
	batchSize := 20
	expectedConcepts := generateConcepts(10)

	s := newMockedHappySearchService(t, basicauth.TestBasicAuthUsername, basicauth.TestBasicAuthPassword, batchSize, "", expectedConcepts)
	defer s.Close()

	csAPI := NewReadAPI(testClient, s.URL, basicauth.TestBasicAuthUsername, basicauth.TestBasicAuthPassword, batchSize)

	actualConcepts, err := csAPI.GetConceptsByIDs(context.Background(), extractIDs(expectedConcepts))
	assert.NoError(t, err)
	assert.Equal(t, expectedConcepts, actualConcepts)

	var tid string
	for i, e := range hook.AllEntries() {
		if i == 0 {
			assert.Equal(t, log.InfoLevel, e.Level)
			assert.Equal(t, "No Transaction ID provided for concept request, so a new one has been generated.", e.Message)
			tid = e.Data[tidUtils.TransactionIDKey].(string)
			assert.NotEmpty(t, tid)
		} else {
			assert.Equal(t, tid, e.Data[tidUtils.TransactionIDKey])
		}
	}
}

func TestGetConceptsByIDsBuildingHTTPRequestError(t *testing.T) {
	batchSize := 20

	csAPI := NewReadAPI(testClient, ":#invalid endpoint", basicauth.TestBasicAuthUsername, basicauth.TestBasicAuthPassword, batchSize)

	ctx := tidUtils.TransactionAwareContext(context.Background(), tidUtils.NewTransactionID())
	_, err := csAPI.GetConceptsByIDs(ctx, []string{"an-id"})
	var urlError *url.Error
	assert.True(t, errors.As(err, &urlError))
	assert.Equal(t, urlError.Op, "parse")
}

func TestGetConceptsByIDsHTTPCallError(t *testing.T) {
	batchSize := 20

	csAPI := NewReadAPI(testClient, "", basicauth.TestBasicAuthUsername, basicauth.TestBasicAuthPassword, batchSize)

	ctx := tidUtils.TransactionAwareContext(context.Background(), tidUtils.NewTransactionID())
	_, err := csAPI.GetConceptsByIDs(ctx, []string{"an-id"})
	var urlError *url.Error
	assert.True(t, errors.As(err, &urlError))
	assert.Equal(t, urlError.Op, "Get")
}

func TestGetConceptsByIDsNon200HTTPStatus(t *testing.T) {
	batchSize := 20

	s := newMockedUnhappySearchService(http.StatusServiceUnavailable, "I am not happy")
	defer s.Close()

	csAPI := NewReadAPI(testClient, s.URL, basicauth.TestBasicAuthUsername, basicauth.TestBasicAuthPassword, batchSize)

	ctx := tidUtils.TransactionAwareContext(context.Background(), tidUtils.NewTransactionID())
	_, err := csAPI.GetConceptsByIDs(ctx, []string{"an-id"})
	assert.True(t, errors.Is(err, ErrUnexpectedResponse))
}

func TestGetConceptsByIDsUnmarshallingPayloadError(t *testing.T) {
	batchSize := 20

	s := newMockedUnhappySearchService(http.StatusOK, "}-a-wrong-json-payload-{")
	defer s.Close()

	csAPI := NewReadAPI(testClient, s.URL, basicauth.TestBasicAuthUsername, basicauth.TestBasicAuthPassword, batchSize)

	ctx := tidUtils.TransactionAwareContext(context.Background(), tidUtils.NewTransactionID())
	_, err := csAPI.GetConceptsByIDs(ctx, []string{"an-id"})
	var jsonErr *json.SyntaxError
	assert.True(t, errors.As(err, &jsonErr))
}

func TestHappyGTG(t *testing.T) {
	batchSize := 20
	expectedConcepts := map[string]Concept{ftBrandUUID: generateConcept(ftBrandUUID)}

	s := newMockedHappySearchService(t, basicauth.TestBasicAuthUsername, basicauth.TestBasicAuthPassword, batchSize, "", expectedConcepts)
	defer s.Close()

	csAPI := NewReadAPI(testClient, s.URL, basicauth.TestBasicAuthUsername, basicauth.TestBasicAuthPassword, batchSize)

	err := csAPI.GTG()
	assert.NoError(t, err)
}

func TestConceptSearchTimeout(t *testing.T) {
	r := vestigo.NewRouter()
	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(500 * time.Millisecond)
	})

	s := httptest.NewServer(r)
	csAPI := NewReadAPI(testClient, s.URL, basicauth.TestBasicAuthUsername, basicauth.TestBasicAuthPassword, 1)

	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond*50)
	defer cancel()

	_, err := csAPI.GetConceptsByIDs(ctx, []string{"1234"})
	assert.Error(t, err)
	assert.True(t, (err.(net.Error)).Timeout())
}

func TestUnhappyGTG(t *testing.T) {
	batchSize := 20

	s := newMockedUnhappySearchService(http.StatusServiceUnavailable, "I am not happy")
	defer s.Close()

	csAPI := NewReadAPI(testClient, s.URL, basicauth.TestBasicAuthUsername, basicauth.TestBasicAuthPassword, batchSize)

	err := csAPI.GTG()
	assert.True(t, errors.Is(err, ErrUnexpectedResponse))
}

func generateConcepts(n int) map[string]Concept {
	concepts := make(map[string]Concept)
	for i := 0; i < n; i++ {
		id := uuid.New().String()
		concepts[id] = generateConcept(id)
	}
	return concepts
}

func generateConcept(id string) Concept {
	return Concept{
		ID:         id,
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

// Removing unparam as we may want to have the possibility of manipulating the username and password for tests
// nolint:unparam
func newMockedHappySearchService(t *testing.T, username string, password string, batchSize int, tid string, expectedConcepts map[string]Concept) *httptest.Server {
	h := &mockedSearchServiceHandler{
		t:                t,
		username:         username,
		password:         password,
		batchSize:        batchSize,
		expectedConcepts: expectedConcepts,
		tid:              tid,
	}

	ts := httptest.NewServer(h)

	return ts
}

type mockedSearchServiceHandler struct {
	t                *testing.T
	username         string
	password         string
	expectedConcepts map[string]Concept
	batchSize        int
	tid              string
}

func (h *mockedSearchServiceHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	values := r.URL.Query()
	assert.Len(h.t, values, 1)
	assert.True(h.t, len(values["ids"]) <= h.batchSize)

	assert.Equal(h.t, "PAC-draft-annotations-api/Version--is-not-a-semantic-version", r.Header.Get("User-Agent"))

	actualBasicAuth := r.Header.Get(basicauth.AuthorizationHeader)
	assert.Equal(h.t, basicauth.EncodeBasicAuthForTests(h.t), actualBasicAuth)

	actualTID := r.Header.Get(tidUtils.TransactionIDHeader)
	if h.tid != "" {
		assert.Equal(h.t, h.tid, actualTID)
	} else {
		assert.NotEmpty(h.t, actualTID)
	}

	concepts := make(map[string]Concept)

	for _, id := range values["ids"] {
		c, found := h.expectedConcepts[id]
		assert.True(h.t, found)
		concepts[c.ID] = c
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
