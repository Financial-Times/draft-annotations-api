package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/Financial-Times/draft-annotations-api/annotations"
	"github.com/Financial-Times/draft-annotations-api/mapper"
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

// DeleteAnnotation deletes a given annotation for a given content uuid.
// It gets the annotations only from UPP skipping V2 annotations because they are not editorially curated.
func (h *Handler) DeleteAnnotation(w http.ResponseWriter, r *http.Request) {
	w.Header().Add("Content-Type", "application/json")

	contentUUID := vestigo.Param(r, "uuid")
	conceptUUID := vestigo.Param(r, "cuuid")
	tID := tidutils.GetTransactionIDFromRequest(r)
	ctx := tidutils.TransactionAwareContext(context.Background(), tID)

	oldHash := r.Header.Get(annotations.PreviousDocumentHashHeader)

	writeLog := log.WithField(tidutils.TransactionIDKey, tID).WithField("uuid", contentUUID)

	if err := validateUUID(contentUUID); err != nil {
		handleWriteErrors("Invalid content UUID", err, writeLog, w, http.StatusInternalServerError)
		return
	}
	if err := validateUUID(conceptUUID); err != nil {
		handleWriteErrors("Invalid concept UUID", err, writeLog, w, http.StatusInternalServerError)
		return
	}
	conceptUUID = mapper.TransformConceptID("/" + conceptUUID)

	writeLog.Debug("Reading annotations from UPP...")
	uppList, err := h.annotationsAPI.GetAllButV2(ctx, contentUUID)
	if err != nil {
		handleErrors(err, writeLog, w)
		return
	}

	writeLog.Debug("Canonicalizing annotations...")
	uppList = h.c14n.Canonicalize(uppList)

	i := 0
	for _, item := range uppList {
		if item.ConceptId == conceptUUID {
			continue
		}

		uppList[i] = item
		i++
	}
	uppList = uppList[:i]

	writeLog.Debug("Writing to annotations RW...")
	newAnnotations := annotations.Annotations{Annotations: uppList}
	newHash, err := h.annotationsRW.Write(ctx, contentUUID, &newAnnotations, oldHash)
	if err != nil {
		handleWriteErrors("Error in writing draft annotations", err, writeLog, w, http.StatusInternalServerError)
		return
	}

	w.Header().Set(annotations.DocumentHashHeader, newHash)
	err = json.NewEncoder(w).Encode(newAnnotations)
	if err != nil {
		handleWriteErrors("Error in encoding draft annotations response", err, writeLog, w, http.StatusInternalServerError)
		return
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

func handleWriteErrors(msg string, err error, writeLog *log.Entry, w http.ResponseWriter, httpStatus int) {
	msg = fmt.Sprintf(msg+": %v", err.Error())
	if isTimeoutErr(err) {
		msg = "Timeout while waiting to write draft annotations"
		httpStatus = http.StatusGatewayTimeout
	}

	writeLog.WithError(err).Error(msg)
	writeMessage(w, msg, httpStatus)
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
		handleWriteErrors("Invalid content UUID", err, writeLog, w, http.StatusBadRequest)
		return
	}

	var draftAnnotations annotations.Annotations
	err := json.NewDecoder(r.Body).Decode(&draftAnnotations)
	if err != nil {
		handleWriteErrors("Unable to unmarshal annotations body", err, writeLog, w, http.StatusBadRequest)
		return
	}

	writeLog.Debug("Canonicalizing annotations...")
	draftAnnotations.Annotations = h.c14n.Canonicalize(draftAnnotations.Annotations)

	writeLog.Debug("Writing to annotations RW...")
	newHash, err := h.annotationsRW.Write(ctx, contentUUID, &draftAnnotations, oldHash)
	if err != nil {
		handleWriteErrors("Error in writing draft annotations", err, writeLog, w, http.StatusInternalServerError)
		return
	}

	w.Header().Set(annotations.DocumentHashHeader, newHash)
	err = json.NewEncoder(w).Encode(draftAnnotations)
	if err != nil {
		handleWriteErrors("Error in encoding draft annotations response", err, writeLog, w, http.StatusInternalServerError)
		return
	}
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
