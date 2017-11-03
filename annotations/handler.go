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
	annotationsAPI   AnnotationsAPI
	c14n             *Canonicalizer
	conceptAugmenter ConceptAugmenter
}


func NewHandler(api AnnotationsAPI, c14n *Canonicalizer, ca ConceptAugmenter) *Handler {
	return &Handler{api, c14n, ca}
}

func (h *Handler) ReadAnnotations(w http.ResponseWriter, r *http.Request) {
	uuid := vestigo.Param(r, "uuid")
	tID := tidutils.GetTransactionIDFromRequest(r)
	ctx := tidutils.TransactionAwareContext(context.Background(), tID)
	//wire in aurora call only if uuid not foud in aurora then  call upp
//call aurora rw
	//if get 404 response on content {
		//call upp
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
		//prediacte mapping will only happens for the upp response return as annotations not byte[]
		convertedBody, err := mapper.ConvertPredicates(respBody)
		if err != nil {
			//error in cnversion?
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

func (h *Handler) WriteAnnotations(w http.ResponseWriter, r *http.Request) {
	draftAnnotations := []Annotation{}
	json.NewDecoder(r.Body).Decode(&draftAnnotations)

	h.c14n.canonicalize(draftAnnotations)

	w.WriteHeader(http.StatusOK)
}
