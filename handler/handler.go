package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Financial-Times/draft-annotations-api/annotations"
	"github.com/Financial-Times/draft-annotations-api/mapper"
	tidutils "github.com/Financial-Times/transactionid-utils-go"
	"github.com/google/uuid"
	"github.com/husobee/vestigo"
	log "github.com/sirupsen/logrus"
)

// AnnotationsAPI interface encapsulates logic for getting published annotations from API
type AnnotationsAPI interface {
	GetAll(context.Context, string) ([]annotations.Annotation, error)
	GetAllButV2(context.Context, string) ([]annotations.Annotation, error)
}

// Interface for the annotations augmenter (currently only functionality in the annotations package)
type Augmenter interface {
	AugmentAnnotations(ctx context.Context, depletedAnnotations []annotations.Annotation) ([]annotations.Annotation, error)
}

// Handler provides endpoints for reading annotations - draft or published, and writing draft annotations.
type Handler struct {
	annotationsRW        annotations.RW
	annotationsAPI       AnnotationsAPI
	c14n                 *annotations.Canonicalizer
	annotationsAugmenter Augmenter
	timeout              time.Duration
}

// New initializes Handler.
func New(rw annotations.RW, annotationsAPI AnnotationsAPI, c14n *annotations.Canonicalizer, augmenter Augmenter, httpTimeout time.Duration) *Handler {
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
	conceptID := mapper.TransformConceptID("/" + vestigo.Param(r, "cuuid"))

	tID := tidutils.GetTransactionIDFromRequest(r)
	ctx := tidutils.TransactionAwareContext(context.Background(), tID)
	writeLog := log.WithField(tidutils.TransactionIDKey, tID).WithField("uuid", contentUUID)

	oldHash := r.Header.Get(annotations.PreviousDocumentHashHeader)

	writeLog.Debug("Validating input and reading annotations from UPP...")
	uppList, httpStatus, err := h.prepareUPPAnnotations(ctx, contentUUID, conceptID)
	if err != nil {
		handleWriteErrors("Error while preparing annotations", err, writeLog, w, httpStatus)
		return
	}

	i := 0
	for _, item := range uppList {
		if item.ConceptId == conceptID {
			continue
		}
		uppList[i] = item
		i++
	}
	uppList = uppList[:i]

	_, newHash, err := h.saveAndReturnAnnotations(ctx, uppList, writeLog, oldHash, contentUUID)
	if err != nil {
		handleWriteErrors("Error writing draft annotations", err, writeLog, w, http.StatusInternalServerError)
		return
	}

	w.Header().Set(annotations.DocumentHashHeader, newHash)
}

// AddAnnotation adds an annotation for a specific content uuid.
// It gets the annotations only from UPP skipping V2 annotations because they are not editorially curated.
func (h *Handler) AddAnnotation(w http.ResponseWriter, r *http.Request) {
	w.Header().Add("Content-Type", "application/json")

	contentUUID := vestigo.Param(r, "uuid")

	tID := tidutils.GetTransactionIDFromRequest(r)
	ctx := tidutils.TransactionAwareContext(context.Background(), tID)
	writeLog := log.WithField(tidutils.TransactionIDKey, tID).WithField("uuid", contentUUID)

	oldHash := r.Header.Get(annotations.PreviousDocumentHashHeader)

	addedAnnotation := annotations.Annotation{}
	err := json.NewDecoder(r.Body).Decode(&addedAnnotation)
	if err != nil {
		handleWriteErrors("Error decoding request body", err, writeLog, w, http.StatusBadRequest)
		return
	}

	if !mapper.IsValidPACPredicate(addedAnnotation.Predicate) {
		handleWriteErrors("Invalid request", errors.New("invalid predicate"), writeLog, w, http.StatusBadRequest)
		return
	}

	writeLog.Debug("Validating input and reading annotations from UPP...")
	uppList, httpStatus, err := h.prepareUPPAnnotations(ctx, contentUUID, addedAnnotation.ConceptId)
	if err != nil {
		handleWriteErrors("Error while preparing annotations", err, writeLog, w, httpStatus)
		return
	}

	var isFound = false
	for _, item := range uppList {
		if addedAnnotation.ConceptId == item.ConceptId && addedAnnotation.Predicate == item.Predicate {
			writeLog.Debug("Annotation is already in list")
			isFound = true
			break
		}
	}
	if !isFound {
		uppList = append(uppList, addedAnnotation)
	}

	_, newHash, err := h.saveAndReturnAnnotations(ctx, uppList, writeLog, oldHash, contentUUID)
	if err != nil {
		handleWriteErrors("Error writing draft annotations", err, writeLog, w, http.StatusInternalServerError)
		return
	}

	w.Header().Set(annotations.DocumentHashHeader, newHash)
}

