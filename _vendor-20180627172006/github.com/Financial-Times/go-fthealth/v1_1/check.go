package v1_1

import (
	"fmt"
	"strings"
	"time"
)

type Check struct {
	ID               string
	Name             string
	Severity         uint8
	BusinessImpact   string
	TechnicalSummary string
	PanicGuide       string
	Checker          func() (string, error)
	Timeout          time.Duration
}

func (ch *Check) runChecker() CheckResult {
	result := CheckResult{
		ID:               ch.ID,
		Name:             ch.Name,
		Severity:         ch.Severity,
		BusinessImpact:   ch.BusinessImpact,
		TechnicalSummary: ch.TechnicalSummary,
		PanicGuide:       ch.PanicGuide,
		PanicGuideIsLink: strings.HasPrefix(ch.PanicGuide, "http"),
		LastUpdated:      time.Now(),
	}
	out, err := ch.check()
	if err != nil {
		result.Ok = false
		result.CheckOutput = err.Error()
	} else {
		result.Ok = true
		result.CheckOutput = out
	}
	return result
}

func (ch *Check) check() (string, error) {
	if ch.Timeout != time.Duration(0) {
		type result struct {
			out string
			err error
		}
		resultCh := make(chan result)
		go func() {
			out, err := ch.Checker()
			resultCh <- result{out, err}
		}()
		select {
		case <-time.After(ch.Timeout):
			return "", fmt.Errorf("Timed out after %v second(s)", ch.Timeout.Seconds())
		case res := <-resultCh:
			return res.out, res.err
		}
	}
	return ch.Checker()
}
