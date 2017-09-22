package annotations

import (
	"context"
	"encoding/json"
	"io"
	"net/http"

	tidutils "github.com/Financial-Times/transactionid-utils-go"
	"github.com/husobee/vestigo"
	log "github.com/sirupsen/logrus"
)

type Handler struct {
	annotationsAPI     AnnotationsAPI
	annotationsService AnnotationsService
}

func NewHandler(api AnnotationsAPI, srv AnnotationsService) *Handler {
	return &Handler{api, srv}
}

func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	uuid := vestigo.Param(r, "uuid")
	tID := tidutils.GetTransactionIDFromRequest(r)
	ctx := tidutils.TransactionAwareContext(context.Background(), tID)

	draftAnnotations, err := h.annotationsService.Read(ctx, uuid)
	if err != nil {
		switch err.(type) {
		case *UPPAnnotationsApiError:
			apiError := err.(*UPPAnnotationsApiError)
			if apiError.StatusCode == http.StatusNotFound || apiError.StatusCode == http.StatusBadRequest {
				w.WriteHeader(apiError.StatusCode)
				io.Copy(w, apiError.Body)
			} else {
				log.WithError(err).WithField(tidutils.TransactionIDKey, tID).WithField("uuid", uuid).Error("Error in reading annotations")
				writeMessage(w, "Service unavailable", http.StatusServiceUnavailable)
			}
		default:
			log.WithError(err).WithField(tidutils.TransactionIDKey, tID).WithField("uuid", uuid).Error("Error in reading annotations")
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}

	w.Header().Add("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(&draftAnnotations)
}

func (h *Handler) Put(w http.ResponseWriter, r *http.Request) {
	uuid := vestigo.Param(r, "uuid")
	tID := tidutils.GetTransactionIDFromRequest(r)
	ctx := tidutils.TransactionAwareContext(context.Background(), tID)

	draftAnnotations := []Annotation{}
	json.NewDecoder(r.Body).Decode(&draftAnnotations)

	draftAnnotations, err := h.annotationsService.Write(ctx, uuid, draftAnnotations, true)
	if err != nil {
		log.WithError(err).WithField(tidutils.TransactionIDKey, tID).WithField("uuid", uuid).Error("Error in processing annotations")
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Add("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(&draftAnnotations)
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
