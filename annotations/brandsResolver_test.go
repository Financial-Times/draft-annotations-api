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
	ftBrand      = "dbb0bdae-1f0c-11e4-b0cb-b2227cce2b54"
	fastFtBrand  = "5c7592a8-1f0c-11e4-b0cb-b2227cce2b54"
	lexLiveBrand = "e363dfb8-f6d9-4f2c-beba-5162b334272b"
	lexBrand     = "2d3e16e0-61cb-4322-8aff-3b01c59f4daa"
	reutersBrand = "ed3b6ec5-6466-47ef-b1d8-16952fd522c7"
)

type BrandsResolverTestSuite struct {
	suite.Suite
	brands  map[string]Brand
	handler http.HandlerFunc
}

func TestBrandsResolverSuite(t *testing.T) {
	suite.Run(t, newBrandsResolverTestSuite())
}

func newBrandsResolverTestSuite() *BrandsResolverTestSuite {
	return &BrandsResolverTestSuite{brands: make(map[string]Brand)}
}

func (s *BrandsResolverTestSuite) SetupTest() {
	s.brands[ftBrand] = Brand{ID: ftBrand}
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

	resolver := NewBrandsResolver(server.URL + "/%v", testAPIKey)

	actual := resolver.GetBrands(ftBrand)
	assert.Len(s.T(), actual, 1, "brands")
	assert.Equal(s.T(), ftBrand, actual[0], "FT brand")
}

func (s *BrandsResolverTestSuite) TestCacheWarming() {
	server := httptest.NewServer(s.handler)
	defer server.Close()

	resolver := NewBrandsResolver(server.URL + "/%v", testAPIKey)
	resolver.Refresh([]string{ftBrand})
	server.Close()

	actual := resolver.GetBrands(ftBrand)
	assert.Len(s.T(), actual, 1, "brands")
	assert.Equal(s.T(), ftBrand, actual[0], "FT brand")
}

func (s *BrandsResolverTestSuite) TestGetFtChildBrandUsingParent() {
	s.brands[fastFtBrand] = Brand{ID: fastFtBrand, ParentBrand: &Brand{ID: ftBrand}}
	server := httptest.NewServer(s.handler)
	defer server.Close()

	resolver := NewBrandsResolver(server.URL + "/%v", testAPIKey)

	actual := resolver.GetBrands(fastFtBrand)
	assert.Len(s.T(), actual, 2, "brands")
	assert.Contains(s.T(), actual, ftBrand, "FT brand")
	assert.Contains(s.T(), actual, fastFtBrand, "FastFT brand")
}

func (s *BrandsResolverTestSuite) TestGetFtChildBrandUsingChildren() {
	s.brands[ftBrand] = Brand{ID: ftBrand, ChildBrands: []Brand{{ID:fastFtBrand}}}
	server := httptest.NewServer(s.handler)
	defer server.Close()

	resolver := NewBrandsResolver(server.URL + "/%v", testAPIKey)
	resolver.Refresh([]string{ftBrand})
	server.Close()

	actual := resolver.GetBrands(fastFtBrand)
	assert.Len(s.T(), actual, 2, "brands")
	assert.Contains(s.T(), actual, ftBrand, "FT brand")
	assert.Contains(s.T(), actual, fastFtBrand, "FastFT brand")
}

func (s *BrandsResolverTestSuite) TestGetFtGrandchildBrand() {
	s.brands[lexLiveBrand] = Brand{ID: lexLiveBrand, ParentBrand: &Brand{ID: lexBrand}}
	s.brands[lexBrand] = Brand{ID: lexBrand, ParentBrand: &Brand{ID: ftBrand}}
	server := httptest.NewServer(s.handler)
	defer server.Close()

	resolver := NewBrandsResolver(server.URL + "/%v", testAPIKey)

	actual := resolver.GetBrands(lexLiveBrand)
	assert.Len(s.T(), actual, 3, "brands")
	assert.Contains(s.T(), actual, ftBrand, "FT brand")
	assert.Contains(s.T(), actual, lexBrand, "Lex brand")
	assert.Contains(s.T(), actual, lexLiveBrand, "Lex Live brand")
}

func (s *BrandsResolverTestSuite) TestGetNonFtBrand() {
	s.brands[reutersBrand] = Brand{ID: reutersBrand} // as if it were a distinct top-level brand
	server := httptest.NewServer(s.handler)
	defer server.Close()

	resolver := NewBrandsResolver(server.URL + "/%v", testAPIKey)

	actual := resolver.GetBrands(reutersBrand)
	assert.Len(s.T(), actual, 1, "brands")
	assert.Equal(s.T(), reutersBrand, actual[0], "Reuters brand")
}

func (s *BrandsResolverTestSuite) TestGetUnknownBrand() {
	unknownBrand := "00000000-0000-0000-0000-000000000000"
	server := httptest.NewServer(s.handler)
	defer server.Close()

	resolver := NewBrandsResolver(server.URL + "/%v", testAPIKey)

	actual := resolver.GetBrands(unknownBrand)
	assert.Empty(s.T(), actual, "brands")
}

func (s *BrandsResolverTestSuite) TestBrandsApiUnavailable() {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	resolver := NewBrandsResolver(server.URL + "/%v", testAPIKey)

	actual := resolver.GetBrands(ftBrand)
	assert.Empty(s.T(), actual, "brands")
}

func (s *BrandsResolverTestSuite) TestBrandsApiUnreachable() {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))

	resolver := NewBrandsResolver(server.URL + "/%v", testAPIKey)
	server.Close()

	actual := resolver.GetBrands(ftBrand)
	assert.Empty(s.T(), actual, "brands")
}

func (s *BrandsResolverTestSuite) TestBrandsApiReturnsClientError() {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer server.Close()

	resolver := NewBrandsResolver(server.URL + "/%v", "")

	actual := resolver.GetBrands(ftBrand)
	assert.Empty(s.T(), actual, "brands")
}
