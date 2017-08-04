package health

import (
	"context"
	"encoding/json"
	"errors"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"

	status "github.com/Financial-Times/service-status-go/httphandlers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestHappyHealthCheck(t *testing.T) {
	annotationsAPI := new(AnnotationsAPIMock)
	annotationsAPI.On("GTG").Return(nil)
	annotationsAPI.On("Endpoint").Return("http://cool.api.ft.com/content")

	h := NewHealthService("", "", "", annotationsAPI)

	req := httptest.NewRequest("GET", "/__health", nil)
	w := httptest.NewRecorder()
	h.HealthCheckHandleFunc()(w, req)

	resp := w.Result()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	hcBody := make(map[string]interface{})

	err := json.NewDecoder(resp.Body).Decode(&hcBody)
	assert.NoError(t, err)
	assert.Len(t, hcBody["checks"], 1)
	assert.True(t, hcBody["ok"].(bool))

	check := hcBody["checks"].([]interface{})[0].(map[string]interface{})
	assert.True(t, check["ok"].(bool))
	assert.Equal(t, "UPP Public Annotations API is healthy", check["checkOutput"])
	assert.Equal(t, "UPP Public Annotations API is not available at http://cool.api.ft.com/content", check["technicalSummary"])

	annotationsAPI.AssertExpectations(t)
}

func TestUnhappyHealthCheck(t *testing.T) {
	annotationsAPI := new(AnnotationsAPIMock)
	annotationsAPI.On("GTG").Return(errors.New("computer says no"))
	annotationsAPI.On("Endpoint").Return("http://cool.api.ft.com/content")
	h := NewHealthService("", "", "", annotationsAPI)

	req := httptest.NewRequest("GET", "/__health", nil)
	w := httptest.NewRecorder()
	h.HealthCheckHandleFunc()(w, req)

	resp := w.Result()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	hcBody := make(map[string]interface{})
	err := json.NewDecoder(resp.Body).Decode(&hcBody)

	assert.NoError(t, err)
	assert.Len(t, hcBody["checks"], 1)
	assert.False(t, hcBody["ok"].(bool))

	check := hcBody["checks"].([]interface{})[0].(map[string]interface{})
	assert.False(t, check["ok"].(bool))
	assert.Equal(t, "computer says no", check["checkOutput"])
	assert.Equal(t, "UPP Public Annotations API is not available at http://cool.api.ft.com/content", check["technicalSummary"])

	annotationsAPI.AssertExpectations(t)
}

func TestHappyGTG(t *testing.T) {
	annotationsAPI := new(AnnotationsAPIMock)
	annotationsAPI.On("GTG").Return(nil)
	annotationsAPI.On("Endpoint").Return("http://cool.api.ft.com/content")
	h := NewHealthService("", "", "", annotationsAPI)

	req := httptest.NewRequest("GET", "/__gtg", nil)
	w := httptest.NewRecorder()
	status.NewGoodToGoHandler(h.GTG)(w, req)

	resp := w.Result()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	annotationsAPI.AssertExpectations(t)
}

func TestUnhappyGTG(t *testing.T) {
	annotationsAPI := new(AnnotationsAPIMock)
	annotationsAPI.On("GTG").Return(errors.New("computer says no"))
	annotationsAPI.On("Endpoint").Return("http://cool.api.ft.com/content")
	h := NewHealthService("", "", "", annotationsAPI)

	req := httptest.NewRequest("GET", "/__gtg", nil)
	w := httptest.NewRecorder()
	status.NewGoodToGoHandler(h.GTG)(w, req)

	resp := w.Result()
	assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)

	body, err := ioutil.ReadAll(resp.Body)
	assert.NoError(t, err)
	assert.Equal(t, "computer says no", string(body))

	annotationsAPI.AssertExpectations(t)
}

type AnnotationsAPIMock struct {
	mock.Mock
}

func (m *AnnotationsAPIMock) Get(ctx context.Context, contentUUID string) (*http.Response, error) {
	args := m.Called(ctx, contentUUID)
	return args.Get(0).(*http.Response), args.Error(1)
}

func (m *AnnotationsAPIMock) GTG() error {
	args := m.Called()
	return args.Error(0)
}

func (m *AnnotationsAPIMock) Endpoint() string {
	args := m.Called()
	return args.String(0)
}
