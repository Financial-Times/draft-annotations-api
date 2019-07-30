package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/Financial-Times/draft-annotations-api/annotations"
	tidutils "github.com/Financial-Times/transactionid-utils-go"
	"github.com/husobee/vestigo"
	uuid "github.com/satori/go.uuid"
	log "github.com/sirupsen/logrus"
)

// AnnotationsAPI interface encapsulates logic for getting published annotations from API
type AnnotationsAPI interface {
	GetAll(context.Context, string) ([]annotations.Annotation, error)
	GetAllButV2(context.Context, string) ([]annotations.Annotation, error)
}

// Handler provides endpoints for reading annotations - draft or published, and writing draft annotations.
type Handler struct {
	annotationsRW        annotations.RW
	annotationsAPI       AnnotationsAPI
	c14n                 *annotations.Canonicalizer
	annotationsAugmenter annotations.Augmenter
	timeout              time.Duration
}

// New initializes Handler.
func New(rw annotations.RW, annotationsAPI AnnotationsAPI, c14n *annotations.Canonicalizer, augmenter annotations.Augmenter, httpTimeout time.Duration) *Handler {
	return &Handler{
		rw,
		annotationsAPI,
		c14n,
		augmenter,
		httpTimeout,
	}
}

// ReadAnnotations gets the annotations for a given content uuid.
// If there are draft annotations, they are returned, otherwise the published annotations are returned.
func (h *Handler) ReadAnnotations(w http.ResponseWriter, r *http.Request) {
	contentUUID := vestigo.Param(r, "uuid")
	tID := tidutils.GetTransactionIDFromRequest(r)

	ctx, cancel := context.WithTimeout(tidutils.TransactionAwareContext(context.Background(), tID), h.timeout)
	defer cancel()

	readLog := readLogEntry(ctx, contentUUID)

	w.Header().Add("Content-Type", "application/json")

	readLog.Debug("Reading Annotations from Annotations R/W")
	rawAnnotations, err := h.readAnnotations(ctx, w, contentUUID, readLog)
	if err != nil {
		handleErrors(err, readLog, w)
		return
	}

	readLog.Debug("Augmenting annotations with recent UPP data")
	augmentedAnnotations, err := h.annotationsAugmenter.AugmentAnnotations(ctx, rawAnnotations)
	if err != nil {
		readLog.WithError(err).Error("Failed to augment annotations")
		handleErrors(err, readLog, w)
		return
	}

	response := annotations.Annotations{Annotations: augmentedAnnotations}
	err = json.NewEncoder(w).Encode(&response)
	if err != nil {
		readLog.WithError(err).Error("Failed to encode response")
		handleErrors(err, readLog, w)
	}
}

func handleErrors(err error, readLog *log.Entry, w http.ResponseWriter) {
	if isTimeoutErr(err) {
		readLog.WithError(err).Error("Timeout while reading annotations.")
		writeMessage(w, "Timeout while reading annotations", http.StatusGatewayTimeout)
		return
	}

	if uppErr, ok := err.(annotations.UPPError); ok {
		if uppErr.UPPBody() != nil {
			readLog.Info("UPP responded with a client error, forwarding UPP response back to client.")
			w.WriteHeader(uppErr.Status())
			w.Write(uppErr.UPPBody())
			return
		}
		writeMessage(w, uppErr.Error(), uppErr.Status())
		return
	}
	writeMessage(w, fmt.Sprintf("Failed to read annotations: %v", err), http.StatusInternalServerError)
}

func (h *Handler) readAnnotations(ctx context.Context, w http.ResponseWriter, contentUUID string, readLog *log.Entry) ([]annotations.Annotation, error) {
	rwAnnotations, hash, found, err := h.annotationsRW.Read(ctx, contentUUID)

	if err != nil {
		return nil, err
	}

	if !found {
		readLog.Debug("Annotations not found, retrieving annotations from UPP")
		anns, err := h.annotationsAPI.GetAll(ctx, contentUUID)
		return anns, err
	}

	w.Header().Set(annotations.DocumentHashHeader, hash)
	return rwAnnotations.Annotations, nil
}

func readLogEntry(ctx context.Context, contentUUID string) *log.Entry {
	tid, _ := tidutils.GetTransactionIDFromContext(ctx)
	return log.WithField(tidutils.TransactionIDKey, tid).WithField("uuid", contentUUID)
}

// WriteAnnotations writes draft annotations for given content.
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

	writeLog.Debug("Canonicalizing annotations...")
	draftAnnotations.Annotations = h.c14n.Canonicalize(draftAnnotations.Annotations)

	writeLog.Debug("Writing to annotations RW...")
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
	err = json.NewEncoder(w).Encode(draftAnnotations)
	if err != nil {
		writeLog.WithError(err).Error("Error in encoding draft annotations response")
		writeMessage(w, fmt.Sprintf("Error in encoding draft annotations response: %v", err.Error()), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
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

	_, err = w.Write(j)
	if err != nil {
		log.WithError(err).Error("Failed to parse response message.")
	}
}
