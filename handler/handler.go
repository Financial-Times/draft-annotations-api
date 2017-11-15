package handler

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

	"fmt"
	"github.com/Financial-Times/draft-annotations-api/annotations"
	"github.com/satori/go.uuid"
)

type Handler struct {
	annotationsRW        annotations.RW
	annotationsAPI       annotations.API
	c14n                 *annotations.Canonicalizer
	annotationsAugmenter annotations.Augmenter
}

func New(rw annotations.RW, annotationsAPI annotations.API, c14n *annotations.Canonicalizer, augmenter annotations.Augmenter) *Handler {
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

	readLog.Info("Reading from annotations RW...")
	rwAnnotations, found, err := h.annotationsRW.ReadDraft(ctx, uuid)
	if err != nil {
		writeMessage(w, fmt.Sprintf("Annotations RW error: %v", err), http.StatusInternalServerError)
		return
	}

	if found {
		readLog.Info("Augmenting annotations...")
		err = h.annotationsAugmenter.AugmentAnnotations(ctx, &rwAnnotations)
		if err != nil {
			writeMessage(w, fmt.Sprintf("Annotations augmenter error: %v", err), http.StatusInternalServerError)
			return
		}
		json.NewEncoder(w).Encode(rwAnnotations)
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

func (h *Handler) WriteAnnotations(w http.ResponseWriter, r *http.Request) {
	w.Header().Add("Content-Type", "application/json")

	uuid := vestigo.Param(r, "uuid")
	tID := tidutils.GetTransactionIDFromRequest(r)
	ctx := tidutils.TransactionAwareContext(context.Background(), tID)

	writeLog := log.WithField(tidutils.TransactionIDKey, tID).WithField("uuid", uuid)

	if err := validateUUID(uuid); err != nil {
		writeLog.WithError(err).Error("Invalid content UUID")
		writeMessage(w, fmt.Sprintf("Invalid content UUID: %v", uuid), http.StatusBadRequest)
		return
	}

	var draftAnnotations []annotations.Annotation
	err := json.NewDecoder(r.Body).Decode(&draftAnnotations)
	if err != nil {
		writeLog.WithError(err).Error("Unable to unmarshal annotations body")
		writeMessage(w, fmt.Sprintf("Unable to unmarshal annotations body: %v", err.Error()), http.StatusBadRequest)
		return
	}

	writeLog.Info("Canonicalizing annotations...")
	draftAnnotations = h.c14n.Canonicalize(draftAnnotations)

	writeLog.Info("Writing from annotations RW...")
	err = h.annotationsRW.WriteDraft(ctx, uuid, draftAnnotations)
	if err != nil {
		writeLog.WithError(err).Error("Error in writing draft annotations")
		writeMessage(w, fmt.Sprintf("Error in writing draft annotations: %v", err.Error()), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func validateUUID(u string) error {
	_, err := uuid.FromString(u)
	return err
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
