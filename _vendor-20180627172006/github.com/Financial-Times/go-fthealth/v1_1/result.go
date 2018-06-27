package v1_1

import (
	"time"
)

type CheckResult struct {
	ID               string    `json:"id"`
	Name             string    `json:"name"`
	Ok               bool      `json:"ok"`
	Severity         uint8     `json:"severity"`
	BusinessImpact   string    `json:"businessImpact"`
	TechnicalSummary string    `json:"technicalSummary"`
	PanicGuide       string    `json:"panicGuide"`
	CheckOutput      string    `json:"checkOutput"`
	LastUpdated      time.Time `json:"lastUpdated"`
	Ack              string    `json:"ack,omitempty"`
	PanicGuideIsLink bool      `json:"-"`
}

type HealthResult struct {
	SchemaVersion float64       `json:"schemaVersion"`
	SystemCode    string        `json:"systemCode"`
	Name          string        `json:"name"`
	Description   string        `json:"description"`
	Checks        []CheckResult `json:"checks"`
	Ok            bool          `json:"ok"`
	Severity      uint8         `json:"severity,omitempty"`
}

func ComputeOverallStatus(result *HealthResult) bool {
	for _, check := range result.Checks {
		if !check.Ok {
			return false
		}
	}
	return true
}

func ComputeOverallSeverity(result *HealthResult) uint8 {
	var severity uint8 = 3
	for _, check := range result.Checks {
		if check.Ok == false && check.Severity < severity {
			severity = check.Severity
		}
	}
	return severity
}
