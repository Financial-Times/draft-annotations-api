package health

import (
	"fmt"
	"net/http"

	fthealth "github.com/Financial-Times/go-fthealth/v1_1"
	"github.com/Financial-Times/service-status-go/gtg"
)

type service interface {
	Endpoint() string
	GTG() error
}

type HealthService struct {
	fthealth.HealthCheck
	rw               service
	annotationsAPI   service
	conceptSearchAPI service
}

func NewHealthService(appSystemCode string, appName string, appDescription string, rw service, annotationsAPI service, conceptSearchAPI service) *HealthService {
	hcService := &HealthService{
		rw:               rw,
		annotationsAPI:   annotationsAPI,
		conceptSearchAPI: conceptSearchAPI,
	}
	hcService.SystemCode = appSystemCode
	hcService.Name = appName
	hcService.Description = appDescription
	hcService.Checks = []fthealth.Check{
		hcService.rwCheck(),
		hcService.annotationsAPICheck(),
		hcService.conceptSearchAPICheck(),
	}
	return hcService
}

func (service *HealthService) HealthCheckHandleFunc() func(w http.ResponseWriter, r *http.Request) {
	return fthealth.Handler(service)
}

func (service *HealthService) rwCheck() fthealth.Check {
	return fthealth.Check{
		ID:               "check-generic-rw-aurora-health",
		BusinessImpact:   "Impossible to read and/or write annotations in PAC",
		Name:             "Check Generic RW Aurora Health",
		PanicGuide:       "https://dewey.ft.com/draft-annotations-api.html",
		Severity:         1,
		TechnicalSummary: fmt.Sprintf("Generic RW Aurora is not available at %v", service.rw.Endpoint()),
		Checker:          service.rwChecker,
	}
}

func (service *HealthService) rwChecker() (string, error) {
	if err := service.rw.GTG(); err != nil {
		return "", err
	}
	return "Generic RW Aurora is healthy", nil
}

func (service *HealthService) annotationsAPICheck() fthealth.Check {
	return fthealth.Check{
		ID:               "check-annotations-api-health",
		BusinessImpact:   "Impossible to serve annotations through PAC",
		Name:             "Check UPP Public Annotations API Health",
		PanicGuide:       "https://dewey.ft.com/draft-annotations-api.html",
		Severity:         1,
		TechnicalSummary: fmt.Sprintf("UPP Public Annotations API is not available at %v", service.annotationsAPI.Endpoint()),
		Checker:          service.annotationsAPIChecker,
	}
}

func (service *HealthService) annotationsAPIChecker() (string, error) {
	if err := service.annotationsAPI.GTG(); err != nil {
		return "", err
	}
	return "UPP Public Annotations API is healthy", nil
}

func (service *HealthService) conceptSearchAPICheck() fthealth.Check {
	return fthealth.Check{
		ID:               "check-concept-search-api-health",
		BusinessImpact:   "Impossible to serve annotations with enriched concept data to clients",
		Name:             "Check UPP Concept Search API Health",
		PanicGuide:       "https://dewey.ft.com/draft-annotations-api.html",
		Severity:         1,
		TechnicalSummary: fmt.Sprintf("UPP Concept Search API is not available at %v", service.conceptSearchAPI.Endpoint()),
		Checker:          service.conceptSearchAPIChecker,
	}
}

func (service *HealthService) conceptSearchAPIChecker() (string, error) {
	if err := service.conceptSearchAPI.GTG(); err != nil {
		return "", err
	}
	return "UPP Concept Search API is healthy", nil
}

func (service *HealthService) GTG() gtg.Status {
	for _, check := range service.Checks {
		if _, err := check.Checker(); err != nil {
			return gtg.Status{GoodToGo: false, Message: err.Error()}
		}
	}
	return gtg.Status{GoodToGo: true}
}
