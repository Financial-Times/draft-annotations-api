package health

import (
	"encoding/json"
	"errors"
	fthealth "github.com/Financial-Times/go-fthealth/v1_1"
	status "github.com/Financial-Times/service-status-go/httphandlers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHappyHealthCheck(t *testing.T) {
	annotationsAPI := new(ServiceMock)
	annotationsAPI.On("GTG").Return(nil)
	annotationsAPI.On("Endpoint").Return("http://cool.api.ft.com/content")

	conceptSearchAPI := new(ServiceMock)
	conceptSearchAPI.On("GTG").Return(nil)
	conceptSearchAPI.On("Endpoint").Return("http://cool.api.ft.com/concepts")

	h := NewHealthService("", "", "", annotationsAPI, conceptSearchAPI)

	req := httptest.NewRequest("GET", "/__health", nil)
	w := httptest.NewRecorder()
	h.HealthCheckHandleFunc()(w, req)

	resp := w.Result()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var result fthealth.HealthResult
	err := json.NewDecoder(resp.Body).Decode(&result)

	assert.NoError(t, err)
	assert.Len(t, result.Checks, 2)
	assert.True(t, result.Ok)

	for _, c := range result.Checks {
		assert.True(t, c.Ok)
		switch c.ID {
		case "check-annotations-api-health":
			assert.Equal(t, "UPP Public Annotations API is healthy", c.CheckOutput)
			assert.Equal(t, "UPP Public Annotations API is not available at http://cool.api.ft.com/content", c.TechnicalSummary)
		case "check-concept-search-api-health":
			assert.Equal(t, "UPP Concept Search API is healthy", c.CheckOutput)
			assert.Equal(t, "UPP Concept Search API is not available at http://cool.api.ft.com/concepts", c.TechnicalSummary)
		}
	}
	annotationsAPI.AssertExpectations(t)
	conceptSearchAPI.AssertExpectations(t)
}

func TestUnhappyHealthCheckDueAnnotationsAPI(t *testing.T) {
	annotationsAPI := new(ServiceMock)
	annotationsAPI.On("GTG").Return(errors.New("computer says no"))
	annotationsAPI.On("Endpoint").Return("http://cool.api.ft.com/content")

	conceptSearchAPI := new(ServiceMock)
	conceptSearchAPI.On("GTG").Return(nil)
	conceptSearchAPI.On("Endpoint").Return("http://cool.api.ft.com/concepts")

	h := NewHealthService("", "", "", annotationsAPI, conceptSearchAPI)

	req := httptest.NewRequest("GET", "/__health", nil)
	w := httptest.NewRecorder()
	h.HealthCheckHandleFunc()(w, req)

	resp := w.Result()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var result fthealth.HealthResult
	err := json.NewDecoder(resp.Body).Decode(&result)

	assert.NoError(t, err)
	assert.Len(t, result.Checks, 2)
	assert.False(t, result.Ok)

	for _, c := range result.Checks {
		switch c.ID {
		case "check-annotations-api-health":
			assert.False(t, c.Ok)
			assert.Equal(t, "computer says no", c.CheckOutput)
			assert.Equal(t, "UPP Public Annotations API is not available at http://cool.api.ft.com/content", c.TechnicalSummary)
		case "check-concept-search-api-health":
			assert.True(t, c.Ok)
			assert.Equal(t, "UPP Concept Search API is healthy", c.CheckOutput)
			assert.Equal(t, "UPP Concept Search API is not available at http://cool.api.ft.com/concepts", c.TechnicalSummary)
		}
	}
	annotationsAPI.AssertExpectations(t)
	conceptSearchAPI.AssertExpectations(t)

}

