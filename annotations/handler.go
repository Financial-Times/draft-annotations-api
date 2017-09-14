package annotations

import (
	"context"
	"encoding/json"
	"io"
	"net/http"

	"bytes"
	"github.com/Financial-Times/draft-annotations-api/mapper"
	tidutils "github.com/Financial-Times/transactionid-utils-go"
	"github.com/husobee/vestigo"
	log "github.com/sirupsen/logrus"
	"io/ioutil"
)

type Handler struct {
	annotationsAPI AnnotationsAPI
}

func NewHandler(api AnnotationsAPI) *Handler {
	return &Handler{api}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	uuid := vestigo.Param(r, "uuid")
	tID := tidutils.GetTransactionIDFromRequest(r)
	ctx := tidutils.TransactionAwareContext(context.Background(), tID)
	resp, err := h.annotationsAPI.Get(ctx, uuid)
	if err != nil {
		log.WithError(err).WithField(tidutils.TransactionIDKey, tID).WithField("uuid", uuid).Error("Error in calling UPP Public Annotations API")
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	w.Header().Add("Content-Type", "application/json")
	if resp.StatusCode == http.StatusOK {
		respBody, _ := ioutil.ReadAll(resp.Body)
		convertedBody, err := mapper.ConvertPredicates(respBody)

		if err != nil {
			log.WithError(err).WithField(tidutils.TransactionIDKey, tID).WithField("uuid", uuid).Error("Error in calling UPP Public Annotations API")
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		} else if err == nil && convertedBody == nil {
			writeMessage(w, "No annotations can be found", http.StatusNotFound)
			return
		} else {
			reader := bytes.NewReader(convertedBody)
			w.WriteHeader(resp.StatusCode)
			io.Copy(w, reader)
			return
		}
	}
	if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusBadRequest {
		w.WriteHeader(resp.StatusCode)
		io.Copy(w, resp.Body)
	} else {
		writeMessage(w, "Service unavailable", http.StatusServiceUnavailable)
	}
}

func writeMessage(w http.ResponseWriter, msg string, status int) {
	w.WriteHeader(status)

	message := make(map[string]interface{})
	message["message"] = msg
	j, err := json.Marshal(&message)

	if err != nil {
		log.WithError(err).Warn("Failed to parse provided message to json, this is a bug.")
		return
	}

	w.Write(j)
}