// ReadAnnotations gets the annotations for a given content uuid.
// If there are draft annotations, they are returned, otherwise the published annotations are returned.
func (h *Handler) ReadAnnotations(w http.ResponseWriter, r *http.Request) {
	contentUUID := vestigo.Param(r, "uuid")
	tID := tidutils.GetTransactionIDFromRequest(r)

	ctx, cancel := context.WithTimeout(tidutils.TransactionAwareContext(r.Context(), tID), h.timeout)
	defer cancel()

	readLog := readLogEntry(ctx, contentUUID)

	w.Header().Add("Content-Type", "application/json")

	showHasBrand := false
	var err error
	queryParam := r.URL.Query().Get("sendHasBrand")
	if queryParam != "" {
		showHasBrand, err = strconv.ParseBool(queryParam)
		if err != nil {
			writeMessage(w, fmt.Sprintf("invalid param sendHasBrand: %s ", queryParam), http.StatusBadRequest)
			return
		}
	}

	result, hash, err := h.readAnnotations(ctx, contentUUID, showHasBrand, readLog)
	if err != nil {
		handleReadErrors(err, readLog, w)
		return
	}
	if hash != "" {
		w.Header().Set(annotations.DocumentHashHeader, hash)
	}

	response := annotations.Annotations{Annotations: result}
	err = json.NewEncoder(w).Encode(&response)
	if err != nil {
		readLog.WithError(err).Error("Failed to encode response")
		handleReadErrors(err, readLog, w)
	}
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

	savedAnnotations, newHash, err := h.saveAndReturnAnnotations(ctx, draftAnnotations.Annotations, writeLog, oldHash, contentUUID)
	if err != nil {
		handleWriteErrors("Error writing draft annotations", err, writeLog, w, http.StatusInternalServerError)
		return
	}

	w.Header().Set(annotations.DocumentHashHeader, newHash)

	err = json.NewEncoder(w).Encode(savedAnnotations)
	if err != nil {
		handleWriteErrors("Error in encoding draft annotations response", err, writeLog, w, http.StatusInternalServerError)
		return
	}
}

// ReplaceAnnotation deletes an annotation for a specific content uuid and adds a new one.
// It gets the annotations only from UPP skipping V2 annotations because they are not editorially curated.
func (h *Handler) ReplaceAnnotation(w http.ResponseWriter, r *http.Request) {
	w.Header().Add("Content-Type", "application/json")

	contentUUID := vestigo.Param(r, "uuid")
	conceptUUID := vestigo.Param(r, "cuuid")

	tID := tidutils.GetTransactionIDFromRequest(r)
	ctx := tidutils.TransactionAwareContext(context.Background(), tID)

	writeLog := log.WithField(tidutils.TransactionIDKey, tID).WithField("uuid", contentUUID)
	oldHash := r.Header.Get(annotations.PreviousDocumentHashHeader)

	if err := validateUUID(conceptUUID); err != nil {
		handleWriteErrors("invalid concept UUID", err, writeLog, w, http.StatusBadRequest)
		return
	}

	conceptUUID = mapper.TransformConceptID("/" + vestigo.Param(r, "cuuid"))

	addedAnnotation := annotations.Annotation{}
	dec := json.NewDecoder(r.Body)
	err := dec.Decode(&addedAnnotation)
	if err != nil {
		handleWriteErrors("Error decoding request body", err, writeLog, w, http.StatusBadRequest)
		return
	}
	if addedAnnotation.Predicate != "" {
		if !mapper.IsValidPACPredicate(addedAnnotation.Predicate) {
			handleWriteErrors("Invalid request", errors.New("invalid predicate"), writeLog, w, http.StatusBadRequest)
			return
		}
	}
	writeLog.Debug("Validating input and reading annotations from UPP...")
	uppList, httpStatus, err := h.prepareUPPAnnotations(ctx, contentUUID, addedAnnotation.ConceptId)
	if err != nil {
		handleWriteErrors("Error while preparing annotations", err, writeLog, w, httpStatus)
		return
	}

	for i := range uppList {
		if uppList[i].ConceptId == conceptUUID {
			uppList[i].ConceptId = addedAnnotation.ConceptId
			if addedAnnotation.Predicate != "" {
				uppList[i].Predicate = addedAnnotation.Predicate
			}
		}
	}

	_, newHash, err := h.saveAndReturnAnnotations(ctx, uppList, writeLog, oldHash, contentUUID)
	if err != nil {
		handleWriteErrors("Error writing draft annotations", err, writeLog, w, http.StatusInternalServerError)
		return
	}

	w.Header().Set(annotations.DocumentHashHeader, newHash)
}

