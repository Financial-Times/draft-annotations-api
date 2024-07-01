package policy

import (
	"net/http"

	"github.com/Financial-Times/go-logger/v2"
)

const (
	Key           = "publication_based_authorization"
	OpaPolicyPath = "draft_annotations_api/publication_based_authorization"
	Read          = "READ"
	Write         = "WRITE"
)

type Result struct {
	IsAuthorized bool     `json:"is_authorized"`
	Reasons      []string `json:"reasons"`
}

func Middleware(n http.Handler, w http.ResponseWriter, req *http.Request, log *logger.UPPLogger, r Result) {
	if r.IsAuthorized {
		n.ServeHTTP(w, req)
	} else {
		log.Infof("Request was not authorised by open policy agent with reasons: %s", r.Reasons)
		http.Error(w, http.StatusText(http.StatusForbidden), http.StatusForbidden)
	}
}
