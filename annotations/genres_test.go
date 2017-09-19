package annotations

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

const (
	news     = "http://www.ft.com/thing/9b40e89c-e87b-3d4f-b72c-2cf7511d2146"
	obituary = "http://www.ft.com/thing/2c4a7847-11ad-308b-b634-4c962708261c"
	recipe   = "http://www.ft.com/thing/a06a0e0f-f5bb-3510-8e70-61b066f821e7"
)

type GenresServiceTestSuite struct {
	suite.Suite
	genres  []Genre
	linter  *IDLinter
	handler http.HandlerFunc
}

func TestGenresServiceSuite(t *testing.T) {
	suite.Run(t, newGenresServiceTestSuite())
}

func newGenresServiceTestSuite() *GenresServiceTestSuite {
	idLinter, _ := NewIDLinter(`^(.+)\/\/api\.ft\.com\/things\/(.+)$`, "$1//www.ft.com/thing/$2")
	return &GenresServiceTestSuite{linter: idLinter}
}

func (s *GenresServiceTestSuite) SetupTest() {
	s.handler = http.HandlerFunc(
		func(w http.ResponseWriter, req *http.Request) {
			assert.Equal(s.T(), testAPIKey, req.Header.Get("X-Api-Key"), "api key")
			w.Header().Set("Content-Type", "application/json")

			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string][]Genre{"concepts": s.genres})
		})
}

func (s *GenresServiceTestSuite) TestGenres() {
	s.genres = []Genre{
		Genre{news},
		Genre{obituary},
	}

	server := httptest.NewServer(s.handler)
	defer server.Close()

	service := NewGenresService(server.URL, testAPIKey, s.linter)

	actual, err := service.Refresh()
	assert.NoError(s.T(), err)
	assert.Len(s.T(), actual, 2, "genres")
	assert.Contains(s.T(), actual, news, "news")
	assert.Contains(s.T(), actual, obituary, "obituary")

	assert.True(s.T(), service.IsConcept(news), "isConcept for news")
	assert.True(s.T(), service.IsGenre(news), "isGenre for news")

	assert.False(s.T(), service.IsConcept(recipe), "isConcept for recipe")
	assert.False(s.T(), service.IsGenre(recipe), "isGenre for recipe")
}

func (s *GenresServiceTestSuite) TestLinter() {
	r := regexp.MustCompile(`^(.+)\/\/www\.ft\.com\/thing\/(.+)$`)
	r.ReplaceAllString(news, "$1//api.ft.com/things/$2")

	s.genres = []Genre{
		Genre{r.ReplaceAllString(news, "$1//api.ft.com/things/$2")},
		Genre{obituary},
	}

	server := httptest.NewServer(s.handler)
	defer server.Close()

	service := NewGenresService(server.URL, testAPIKey, s.linter)

	actual, err := service.Refresh()
	assert.NoError(s.T(), err)
	assert.Len(s.T(), actual, 2, "genres")
	assert.Contains(s.T(), actual, news, "news")
	assert.Contains(s.T(), actual, obituary, "obituary")

	assert.True(s.T(), service.IsConcept(news), "isConcept for news")
	assert.True(s.T(), service.IsGenre(news), "isGenre for news")
	assert.True(s.T(), service.IsConcept(obituary), "isConcept for obituary")
	assert.True(s.T(), service.IsGenre(obituary), "isGenre for obituary")

	assert.False(s.T(), service.IsConcept(recipe), "isConcept for recipe")
	assert.False(s.T(), service.IsGenre(recipe), "isGenre for recipe")
}

func (s *GenresServiceTestSuite) TestGenresApiUnavailable() {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	service := NewGenresService(server.URL, testAPIKey, s.linter)

	_, err := service.Refresh()
	assert.Error(s.T(), err)
}

func (s *GenresServiceTestSuite) TestGenresApiUnreachable() {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))

	service := NewGenresService(server.URL, testAPIKey, s.linter)
	server.Close()

	_, err := service.Refresh()
	assert.Error(s.T(), err)
}

func (s *GenresServiceTestSuite) TestGenresApiReturnsClientError() {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer server.Close()

	service := NewGenresService(server.URL, "", s.linter)

	_, err := service.Refresh()
	assert.Error(s.T(), err)
}

func (s *GenresServiceTestSuite) TestIsGenre() {
}