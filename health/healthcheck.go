package health

import (
	"fmt"
	"net/http"

	"github.com/Financial-Times/draft-annotations-api/annotations"
	fthealth "github.com/Financial-Times/go-fthealth/v1_1"
	"github.com/Financial-Times/service-status-go/gtg"
)

type HealthService struct {
	fthealth.HealthCheck
	annotationsAPI annotations.AnnotationsAPI
}

func NewHealthService(appSystemCode string, appName string, appDescription string, api annotations.AnnotationsAPI) *HealthService {
	service := &HealthService{annotationsAPI: api}
	service.SystemCode = appSystemCode
	service.Name = appName
	service.Description = appDescription
	service.Checks = []fthealth.Check{
		service.annotationsAPICheck(),
	}
	return service
}

func (service *HealthService) HealthCheckHandleFunc() func(w http.ResponseWriter, r *http.Request) {
	return fthealth.Handler(service)
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
		return "UPP Public Annotations API is not healthy", err
	}
	return "UPP Public Annotations API is healthy", nil
}

func (service *HealthService) GTG() gtg.Status {
	for _, check := range service.Checks {
		if _, err := check.Checker(); err != nil {
			return gtg.Status{GoodToGo: false, Message: err.Error()}
		}
	}
	return gtg.Status{GoodToGo: true}
}
