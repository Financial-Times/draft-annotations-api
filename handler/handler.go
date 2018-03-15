package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"time"

	"github.com/Financial-Times/draft-annotations-api/annotations"
	"github.com/Financial-Times/draft-annotations-api/mapper"
	tidutils "github.com/Financial-Times/transactionid-utils-go"
	"github.com/husobee/vestigo"
	"github.com/satori/go.uuid"
	log "github.com/sirupsen/logrus"
)

var (
	errNoAnnotations         = errors.New(`No annotations can be found`)
	errUPPBadRequest         = errors.New(`UPP responded with a client error`)
	errUPPServiceUnavailable = errors.New(`UPP responded with an unexpected error`)
)

type Handler struct {
	annotationsRW        annotations.RW
	annotationsAPI       annotations.UPPAnnotationsAPI
	c14n                 *annotations.Canonicalizer
	annotationsAugmenter annotations.Augmenter
	timeout              time.Duration
}

func New(rw annotations.RW, annotationsAPI annotations.UPPAnnotationsAPI, c14n *annotations.Canonicalizer, augmenter annotations.Augmenter, httpTimeout time.Duration) *Handler {
	return &Handler{
		rw,
		annotationsAPI,
		c14n,
		augmenter,
		httpTimeout,
	}
}

func (h *Handler) ReadAnnotations(w http.ResponseWriter, r *http.Request) {
	contentUUID := vestigo.Param(r, "uuid")
	tID := tidutils.GetTransactionIDFromRequest(r)

	ctx, cancel := context.WithTimeout(tidutils.TransactionAwareContext(context.Background(), tID), h.timeout)
	defer cancel()

	readLog := readLogEntry(ctx, contentUUID)

	w.Header().Add("Content-Type", "application/json")

	readLog.Info("Reading Annotations from Annotations R/W")
	rawAnnotations, err := h.readAnnotations(ctx, w, contentUUID)
	if err != nil {
		handleErrors(err, readLog, w)
		return
	}

	readLog.Info("Augmenting annotations with recent UPP data")
	augmentedAnnotations, err := h.annotationsAugmenter.AugmentAnnotations(ctx, rawAnnotations)
	if err != nil {
		readLog.WithError(err).Error("Failed to augment annotations")
		writeMessage(w, "Failed to augment annotations", http.StatusInternalServerError)
		return
	}

	response := annotations.Annotations{Annotations: augmentedAnnotations}
	json.NewEncoder(w).Encode(&response)
}

func handleErrors(err error, readLog *log.Entry, w http.ResponseWriter) {
	if isTimeoutErr(err) {
		readLog.WithError(err).Error("Timeout while reading annotations.")
		writeMessage(w, "Timeout while reading annotations", http.StatusGatewayTimeout)
		return
	}

	switch err {
	case errUPPBadRequest:
		readLog.Info("UPP responded with a client error, forwarding UPP response back to client.")
	case errNoAnnotations:
		writeMessage(w, "No annotations found", http.StatusNotFound)
	case errUPPServiceUnavailable:
		writeMessage(w, "Service unavailable", http.StatusServiceUnavailable)
	default:
		writeMessage(w, fmt.Sprintf("Failed to read annotations: %v", err), http.StatusInternalServerError)
	}
}

func (h *Handler) readAnnotations(ctx context.Context, w http.ResponseWriter, contentUUID string) ([]annotations.Annotation, error) {
	rwAnnotations, hash, found, err := h.annotationsRW.Read(ctx, contentUUID)

	if err != nil {
		return nil, err
	}

	if !found {
		return h.readAnnotationsFromUPP(ctx, w, contentUUID)
	}

	w.Header().Set(annotations.DocumentHashHeader, hash)
	return rwAnnotations.Annotations, nil
}

func readLogEntry(ctx context.Context, contentUUID string) *log.Entry {
	tid, _ := tidutils.GetTransactionIDFromContext(ctx)
	return log.WithField(tidutils.TransactionIDKey, tid).WithField("uuid", contentUUID)
}

func (h *Handler) readAnnotationsFromUPP(ctx context.Context, w http.ResponseWriter, contentUUID string) ([]annotations.Annotation, error) {
	readLog := readLogEntry(ctx, contentUUID)
	readLog.Info("Annotations not found, retrieving annotations from UPP")

	uppResponse, err := h.annotationsAPI.Get(ctx, contentUUID)

	if err != nil {
		return nil, err
	}

	defer uppResponse.Body.Close()

	if uppResponse.StatusCode != http.StatusOK {
		if uppResponse.StatusCode == http.StatusNotFound || uppResponse.StatusCode == http.StatusBadRequest {
			w.WriteHeader(uppResponse.StatusCode)
			io.Copy(w, uppResponse.Body)
			return nil, errUPPBadRequest
		}

		return nil, errUPPServiceUnavailable
	}

	respBody, _ := ioutil.ReadAll(uppResponse.Body)
	convertedBody, err := mapper.ConvertPredicates(respBody)
	if err != nil {
		return nil, errors.New("Failed to map predicates from UPP response")
	}

	if err == nil && convertedBody == nil {
		return nil, errNoAnnotations
	}

	rawAnnotations := []annotations.Annotation{}
	json.Unmarshal(convertedBody, &rawAnnotations)

	return rawAnnotations, nil
}

func (h *Handler) WriteAnnotations(w http.ResponseWriter, r *http.Request) {
	w.Header().Add("Content-Type", "application/json")

	contentUUID := vestigo.Param(r, "uuid")
	tID := tidutils.GetTransactionIDFromRequest(r)
	ctx := tidutils.TransactionAwareContext(context.Background(), tID)

	oldHash := r.Header.Get(annotations.PreviousDocumentHashHeader)

	writeLog := log.WithField(tidutils.TransactionIDKey, tID).WithField("uuid", contentUUID)

	if err := validateUUID(contentUUID); err != nil {
		writeLog.WithError(err).Error("Invalid content UUID")
		writeMessage(w, fmt.Sprintf("Invalid content UUID: %v", contentUUID), http.StatusBadRequest)
		return
	}

	var draftAnnotations annotations.Annotations
	err := json.NewDecoder(r.Body).Decode(&draftAnnotations)
	if err != nil {
		writeLog.WithError(err).Error("Unable to unmarshal annotations body")
		writeMessage(w, fmt.Sprintf("Unable to unmarshal annotations body: %v", err.Error()), http.StatusBadRequest)
		return
	}

	writeLog.Info("Canonicalizing annotations...")
	draftAnnotations.Annotations = h.c14n.Canonicalize(draftAnnotations.Annotations)

	writeLog.Info("Writing to annotations RW...")
	newHash, err := h.annotationsRW.Write(ctx, contentUUID, &draftAnnotations, oldHash)

	if isTimeoutErr(err) {
		writeLog.WithError(err).Error("Timeout while waiting to write draft annotations.")
		writeMessage(w, "Timeout while waiting to write draft annotations", http.StatusGatewayTimeout)
		return
	}

	if err != nil {
		writeLog.WithError(err).Error("Error in writing draft annotations")
		writeMessage(w, fmt.Sprintf("Error in writing draft annotations: %v", err.Error()), http.StatusInternalServerError)
		return
	}

	w.Header().Set(annotations.DocumentHashHeader, newHash)
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(draftAnnotations)
}

func isTimeoutErr(err error) bool {
	netErr, ok := err.(net.Error)
	return ok && netErr.Timeout()
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
		log.WithError(err).Error("Failed to parse provided message to json, this is a bug.")
		return
	}

	w.Write(j)
}
