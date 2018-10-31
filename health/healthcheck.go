package health

import (
	"fmt"
	"net/http"
	"time"

	fthealth "github.com/Financial-Times/go-fthealth/v1_1"
	"github.com/Financial-Times/service-status-go/gtg"
	log "github.com/sirupsen/logrus"
)

type externalService interface {
	Endpoint() string
	GTG() error
}

type HealthService struct {
	fthealth.TimedHealthCheck
	rw             externalService
	annotationsAPI externalService
	conceptRead    externalService
}

func NewHealthService(appSystemCode string, appName string, appDescription string, rw externalService, annotationsAPI externalService, conceptRead externalService) *HealthService {
	hcService := &HealthService{
		rw:             rw,
		annotationsAPI: annotationsAPI,
		conceptRead:    conceptRead,
	}
	hcService.SystemCode = appSystemCode
	hcService.Name = appName
	hcService.Description = appDescription
	hcService.Timeout = 10 * time.Second
	hcService.Checks = []fthealth.Check{
		hcService.rwCheck(),
		hcService.annotationsAPICheck(),
		hcService.conceptReadCheck(),
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
		Checker:          service.externalServiceChecker(service.rw, "Generic RW Aurora is healthy"),
	}
}

func (service *HealthService) annotationsAPICheck() fthealth.Check {
	return fthealth.Check{
		ID:               "check-annotations-api-health",
		BusinessImpact:   "Impossible to serve annotations through PAC",
		Name:             "Check UPP Public Annotations API Health",
		PanicGuide:       "https://dewey.ft.com/draft-annotations-api.html",
		Severity:         1,
		TechnicalSummary: fmt.Sprintf("UPP Public Annotations API is not available at %v", service.annotationsAPI.Endpoint()),
		Checker:          service.externalServiceChecker(service.annotationsAPI, "UPP Public Annotations API is healthy"),
	}
}

func (service *HealthService) conceptReadCheck() fthealth.Check {
	return fthealth.Check{
		ID:               "check-internal-concordances-api-health",
		BusinessImpact:   "Impossible to serve annotations with enriched concept data to clients",
		Name:             "Check UPP Internal Concordances API Health",
		PanicGuide:       "https://dewey.ft.com/draft-annotations-api.html",
		Severity:         1,
		TechnicalSummary: fmt.Sprintf("UPP Internal Concordances API is not available at %v", service.conceptRead.Endpoint()),
		Checker:          service.externalServiceChecker(service.conceptRead, "UPP Internal Concordances API is healthy"),
	}
}

func (service *HealthService) externalServiceChecker(svc externalService, okMsg string) func() (string, error) {
	return func() (string, error) {
		if err := svc.GTG(); err != nil {
			log.WithField("url", svc.Endpoint()).WithError(err).Error("Failed to check external service")
			return "", err
		}
		return okMsg, nil
	}
}

func (service *HealthService) GTG() gtg.Status {
	var checks []gtg.StatusChecker

	for idx := range service.Checks {
		check := service.Checks[idx]

		checks = append(checks, func() gtg.Status {
			if _, err := check.Checker(); err != nil {
				return gtg.Status{GoodToGo: false, Message: err.Error()}
			}
			return gtg.Status{GoodToGo: true}
		})
	}
	return gtg.FailFastParallelCheck(checks)()
}
