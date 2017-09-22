package annotations

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

const (
	ftBrand      = "http://www.ft.com/thing/dbb0bdae-1f0c-11e4-b0cb-b2227cce2b54"
	fastFtBrand  = "http://www.ft.com/thing/5c7592a8-1f0c-11e4-b0cb-b2227cce2b54"
	lexLiveBrand = "http://www.ft.com/thing/e363dfb8-f6d9-4f2c-beba-5162b334272b"
	lexBrand     = "http://www.ft.com/thing/2d3e16e0-61cb-4322-8aff-3b01c59f4daa"
	reutersBrand = "http://www.ft.com/thing/ed3b6ec5-6466-47ef-b1d8-16952fd522c7"
)

type BrandsResolverTestSuite struct {
	suite.Suite
	brands  map[string]Brand
	linter *IDLinter
	handler http.HandlerFunc
}

func TestBrandsResolverSuite(t *testing.T) {
	suite.Run(t, newBrandsResolverTestSuite())
}

func newBrandsResolverTestSuite() *BrandsResolverTestSuite {
	idLinter, _ := NewIDLinter(`^(.+)\/\/api\.ft\.com\/things\/(.+)$`, "$1//www.ft.com/thing/$2")
	return &BrandsResolverTestSuite{brands: make(map[string]Brand), linter: idLinter}
}

func (s *BrandsResolverTestSuite) SetupTest() {
	i := strings.LastIndex(ftBrand, "/")
	s.brands[ftBrand[i+1:]] = Brand{ID: ftBrand}
	s.handler = http.HandlerFunc(
		func(w http.ResponseWriter, req *http.Request) {
			assert.Equal(s.T(), testAPIKey, req.Header.Get("X-Api-Key"), "api key")
			w.Header().Set("Content-Type", "application/json")

			i := strings.LastIndex(req.URL.Path, "/")
			requestUUID := req.URL.Path[i+1:]

			if brands, found := s.brands[requestUUID]; found {
				w.WriteHeader(http.StatusOK)
				json.NewEncoder(w).Encode(brands)
			} else {
				w.WriteHeader(http.StatusNotFound)
			}
		})
}

func (s *BrandsResolverTestSuite) TestGetFtBrand() {
	server := httptest.NewServer(s.handler)
	defer server.Close()

	resolver := NewBrandsResolver(server.URL + "/%v", testAPIKey, s.linter)

	actual := resolver.GetBrands(ftBrand)
	assert.Len(s.T(), actual, 1, "brands")
	assert.Equal(s.T(), ftBrand, actual[0], "FT brand")
}

func (s *BrandsResolverTestSuite) TestCacheWarming() {
	server := httptest.NewServer(s.handler)
	defer server.Close()

	resolver := NewBrandsResolver(server.URL + "/%v", testAPIKey, s.linter)
	resolver.Refresh([]string{ftBrand})
	server.Close()

	actual := resolver.GetBrands(ftBrand)
	assert.Len(s.T(), actual, 1, "brands")
	assert.Equal(s.T(), ftBrand, actual[0], "FT brand")
}

func (s *BrandsResolverTestSuite) TestGetFtChildBrandUsingParent() {
	i := strings.LastIndex(fastFtBrand, "/")
	s.brands[fastFtBrand[i+1:]] = Brand{ID: fastFtBrand, ParentBrand: &Brand{ID: ftBrand}}
	server := httptest.NewServer(s.handler)
	defer server.Close()

	resolver := NewBrandsResolver(server.URL + "/%v", testAPIKey, s.linter)

	actual := resolver.GetBrands(fastFtBrand)
	assert.Len(s.T(), actual, 2, "brands")
	assert.Contains(s.T(), actual, ftBrand, "FT brand")
	assert.Contains(s.T(), actual, fastFtBrand, "FastFT brand")
}

func (s *BrandsResolverTestSuite) TestGetFtChildBrandUsingChildren() {
	i := strings.LastIndex(ftBrand, "/")
	s.brands[ftBrand[i+1:]] = Brand{ID: ftBrand, ChildBrands: []Brand{{ID:fastFtBrand}}}
	server := httptest.NewServer(s.handler)
	defer server.Close()

	resolver := NewBrandsResolver(server.URL + "/%v", testAPIKey, s.linter)
	resolver.Refresh([]string{ftBrand})
	server.Close()

	actual := resolver.GetBrands(fastFtBrand)
	assert.Len(s.T(), actual, 2, "brands")
	assert.Contains(s.T(), actual, ftBrand, "FT brand")
	assert.Contains(s.T(), actual, fastFtBrand, "FastFT brand")
}

func (s *BrandsResolverTestSuite) TestGetFtGrandchildBrand() {
	i := strings.LastIndex(lexLiveBrand, "/")
	s.brands[lexLiveBrand[i+1:]] = Brand{ID: lexLiveBrand, ParentBrand: &Brand{ID: lexBrand}}

	i = strings.LastIndex(lexBrand, "/")
	s.brands[lexBrand[i+1:]] = Brand{ID: lexBrand, ParentBrand: &Brand{ID: ftBrand}}
	server := httptest.NewServer(s.handler)
	defer server.Close()

	resolver := NewBrandsResolver(server.URL + "/%v", testAPIKey, s.linter)

	actual := resolver.GetBrands(lexLiveBrand)
	assert.Len(s.T(), actual, 3, "brands")
	assert.Contains(s.T(), actual, ftBrand, "FT brand")
	assert.Contains(s.T(), actual, lexBrand, "Lex brand")
	assert.Contains(s.T(), actual, lexLiveBrand, "Lex Live brand")
}

func (s *BrandsResolverTestSuite) TestGetNonFtBrand() {
	i := strings.LastIndex(reutersBrand, "/")
	s.brands[reutersBrand[i+1:]] = Brand{ID: reutersBrand} // as if it were a distinct top-level brand
	server := httptest.NewServer(s.handler)
	defer server.Close()

	resolver := NewBrandsResolver(server.URL + "/%v", testAPIKey, s.linter)

	actual := resolver.GetBrands(reutersBrand)
	assert.Len(s.T(), actual, 1, "brands")
	assert.Equal(s.T(), reutersBrand, actual[0], "Reuters brand")
}

func (s *BrandsResolverTestSuite) TestGetUnknownBrand() {
	unknownBrand := "http://www.ft.com/thing/00000000-0000-0000-0000-000000000000"
	server := httptest.NewServer(s.handler)
	defer server.Close()

	resolver := NewBrandsResolver(server.URL + "/%v", testAPIKey, s.linter)

	actual := resolver.GetBrands(unknownBrand)
	assert.Empty(s.T(), actual, "brands")
}

func (s *BrandsResolverTestSuite) TestBrandsApiUnavailable() {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	resolver := NewBrandsResolver(server.URL + "/%v", testAPIKey, s.linter)

	actual := resolver.GetBrands(ftBrand)
	assert.Empty(s.T(), actual, "brands")
}

func (s *BrandsResolverTestSuite) TestBrandsApiUnreachable() {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))

	resolver := NewBrandsResolver(server.URL + "/%v", testAPIKey, s.linter)
	server.Close()

	actual := resolver.GetBrands(ftBrand)
	assert.Empty(s.T(), actual, "brands")
}

func (s *BrandsResolverTestSuite) TestBrandsApiReturnsClientError() {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer server.Close()

	resolver := NewBrandsResolver(server.URL + "/%v", "", s.linter)

	actual := resolver.GetBrands(ftBrand)
	assert.Empty(s.T(), actual, "brands")
}