func (h *Handler) prepareUPPAnnotations(ctx context.Context, contentUUID string, conceptID string) ([]annotations.Annotation, int, error) {

	if err := validateUUID(contentUUID); err != nil {
		return nil, http.StatusBadRequest, fmt.Errorf("invalid content ID : %w", err)
	}

	if conceptID != mapper.TransformConceptID(conceptID) {
		return nil, http.StatusBadRequest, errors.New("invalid concept ID URI")
	}
	i := strings.LastIndex(conceptID, "/")
	if i == -1 || i == len(conceptID)-1 {
		return nil, http.StatusBadRequest, errors.New("concept ID is empty")
	}
	if err := validateUUID(conceptID[i+1:]); err != nil {
		return nil, http.StatusBadRequest, fmt.Errorf("invalid concept ID : %w", err)
	}

	ann, err := h.annotationsAPI.GetAllButV2(ctx, contentUUID)
	if err != nil {
		var uppErr annotations.UPPError
		if errors.As(err, &uppErr) && uppErr.Status() == http.StatusNotFound {
			return nil, uppErr.Status(), err
		}
		return nil, http.StatusInternalServerError, err
	}
	return ann, http.StatusOK, nil
}

func (h *Handler) saveAndReturnAnnotations(ctx context.Context, uppList []annotations.Annotation, writeLog *log.Entry, oldHash string, contentUUID string) (*annotations.Annotations, string, error) {
	writeLog.Debug("Move to HasBrand annotations...")
	uppList, err := h.annotationsAugmenter.AugmentAnnotations(ctx, uppList)
	if err != nil {
		return nil, "", err
	}
	uppList, err = switchToHasBrand(uppList)
	if err != nil {
		return nil, "", err
	}
	writeLog.Debug("Canonicalizing annotations...")
	uppList = h.c14n.Canonicalize(uppList)
	writeLog.Debug("Writing to annotations RW...")
	newAnnotations := &annotations.Annotations{Annotations: uppList}
	newHash, err := h.annotationsRW.Write(ctx, contentUUID, newAnnotations, oldHash)
	if err != nil {
		return nil, "", err
	}
	return newAnnotations, newHash, nil
}

func (h *Handler) readAnnotations(ctx context.Context, contentUUID string, showHasBrand bool, readLog *log.Entry) ([]annotations.Annotation, string, error) {
	var (
		result        []annotations.Annotation
		hash          string
		hasDraft      bool
		err           error
		rwAnnotations *annotations.Annotations
	)
	readLog.Info("Reading Annotations from Annotations R/W")
	rwAnnotations, hash, hasDraft, err = h.annotationsRW.Read(ctx, contentUUID)

	if err != nil {
		return nil, hash, err
	}

	if hasDraft {
		result = rwAnnotations.Annotations
	} else {
		readLog.Info("Annotations not found, retrieving annotations from UPP")
		result, err = h.annotationsAPI.GetAll(ctx, contentUUID)
		if err != nil {
			return nil, hash, err
		}
	}
	readLog.Info("Augmenting annotations with recent UPP data")
	result, err = h.annotationsAugmenter.AugmentAnnotations(ctx, result)
	if err != nil {
		readLog.WithError(err).Error("Failed to augment annotations")
		return nil, hash, err
	}

	if !showHasBrand {
		result = switchToIsClassifiedBy(result)
	}

	return result, hash, err
}

func handleReadErrors(err error, readLog *log.Entry, w http.ResponseWriter) {
	if isTimeoutErr(err) {
		readLog.WithError(err).Error("Timeout while reading annotations.")
		writeMessage(w, "Timeout while reading annotations", http.StatusGatewayTimeout)
		return
	}
	var uppErr annotations.UPPError
	if errors.As(err, &uppErr) {
		if uppErr.UPPBody() != nil {
			readLog.WithError(err).Error("UPP responded with a client error, forwarding UPP response back to client.")
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

func readLogEntry(ctx context.Context, contentUUID string) *log.Entry {
	tid, _ := tidutils.GetTransactionIDFromContext(ctx)
	return log.WithField(tidutils.TransactionIDKey, tid).WithField("uuid", contentUUID)
}

func isTimeoutErr(err error) bool {
	var e net.Error
	if !errors.As(err, &e) {
		return false
	}
	return e.Timeout()
}

func validateUUID(u string) error {
	_, err := uuid.Parse(u)
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

func switchToHasBrand(toChange []annotations.Annotation) ([]annotations.Annotation, error) {
	changed := make([]annotations.Annotation, len(toChange))
	for idx, ann := range toChange {
		// We have removed Predicate and Type validation here.
		// Validating not the user input but the saved annotations can (and did) cause unexpected client errors.
		// To ensure we have only valid predicates we are adding filtering in the augmenter.
		if ann.Predicate == mapper.PredicateIsClassifiedBy && ann.Type == mapper.ConceptTypeBrand {
			ann.Predicate = mapper.PredicateHasBrand
		}

		changed[idx] = ann
	}

	return changed, nil
}

func switchToIsClassifiedBy(toChange []annotations.Annotation) []annotations.Annotation {
	changed := make([]annotations.Annotation, len(toChange))
	for idx, ann := range toChange {
		if ann.Predicate == mapper.PredicateHasBrand {
			ann.Predicate = mapper.PredicateIsClassifiedBy
		}
		changed[idx] = ann
	}
	return changed
}
