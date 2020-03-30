package annotations

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"

	tidUtils "github.com/Financial-Times/transactionid-utils-go"
	log "github.com/sirupsen/logrus"
)

const rwURLPattern = "%s/drafts/content/%s/annotations"
const DocumentHashHeader = "Document-Hash"
const PreviousDocumentHashHeader = "Previous-Document-Hash"

type RW interface {
	Read(ctx context.Context, contentUUID string) (*Annotations, string, bool, error)
	Write(ctx context.Context, contentUUID string, annotations *Annotations, hash string) (string, error)
	Endpoint() string
	GTG() error
}

type annotationsRW struct {
	endpoint   string
	httpClient *http.Client
}

func NewRW(client *http.Client, endpoint string) RW {
	return &annotationsRW{endpoint, client}
}

var ErrUnexpectedStatusRead = errors.New("annotations RW returned an unexpected HTTP status code in read operation")
var ErrUnexpectedStatusWrite = errors.New("annotations RW returned an unexpected HTTP status code in write operation")
var ErrGTGCall = errors.New("gtg HTTP call error")
var ErrGTGRequest = errors.New("gtg HTTP request error")
var ErrGTGUnexpectedStatus = errors.New("gtg returned unexpected status")

func (rw *annotationsRW) Read(ctx context.Context, contentUUID string) (*Annotations, string, bool, error) {
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

	resp, err := rw.httpClient.Do(req.WithContext(ctx))
	if err != nil {
		readLog.WithError(err).Error("Error making the HTTP read request to annotations RW")
		return nil, "", false, err
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		var annotations Annotations
		err = json.NewDecoder(resp.Body).Decode(&annotations)
		if err != nil {
			readLog.WithError(err).Error("Error in unmarshalling the HTTP response from annotations RW")
			return nil, "", false, err
		}
		hash := resp.Header.Get(DocumentHashHeader)
		return &annotations, hash, true, nil
	case http.StatusNotFound:
		return nil, "", false, nil
	default:
		return nil, "", false, fmt.Errorf("status %v: %w", resp.StatusCode, ErrUnexpectedStatusRead)
	}
}

func (rw *annotationsRW) Write(ctx context.Context, contentUUID string, annotations *Annotations, hash string) (string, error) {
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

	req.Header.Set(PreviousDocumentHashHeader, hash)

	resp, err := rw.httpClient.Do(req.WithContext(ctx))
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
		return "", fmt.Errorf("status %v: %w", resp.StatusCode, ErrUnexpectedStatusWrite)
	}
}

func (rw *annotationsRW) Endpoint() string {
	return rw.endpoint
}

func (rw *annotationsRW) GTG() error {
	req, err := http.NewRequest("GET", rw.endpoint+"/__gtg", nil)
	if err != nil {
		log.WithError(err).Error("Error in creating the HTTP request to annotations RW GTG")
		return fmt.Errorf("%w %v", ErrGTGRequest, err)
	}

	resp, err := rw.httpClient.Do(req)
	if err != nil {
		log.WithError(err).Error("Error making the HTTP request to annotations RW GTG")
		return fmt.Errorf("%w %v", ErrGTGCall, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			err = fmt.Errorf("status %v: %w", resp.StatusCode, ErrGTGUnexpectedStatus)
		} else {
			err = fmt.Errorf("status %v %s: %w", resp.StatusCode, string(body), ErrGTGUnexpectedStatus)
		}
		return err
	}

	return nil
}
