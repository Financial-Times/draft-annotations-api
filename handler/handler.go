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

	"github.com/Financial-Times/go-logger/v2"
	"github.com/gorilla/mux"

	"github.com/Financial-Times/draft-annotations-api/annotations"
	"github.com/Financial-Times/draft-annotations-api/mapper"
	tidutils "github.com/Financial-Times/transactionid-utils-go"
	"github.com/google/uuid"
)

const SchemaNameHeader = "Schema-Name"

// AnnotationsAPI interface encapsulates logic for getting published annotations from API
type AnnotationsAPI interface {
	GetAll(context.Context, string) ([]interface{}, error)
	GetAllButV2(context.Context, string) ([]interface{}, error)
}

// Interface for the annotations augmenter (currently only functionality in the annotations package)
type Augmenter interface {
	AugmentAnnotations(ctx context.Context, depletedAnnotations []interface{}) ([]interface{}, error)
}

// Interface for json validator
type jsonValidator interface {
	ValidateByAPI(interface{}, string, string, []interface{}) error
	ValidateBySchema(content interface{}, schemaName string) (err error)
}

// Handler provides endpoints for reading annotations - draft or published, and writing draft annotations.
type Handler struct {
	annotationsRW        annotations.RW
	annotationsAPI       AnnotationsAPI
	c14n                 *annotations.Canonicalizer
	annotationsAugmenter Augmenter
	validator            jsonValidator
	timeout              time.Duration
	log                  *logger.UPPLogger
}

// New initializes Handler.
func New(rw annotations.RW, annotationsAPI AnnotationsAPI, c14n *annotations.Canonicalizer, augmenter Augmenter, validator jsonValidator, httpTimeout time.Duration, log *logger.UPPLogger) *Handler {
	return &Handler{
		rw,
		annotationsAPI,
		c14n,
		augmenter,
		validator,
		httpTimeout,
		log,
	}
}

