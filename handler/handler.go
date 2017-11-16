package handler

import (
	"context"
	"encoding/json"
	"io"
	"net/http"

	"bytes"
	"io/ioutil"

	"github.com/Financial-Times/draft-annotations-api/mapper"
	tidutils "github.com/Financial-Times/transactionid-utils-go"
	"github.com/husobee/vestigo"
	log "github.com/sirupsen/logrus"

	"fmt"

	"github.com/Financial-Times/draft-annotations-api/annotations"
)

type Handler struct {
	annotationsRW        annotations.RW
	annotationsAPI       annotations.UPPAnnotationsAPI
	c14n                 *annotations.Canonicalizer
	annotationsAugmenter annotations.Augmenter
}

func New(rw annotations.RW, annotationsAPI annotations.UPPAnnotationsAPI, c14n *annotations.Canonicalizer, augmenter annotations.Augmenter) *Handler {
	return &Handler{
		rw,
		annotationsAPI,
		c14n,
		augmenter,
	}
}

func (h *Handler) ReadAnnotations(w http.ResponseWriter, r *http.Request) {
	uuid := vestigo.Param(r, "uuid")
	tID := tidutils.GetTransactionIDFromRequest(r)
	ctx := tidutils.TransactionAwareContext(context.Background(), tID)

	readLog := log.WithField(tidutils.TransactionIDKey, tID).WithField("uuid", uuid)

	w.Header().Add("Content-Type", "application/json")

	readLog.Info("Calling annotations RW...")
	rwAnnotations, found, err := h.annotationsRW.Read(ctx, uuid)
	if err != nil {
		writeMessage(w, fmt.Sprintf("Annotations RW error: %v", err), http.StatusInternalServerError)
		return
	}

	if found {
		readLog.Info("Augmenting annotations...")
		augmentedAnnotations, err := h.annotationsAugmenter.AugmentAnnotations(ctx, rwAnnotations)
		if err != nil {
			writeMessage(w, fmt.Sprintf("Annotations augmenter error: %v", err), http.StatusInternalServerError)
			return
		}
		json.NewEncoder(w).Encode(augmentedAnnotations)
		return
	} else {
		readLog.Info("Annotations not found: Retrieving annotations from UPP")
		resp, err := h.annotationsAPI.Get(ctx, uuid)
		if err != nil {
			readLog.WithError(err).Error("Error in calling UPP Public Annotations API")
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode == http.StatusOK {
			respBody, _ := ioutil.ReadAll(resp.Body)
			convertedBody, err := mapper.ConvertPredicates(respBody)
			if err != nil {
				readLog.WithError(err).Error("Error converting predicates from UPP Public Annotations API response")
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
	var draftAnnotations []annotations.Annotation
	json.NewDecoder(r.Body).Decode(&draftAnnotations)

	h.c14n.Canonicalize(draftAnnotations)

	w.WriteHeader(http.StatusOK)
}