func TestUnhappyHealthCheckDueConceptSearchAPI(t *testing.T) {
	annotationsAPI := new(ServiceMock)
	annotationsAPI.On("GTG").Return(nil)
	annotationsAPI.On("Endpoint").Return("http://cool.api.ft.com/content")

	conceptSearchAPI := new(ServiceMock)
	conceptSearchAPI.On("GTG").Return(errors.New("computer says no"))
	conceptSearchAPI.On("Endpoint").Return("http://cool.api.ft.com/concepts")

	h := NewHealthService("", "", "", annotationsAPI, conceptSearchAPI)

	req := httptest.NewRequest("GET", "/__health", nil)
	w := httptest.NewRecorder()
	h.HealthCheckHandleFunc()(w, req)

	resp := w.Result()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var result fthealth.HealthResult
	err := json.NewDecoder(resp.Body).Decode(&result)

	assert.NoError(t, err)
	assert.Len(t, result.Checks, 2)
	assert.False(t, result.Ok)

	for _, c := range result.Checks {
		switch c.ID {
		case "check-annotations-api-health":
			assert.True(t, c.Ok)
			assert.Equal(t, "UPP Public Annotations API is healthy", c.CheckOutput)
			assert.Equal(t, "UPP Public Annotations API is not available at http://cool.api.ft.com/content", c.TechnicalSummary)
		case "check-concept-search-api-health":
			assert.False(t, c.Ok)
			assert.Equal(t, "computer says no", c.CheckOutput)
			assert.Equal(t, "UPP Concept Search API is not available at http://cool.api.ft.com/concepts", c.TechnicalSummary)
		}
	}
	annotationsAPI.AssertExpectations(t)
	conceptSearchAPI.AssertExpectations(t)

}

func TestHappyGTG(t *testing.T) {
	annotationsAPI := new(ServiceMock)
	annotationsAPI.On("GTG").Return(nil)
	annotationsAPI.On("Endpoint").Return("http://cool.api.ft.com/content")

	conceptSearchAPI := new(ServiceMock)
	conceptSearchAPI.On("GTG").Return(nil)
	conceptSearchAPI.On("Endpoint").Return("http://cool.api.ft.com/concepts")

	h := NewHealthService("", "", "", annotationsAPI, conceptSearchAPI)

	req := httptest.NewRequest("GET", "/__gtg", nil)
	w := httptest.NewRecorder()
	status.NewGoodToGoHandler(h.GTG)(w, req)

	resp := w.Result()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	annotationsAPI.AssertExpectations(t)
	conceptSearchAPI.AssertExpectations(t)
}

func TestUnhappyGTGDueConceptSearchAPI(t *testing.T) {
	annotationsAPI := new(ServiceMock)
	annotationsAPI.On("GTG").Return(nil)
	annotationsAPI.On("Endpoint").Return("http://cool.api.ft.com/content")

	conceptSearchAPI := new(ServiceMock)
	conceptSearchAPI.On("GTG").Return(errors.New("I am not good at all"))
	conceptSearchAPI.On("Endpoint").Return("http://cool.api.ft.com/concepts")

	h := NewHealthService("", "", "", annotationsAPI, conceptSearchAPI)

	req := httptest.NewRequest("GET", "/__gtg", nil)
	w := httptest.NewRecorder()
	status.NewGoodToGoHandler(h.GTG)(w, req)

	resp := w.Result()
	assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)

	body, err := ioutil.ReadAll(resp.Body)
	assert.NoError(t, err)
	assert.Equal(t, "I am not good at all", string(body))

	annotationsAPI.AssertExpectations(t)
	conceptSearchAPI.AssertExpectations(t)
}

func TestUnhappyGTGDueAnnotationsAPI(t *testing.T) {
	annotationsAPI := new(ServiceMock)
	annotationsAPI.On("GTG").Return(errors.New("I am not good at all"))
	annotationsAPI.On("Endpoint").Return("http://cool.api.ft.com/content")

	conceptSearchAPI := new(ServiceMock)
	conceptSearchAPI.On("Endpoint").Return("http://cool.api.ft.com/concepts")

	h := NewHealthService("", "", "", annotationsAPI, conceptSearchAPI)

	req := httptest.NewRequest("GET", "/__gtg", nil)
	w := httptest.NewRecorder()
	status.NewGoodToGoHandler(h.GTG)(w, req)

	resp := w.Result()
	assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)

	body, err := ioutil.ReadAll(resp.Body)
	assert.NoError(t, err)
	assert.Equal(t, "I am not good at all", string(body))

	annotationsAPI.AssertExpectations(t)
	conceptSearchAPI.AssertExpectations(t)
}

type ServiceMock struct {
	mock.Mock
}

func (m *ServiceMock) GTG() error {
	args := m.Called()
	return args.Error(0)
}

func (m *ServiceMock) Endpoint() string {
	args := m.Called()
	return args.String(0)
}
