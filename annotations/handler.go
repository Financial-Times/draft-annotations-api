package annotations

import (
	"context"
	"io"
	"net/http"

	tidutils "github.com/Financial-Times/transactionid-utils-go"
	"github.com/husobee/vestigo"
	log "github.com/sirupsen/logrus"
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
	w.WriteHeader(resp.StatusCode)

	io.Copy(w, resp.Body)
}
