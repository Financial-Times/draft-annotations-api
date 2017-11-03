package annotations

import (
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"net/http"
	"net/http/httptest"
	"github.com/husobee/vestigo"
	"testing"
)

const (
	gtgPath        = "/__concept-search-api/__gtg"
	conceptsPath   = "/concepts"
	testCredetials = "username:password"
)

func TestIsGTG(t *testing.T) {
	m := new(mockConceptSearchAPIServer)
	m.On("IsGTG").Return(http.StatusOK)

	testServer := m.startMockConceptSearchAPIServer(t)
	defer testServer.Close()

	conceptAugmenter, err := NewConceptAugmenter(testServer.URL+conceptsPath, testServer.URL+gtgPath, testCredetials, testAPIKey)
	assert.NoError(t, err, "Creation of a new concept sugmenter should not return an error")
	assert.NoError(t, conceptAugmenter.IsGTG(), "No GTG errors")
}

func TestIsNotGTG(t *testing.T) {
	m := new(mockConceptSearchAPIServer)
	m.On("IsGTG").Return(http.StatusServiceUnavailable)

	testServer := m.startMockConceptSearchAPIServer(t)
	defer testServer.Close()

	conceptAugmenter, err := NewConceptAugmenter(testServer.URL+conceptsPath, testServer.URL+gtgPath, testCredetials, testAPIKey)
	assert.NoError(t, err, "Creation of a new concept sugmenter should not return an error")
	assert.EqualError(t, conceptAugmenter.IsGTG(), "gtg returned a non-200 HTTP status [503]: ", "GTG should return 503")
}

func TestGTGAuthorizationError(t *testing.T) {
	m := new(mockConceptSearchAPIServer)
	m.On("IsGTG").Return(http.StatusUnauthorized)

	testServer := m.startMockConceptSearchAPIServer(t)
	defer testServer.Close()

	conceptAugmenter, err := NewConceptAugmenter(testServer.URL+conceptsPath, testServer.URL+gtgPath, testCredetials, testAPIKey)
	assert.NoError(t, err, "Creation of a new concept sugmenter should not return an error")
	assert.EqualError(t, conceptAugmenter.IsGTG(), "gtg returned a non-200 HTTP status [401]: ", "GTG should return 401")
}

/*func TestGTGInvalidURL(t *testing.T) {
	conceptAugmenter, createErr := NewConceptAugmenter(":#",":#", testCredetials, testAPIKey )
	assert.NoError(t, createErr, "Creation of a new concept sugmenter should not return an error")
	err := conceptAugmenter.IsGTG()
	assert.EqualError(t, err, "gtg request error: parse :: missing protocol scheme")
}*/





type mockConceptSearchAPIServer struct {
	mock.Mock
}



func (m *mockConceptSearchAPIServer) IsGTG() int {
	args := m.Called()
	return args.Int(0)
}

func (m *mockConceptSearchAPIServer) startMockConceptSearchAPIServer(t *testing.T) *httptest.Server {

	r := vestigo.NewRouter()

	r.HandleFunc(gtgPath, func(w http.ResponseWriter, req *http.Request) {
		user, password, ok := req.BasicAuth()
		assert.True(t, ok)
		assert.Equal(t, "username", user)
		assert.Equal(t, "password", password)
		w.WriteHeader(m.IsGTG())
	})

	return httptest.NewServer(r)
}