// DeleteAnnotation deletes a given annotation for a given content uuid.
// It gets the annotations only from UPP skipping V2 annotations because they are not editorially curated.
func (h *Handler) DeleteAnnotation(w http.ResponseWriter, r *http.Request) {
	w.Header().Add("Content-Type", "application/json")

	contentUUID := mux.Vars(r)["uuid"]
	conceptID := mapper.TransformConceptID("/" + mux.Vars(r)["cuuid"])

	tID := tidutils.GetTransactionIDFromRequest(r)
	ctx := tidutils.TransactionAwareContext(context.Background(), tID)
	writeLog := h.log.WithTransactionID(tID).WithUUID(contentUUID)

	oldHash := r.Header.Get(annotations.PreviousDocumentHashHeader)
	schemaVersion := r.Header.Get(annotations.SchemaVersionHeader)
	if schemaVersion == "" {
		schemaVersion = annotations.DefaultSchemaVersion
	}
	ctx = context.WithValue(ctx, annotations.SchemaVersionHeaderKey(annotations.SchemaVersionHeader), schemaVersion)

	origin := r.Header.Get(annotations.OriginSystemIDHeader)
	if origin == "" {
		handleWriteErrors("Invalid request", errors.New("X-Origin-System-Id header missing"), writeLog, w, http.StatusBadRequest)
		return
	}
	ctx = context.WithValue(ctx, annotations.OriginSystemIDHeaderKey(annotations.OriginSystemIDHeader), origin)

	writeLog.Debug("Validating input and reading annotations from UPP...")
	uppList, httpStatus, err := h.prepareUPPAnnotations(ctx, contentUUID, conceptID)
	if err != nil {
		handleWriteErrors("Error while preparing annotations", err, writeLog, w, httpStatus)
		return
	}

	i := 0
	for _, item := range uppList {
		if item.(map[string]interface{})["id"] == conceptID {
			continue
		}
		uppList[i] = item
		i++
	}
	uppList = uppList[:i]

	annotationsBody := make(map[string]interface{})
	annotationsBody["annotations"] = uppList
	_, newHash, err := h.saveAndReturnAnnotations(ctx, annotationsBody, writeLog, oldHash, contentUUID)
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

	contentUUID := mux.Vars(r)["uuid"]

	tID := tidutils.GetTransactionIDFromRequest(r)
	ctx := tidutils.TransactionAwareContext(context.Background(), tID)
	writeLog := h.log.WithTransactionID(tID).WithUUID(contentUUID)

	oldHash := r.Header.Get(annotations.PreviousDocumentHashHeader)
	schemaVersion := r.Header.Get(annotations.SchemaVersionHeader)
	if schemaVersion == "" {
		schemaVersion = annotations.DefaultSchemaVersion
	}
	ctx = context.WithValue(ctx, annotations.SchemaVersionHeaderKey(annotations.SchemaVersionHeader), schemaVersion)

	origin := r.Header.Get(annotations.OriginSystemIDHeader)
	if origin == "" {
		handleWriteErrors("Invalid request", errors.New("X-Origin-System-Id header missing"), writeLog, w, http.StatusBadRequest)
		return
	}
	ctx = context.WithValue(ctx, annotations.OriginSystemIDHeaderKey(annotations.OriginSystemIDHeader), origin)

	var addedAnnotationBody map[string]interface{}
	err := json.NewDecoder(r.Body).Decode(&addedAnnotationBody)
	if err != nil {
		handleWriteErrors("Error decoding request body", err, writeLog, w, http.StatusBadRequest)
		return
	}

	var publication []interface{}
	var ok bool
	if v, found := addedAnnotationBody["publication"]; found {
		if publication, ok = v.([]interface{}); !ok {
			handleWriteErrors("Invalid request", errors.New("publication is not in correct format"), writeLog, w, http.StatusBadRequest)
			return
		}
	} else {
		handleWriteErrors("Invalid request", errors.New("publication is missing"), writeLog, w, http.StatusBadRequest)
		return
	}

	err = h.validator.ValidateByAPI(addedAnnotationBody, r.Method, r.RequestURI, publication)
	if err != nil {
		handleWriteErrors("Failed to validate request body", err, writeLog, w, http.StatusBadRequest)
		return
	}

	addedAnnotation := addedAnnotationBody["annotation"].(map[string]interface{})
	if origin == annotations.PACOriginSystemID && !mapper.IsValidPACPredicate(addedAnnotation["predicate"].(string)) {
		handleWriteErrors("Invalid request", errors.New("invalid predicate"), writeLog, w, http.StatusBadRequest)
		return
	}

	conceptID := addedAnnotation["id"]
	if conceptID == nil {
		conceptID = ""
	}
	writeLog.Debug("Validating input and reading annotations from UPP...")
	uppList, httpStatus, err := h.prepareUPPAnnotations(ctx, contentUUID, conceptID.(string))
	if err != nil {
		handleWriteErrors("Error while preparing annotations", err, writeLog, w, httpStatus)
		return
	}

	var isFound = false
	for _, item := range uppList {
		ann := item.(map[string]interface{})
		if addedAnnotation["id"] == ann["id"] && addedAnnotation["predicate"] == ann["predicate"] {
			writeLog.Debug("Annotation is already in list")
			isFound = true
			break
		}
	}
	if !isFound {
		uppList = append(uppList, addedAnnotation)
	}

	annotationsBody := make(map[string]interface{})
	annotationsBody["annotations"] = uppList
	annotationsBody["publication"] = addedAnnotationBody["publication"]
	_, newHash, err := h.saveAndReturnAnnotations(ctx, annotationsBody, writeLog, oldHash, contentUUID)
	if err != nil {
		handleWriteErrors("Error writing draft annotations", err, writeLog, w, http.StatusInternalServerError)
		return
	}

	w.Header().Set(annotations.DocumentHashHeader, newHash)
}

