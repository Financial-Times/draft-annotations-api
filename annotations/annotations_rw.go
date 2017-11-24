package annotations

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"

	tidUtils "github.com/Financial-Times/transactionid-utils-go"
	log "github.com/sirupsen/logrus"
)

const rwURLPattern = "%s/drafts/content/%s/annotations"
const DocumentHashHeader = "X-Document-Hash"
const PreviousDocumentHashHeader = "X-Previous-Document-Hash"

type RW interface {
	Read(ctx context.Context, contentUUID string) ([]Annotation, string, bool, error)
	Write(ctx context.Context, contentUUID string, annotations []Annotation, hash string) (string, error)
	Endpoint() string
	GTG() error
}

type annotationsRW struct {
	endpoint   string
	httpClient *http.Client
}

func NewRW(endpoint string) RW {
	return &annotationsRW{endpoint, &http.Client{}}
}

func (rw *annotationsRW) Read(ctx context.Context, contentUUID string) ([]Annotation, string, bool, error) {
	tid, err := tidUtils.GetTransactionIDFromContext(ctx)

	if err != nil {
		tid = tidUtils.NewTransactionID()
		log.WithField(tidUtils.TransactionIDKey, tid).
			WithField("uuid", contentUUID).
			WithError(err).
			Warn("Transaction ID error in getting annotations from RW with concept data: Generated a new transaction ID")
		ctx = tidUtils.TransactionAwareContext(ctx, tid)
	}

	readLog := log.WithField(tidUtils.TransactionIDKey, tid).WithField("uuid", contentUUID)

	req, err := http.NewRequest("GET", fmt.Sprintf(rwURLPattern, rw.endpoint, contentUUID), nil)
	if err != nil {
		readLog.WithError(err).Error("Error in creating the HTTP read request to annotations RW")
		return nil, "", false, err
	}
	req.Header.Set(tidUtils.TransactionIDHeader, tid)

	resp, err := rw.httpClient.Do(req)
	if err != nil {
		readLog.WithError(err).Error("Error making the HTTP read request to annotations RW")
		return nil, "", false, err
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		var annotations []Annotation
		err = json.NewDecoder(resp.Body).Decode(&annotations)
		if err != nil {
			readLog.WithError(err).Error("Error in unmarshalling the HTTP response from annotations RW")
			return nil, "", false, err
		}
		hash := resp.Header.Get(DocumentHashHeader)
		return annotations, hash, true, nil
	case http.StatusNotFound:
		return nil, "", false, nil
	default:
		return nil, "", false, fmt.Errorf("annotations RW returned an unexpected HTTP status code in read operation: %v", resp.StatusCode)
	}
}

func (rw *annotationsRW) Write(ctx context.Context, contentUUID string, annotations []Annotation, hash string) (string, error) {
	tid, err := tidUtils.GetTransactionIDFromContext(ctx)

	if err != nil {
		tid = tidUtils.NewTransactionID()
		log.WithField(tidUtils.TransactionIDKey, tid).
			WithField("uuid", contentUUID).
			WithError(err).
			Warn("Transaction ID error in writing annotations to RW with concept data: Generated a new transaction ID")
		ctx = tidUtils.TransactionAwareContext(ctx, tid)
	}

	writeLog := log.WithField(tidUtils.TransactionIDKey, tid).WithField("uuid", contentUUID)

	annotationsBody, err := json.Marshal(annotations)
	if err != nil {
		writeLog.WithError(err).Error("Unable to marshall annotations that needs to be written")
		return "", err
	}

	req, err := http.NewRequest("PUT", fmt.Sprintf(rwURLPattern, rw.endpoint, contentUUID), bytes.NewBuffer(annotationsBody))
	if err != nil {
		writeLog.WithError(err).Error("Error in creating the HTTP write request to annotations RW")
		return "", err
	}
	req.Header.Set(tidUtils.TransactionIDHeader, tid)
	req.Header.Set(PreviousDocumentHashHeader, hash)

	resp, err := rw.httpClient.Do(req)
	if err != nil {
		writeLog.WithError(err).Error("Error making the HTTP request to annotations RW")
		return "", err
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK, http.StatusCreated:
		newHash := resp.Header.Get(DocumentHashHeader)
		return newHash, nil
	default:
		return "", fmt.Errorf("annotations RW returned an unexpected HTTP status code in write operation: %v", resp.StatusCode)
	}
}

func (rw *annotationsRW) Endpoint() string {
	return rw.endpoint
}

func (rw *annotationsRW) GTG() error {
	req, err := http.NewRequest("GET", rw.endpoint+"/__gtg", nil)
	if err != nil {
		log.WithError(err).Error("Error in creating the HTTP request to annotations RW GTG")
		return fmt.Errorf("gtg HTTP request error: %v", err)
	}

	resp, err := rw.httpClient.Do(req)
	if err != nil {
		log.WithError(err).Error("Error making the HTTP request to annotations RW GTG")
		return fmt.Errorf("gtg HTTP call error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := ioutil.ReadAll(resp.Body)
		return fmt.Errorf("gtg returned unexpected status %v: %v", resp.StatusCode, string(body))
	}

	return nil
}
