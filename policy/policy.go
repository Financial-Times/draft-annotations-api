package policy

import (
	"net/http"

	"github.com/Financial-Times/go-logger/v2"
	"github.com/Financial-Times/opa-client-go"
)

const (
	OpaPolicyPath = "draft_annotations_api/"
	ReadKey       = "read"
	WriteKey      = "write"
	WritePBLC     = "PBLC_WRITE_"
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
func ResponseMiddleware(w opa.ResponseInterceptor, _ *http.Request, log *logger.UPPLogger, r Result) {
	if r.IsAuthorized {
		w.ResponseWriter.Header().Set("Content-Type", "application/json")
		w.ResponseWriter.WriteHeader(w.Status)
		_, err := w.ResponseWriter.Write(w.Body.Bytes())
		if err != nil {
			log.WithError(err).Error("error while writing response to client")
			http.Error(w.ResponseWriter, err.Error(), http.StatusInternalServerError)
		}
	} else {
		log.Infof("Request was not authorised by open policy agent with reasons: %s", r.Reasons)
		http.Error(w.ResponseWriter, http.StatusText(http.StatusForbidden), http.StatusForbidden)
	}
}