// ReadAnnotations gets the annotations for a given content uuid.
// If there are draft annotations, they are returned, otherwise the published annotations are returned.
func (h *Handler) ReadAnnotations(w http.ResponseWriter, r *http.Request) {
	contentUUID := mux.Vars(r)["uuid"]
	tID := tidutils.GetTransactionIDFromRequest(r)
	readLog := h.log.WithTransactionID(tID).WithUUID(contentUUID)

	ctx, cancel := context.WithTimeout(tidutils.TransactionAwareContext(r.Context(), tID), h.timeout)
	defer cancel()

	origin := r.Header.Get(annotations.OriginSystemIDHeader)
	if origin == "" {
		writeMessage(w, readLog, "X-Origin-System-Id header missing", http.StatusBadRequest)
		return
	}
	ctx = context.WithValue(ctx, annotations.OriginSystemIDHeaderKey(annotations.OriginSystemIDHeader), origin)

	w.Header().Add("Content-Type", "application/json")

	showHasBrand := false
	var err error
	queryParam := r.URL.Query().Get("sendHasBrand")
	if queryParam != "" {
		showHasBrand, err = strconv.ParseBool(queryParam)
		if err != nil {
			writeMessage(w, readLog, fmt.Sprintf("invalid param sendHasBrand: %s ", queryParam), http.StatusBadRequest)
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

	err = json.NewEncoder(w).Encode(&result)
	if err != nil {
		readLog.WithError(err).Error("Failed to encode response")
		handleReadErrors(err, readLog, w)
	}
}

// WriteAnnotations writes draft annotations for given content.
func (h *Handler) WriteAnnotations(w http.ResponseWriter, r *http.Request) {
	w.Header().Add("Content-Type", "application/json")

	contentUUID := mux.Vars(r)["uuid"]
	tID := tidutils.GetTransactionIDFromRequest(r)
	ctx := tidutils.TransactionAwareContext(context.Background(), tID)

	oldHash := r.Header.Get(annotations.PreviousDocumentHashHeader)
	schemaVersion := r.Header.Get(annotations.SchemaVersionHeader)
	if schemaVersion == "" {
		schemaVersion = annotations.DefaultSchemaVersion
	}
	ctx = context.WithValue(ctx, annotations.SchemaVersionHeaderKey(annotations.SchemaVersionHeader), schemaVersion)

	writeLog := h.log.WithTransactionID(tID).WithUUID(contentUUID)

	origin := r.Header.Get(annotations.OriginSystemIDHeader)
	if origin == "" {
		handleWriteErrors("Invalid request", errors.New("X-Origin-System-Id header missing"), writeLog, w, http.StatusBadRequest)
		return
	}
	ctx = context.WithValue(ctx, annotations.OriginSystemIDHeaderKey(annotations.OriginSystemIDHeader), origin)

	if err := validateUUID(contentUUID); err != nil {
		handleWriteErrors("Invalid content UUID", err, writeLog, w, http.StatusBadRequest)
		return
	}

	var draftAnnotationsBody map[string]interface{}
	err := json.NewDecoder(r.Body).Decode(&draftAnnotationsBody)
	if err != nil {
		handleWriteErrors("Unable to unmarshal annotations body", err, writeLog, w, http.StatusBadRequest)
		return
	}

	var publication []interface{}
	var ok bool
	if v, found := draftAnnotationsBody["publication"]; found {
		if publication, ok = v.([]interface{}); !ok {
			handleWriteErrors("Invalid request", errors.New("publication is not in correct format"), writeLog, w, http.StatusBadRequest)
			return
		}
	} else {
		handleWriteErrors("Invalid request", errors.New("publication is missing"), writeLog, w, http.StatusBadRequest)
		return
	}

	err = h.validator.ValidateByAPI(draftAnnotationsBody, r.Method, r.RequestURI, publication)
	if err != nil {
		handleWriteErrors("Failed to validate request body", err, writeLog, w, http.StatusBadRequest)
		return
	}

	savedAnnotations, newHash, err := h.saveAndReturnAnnotations(ctx, draftAnnotationsBody, writeLog, oldHash, contentUUID)
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

	contentUUID := mux.Vars(r)["uuid"]
	conceptUUID := mux.Vars(r)["cuuid"]

	tID := tidutils.GetTransactionIDFromRequest(r)
	ctx := tidutils.TransactionAwareContext(context.Background(), tID)

	writeLog := h.log.WithTransactionID(tID).WithUUID(contentUUID)
	oldHash := r.Header.Get(annotations.PreviousDocumentHashHeader)
	schemaVersion := r.Header.Get(annotations.SchemaVersionHeader)
	if schemaVersion == "" {
		schemaVersion = annotations.DefaultSchemaVersion
	}
	ctx = context.WithValue(ctx, annotations.SchemaVersionHeaderKey(annotations.SchemaVersionHeader), schemaVersion)

	origin := r.Header.Get(annotations.OriginSystemIDHeader)
	if origin == "" {
		handleWriteErrors("Invalid request", errors.New("X-Origin-System-Id header missing"), writeLog, w, http.StatusBadRequest)
		return
	}
	ctx = context.WithValue(ctx, annotations.OriginSystemIDHeaderKey(annotations.OriginSystemIDHeader), origin)

	if err := validateUUID(conceptUUID); err != nil {
		handleWriteErrors("invalid concept UUID", err, writeLog, w, http.StatusBadRequest)
		return
	}

	conceptUUID = mapper.TransformConceptID("/" + mux.Vars(r)["cuuid"])

	addedAnnotationBody := map[string]interface{}{}
	dec := json.NewDecoder(r.Body)
	err := dec.Decode(&addedAnnotationBody)
	if err != nil {
		handleWriteErrors("Error decoding request body", err, writeLog, w, http.StatusBadRequest)
		return
	}

	var publication []interface{}
	var ok bool
	if v, found := addedAnnotationBody["publication"]; found {
		if publication, ok = v.([]interface{}); !ok {
			handleWriteErrors("Invalid request", errors.New("publication is not in correct format"), writeLog, w, http.StatusBadRequest)
			return
		}
	} else {
		handleWriteErrors("Invalid request", errors.New("publication is missing"), writeLog, w, http.StatusBadRequest)
		return
	}

	err = h.validator.ValidateByAPI(addedAnnotationBody, r.Method, r.RequestURI, publication)
	if err != nil {
		handleWriteErrors("Failed to validate request body", err, writeLog, w, http.StatusBadRequest)
		return
	}

	addedAnnotation := addedAnnotationBody["annotation"].(map[string]interface{})
	if addedAnnotation["predicate"] != nil {
		if origin == annotations.PACOriginSystemID && !mapper.IsValidPACPredicate(addedAnnotation["predicate"].(string)) {
			handleWriteErrors("Invalid request", errors.New("invalid predicate"), writeLog, w, http.StatusBadRequest)
			return
		}
	}
	writeLog.Debug("Validating input and reading annotations from UPP...")
	uppList, httpStatus, err := h.prepareUPPAnnotations(ctx, contentUUID, addedAnnotation["id"].(string))
	if err != nil {
		handleWriteErrors("Error while preparing annotations", err, writeLog, w, httpStatus)
		return
	}

	for i := range uppList {
		ann := uppList[i].(map[string]interface{})
		if ann["id"] == conceptUUID {
			ann["id"] = addedAnnotation["id"]
			if addedAnnotation["predicate"] != nil {
				ann["predicate"] = addedAnnotation["predicate"]
			}
		}
	}

	annotationsBody := make(map[string]interface{})
	annotationsBody["annotations"] = uppList
	annotationsBody["publication"] = addedAnnotationBody["publication"]
	_, newHash, err := h.saveAndReturnAnnotations(ctx, annotationsBody, writeLog, oldHash, contentUUID)
	if err != nil {
		handleWriteErrors("Error writing draft annotations", err, writeLog, w, http.StatusInternalServerError)
		return
	}

	w.Header().Set(annotations.DocumentHashHeader, newHash)
}

// Validate request body against the available schemas
func (h *Handler) Validate(w http.ResponseWriter, r *http.Request) {
	tID := tidutils.GetTransactionIDFromRequest(r)
	validateLog := h.log.WithTransactionID(tID)

	schemaName := r.Header.Get(SchemaNameHeader)
	if schemaName == "" {
		handleWriteErrors("Invalid request", errors.New("Schema-Name header missing"), validateLog, w, http.StatusBadRequest)
		return
	}

	requestBody := map[string]interface{}{}
	dec := json.NewDecoder(r.Body)
	err := dec.Decode(&requestBody)
	if err != nil {
		handleWriteErrors("Error decoding request body", err, validateLog, w, http.StatusBadRequest)
		return
	}

	err = h.validator.ValidateBySchema(requestBody, schemaName)
	if err != nil {
		handleWriteErrors("Failed to validate request body", err, validateLog, w, http.StatusBadRequest)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (h *Handler) prepareUPPAnnotations(ctx context.Context, contentUUID string, conceptID string) ([]interface{}, int, error) {
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

func (h *Handler) saveAndReturnAnnotations(ctx context.Context, uppList map[string]interface{}, writeLog *logger.LogEntry, oldHash string, contentUUID string) (map[string]interface{}, string, error) {
	writeLog.Debug("Move to HasBrand annotations...")
	var err error
	anns, ok := uppList["annotations"].([]interface{})
	if !ok {
		return nil, "", errors.New("invalid annotations representation")
	}
	anns, err = h.annotationsAugmenter.AugmentAnnotations(ctx, anns)
	if err != nil {
		return nil, "", err
	}

	origin := ctx.Value(annotations.OriginSystemIDHeaderKey(annotations.OriginSystemIDHeader)).(string)
	if origin == annotations.PACOriginSystemID {
		anns = switchToHasBrand(anns)
	}

	writeLog.Debug("Canonicalizing annotations...")
	anns = h.c14n.Canonicalize(anns)

	writeLog.Debug("Writing to annotations RW...")
	uppList["annotations"] = anns
	newHash, err := h.annotationsRW.Write(ctx, contentUUID, uppList, oldHash)
	if err != nil {
		return nil, "", err
	}
	return uppList, newHash, nil
}

func (h *Handler) readAnnotations(ctx context.Context, contentUUID string, showHasBrand bool, readLog *logger.LogEntry) (map[string]interface{}, string, error) {
	var (
		hash          string
		hasDraft      bool
		err           error
		rwAnnotations map[string]interface{}
	)
	result := make(map[string]interface{})
	readLog.Info("Reading Annotations from Annotations R/W")
	rwAnnotations, hash, hasDraft, err = h.annotationsRW.Read(ctx, contentUUID)

	if err != nil {
		return nil, hash, err
	}

	if hasDraft {
		result = rwAnnotations
	} else {
		readLog.Info("Annotations not found, retrieving annotations from UPP")
		result["annotations"], err = h.annotationsAPI.GetAll(ctx, contentUUID)
		if err != nil {
			return nil, hash, err
		}
	}
	readLog.Info("Augmenting annotations with recent UPP data")
	result["annotations"], err = h.annotationsAugmenter.AugmentAnnotations(ctx, result["annotations"].([]interface{}))
	if err != nil {
		readLog.WithError(err).Error("Failed to augment annotations")
		return nil, hash, err
	}

	if !showHasBrand {
		result["annotations"] = switchToIsClassifiedBy(result["annotations"].([]interface{}))
	}

	return result, hash, err
}

func handleReadErrors(err error, readLog *logger.LogEntry, w http.ResponseWriter) {
	if isTimeoutErr(err) {
		readLog.WithError(err).Error("Timeout while reading annotations.")
		writeMessage(w, readLog, "Timeout while reading annotations", http.StatusGatewayTimeout)
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
		writeMessage(w, readLog, uppErr.Error(), uppErr.Status())
		return
	}
	writeMessage(w, readLog, fmt.Sprintf("Failed to read annotations: %v", err), http.StatusInternalServerError)
}

func handleWriteErrors(msg string, err error, writeLog *logger.LogEntry, w http.ResponseWriter, httpStatus int) {
	msg = fmt.Sprintf(msg+": %v", err.Error())
	if isTimeoutErr(err) {
		msg = "Timeout while waiting to write draft annotations"
		httpStatus = http.StatusGatewayTimeout
	}

	writeLog.WithError(err).Error(msg)
	writeMessage(w, writeLog, msg, httpStatus)
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

func writeMessage(w http.ResponseWriter, logEntry *logger.LogEntry, msg string, status int) {
	w.WriteHeader(status)

	message := make(map[string]interface{})
	message["message"] = msg
	j, err := json.Marshal(&message)
	if err != nil {
		logEntry.WithError(err).Error("Failed to parse provided message to json.")
		return
	}

	_, err = w.Write(j)
	if err != nil {
		logEntry.WithError(err).Error("Failed to parse response message.")
	}
}

func switchToHasBrand(toChange []interface{}) []interface{} {
	changed := make([]interface{}, len(toChange))
	for idx, annotation := range toChange {
		// We have removed Predicate and Type validation here.
		// Validating not the user input but the saved annotations can (and did) cause unexpected client errors.
		// To ensure we have only valid predicates we are adding filtering in the augmenter.
		ann := annotation.(map[string]interface{})
		if ann["predicate"] == mapper.PredicateIsClassifiedBy && ann["type"] == mapper.ConceptTypeBrand {
			ann["predicate"] = mapper.PredicateHasBrand
		}

		changed[idx] = ann
	}

	return changed
}

func switchToIsClassifiedBy(toChange []interface{}) []interface{} {
	changed := make([]interface{}, len(toChange))
	for idx, annotation := range toChange {
		ann := annotation.(map[string]interface{})
		if ann["predicate"] == mapper.PredicateHasBrand {
			ann["predicate"] = mapper.PredicateIsClassifiedBy
		}
		changed[idx] = ann
	}
	return changed
}
