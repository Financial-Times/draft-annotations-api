package annotations

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	tidUtils "github.com/Financial-Times/transactionid-utils-go"
	log "github.com/sirupsen/logrus"
)

type SchemaVersionHeaderKey string

const (
	rwURLPattern               = "%s/draft-annotations/%s"
	DocumentHashHeader         = "Document-Hash"
	PreviousDocumentHashHeader = "Previous-Document-Hash"
	SchemaVersionHeader        = "X-Schema-Version"
	DefaultSchemaVersion       = "1"
)

type RW interface {
	Read(ctx context.Context, contentUUID string) (map[string]interface{}, string, bool, error)
	Write(ctx context.Context, contentUUID string, data map[string]interface{}, hash string) (string, error)
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
var ErrGTGNotOK = errors.New("gtg returned a non-200 HTTP status")

func (rw *annotationsRW) Read(ctx context.Context, contentUUID string) (map[string]interface{}, string, bool, error) {
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
		var data map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&data)
		if err != nil {
			readLog.WithError(err).Error("Error in unmarshalling the HTTP response from annotations RW")
			return nil, "", false, err
		}
		hash := resp.Header.Get(DocumentHashHeader)
		return data, hash, true, nil
	case http.StatusNotFound:
		return nil, "", false, nil
	default:
		return nil, "", false, fmt.Errorf("status %d: %w", resp.StatusCode, ErrUnexpectedStatusRead)
	}
}

func (rw *annotationsRW) Write(ctx context.Context, contentUUID string, data map[string]interface{}, hash string) (string, error) {
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

	annotationsBody, err := json.Marshal(data)
	if err != nil {
		writeLog.WithError(err).Error("Unable to marshall annotations that needs to be written")
		return "", err
	}

	req, err := http.NewRequest("POST", fmt.Sprintf(rwURLPattern, rw.endpoint, contentUUID), bytes.NewBuffer(annotationsBody))
	if err != nil {
		writeLog.WithError(err).Error("Error in creating the HTTP write request to annotations RW")
		return "", err
	}

	schemaVersion := ctx.Value(SchemaVersionHeaderKey(SchemaVersionHeader)).(string)
	req.Header.Set(SchemaVersionHeader, schemaVersion)
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
		return "", fmt.Errorf("status %d: %w", resp.StatusCode, ErrUnexpectedStatusWrite)
	}
}

func (rw *annotationsRW) Endpoint() string {
	return rw.endpoint
}

func (rw *annotationsRW) GTG() error {
	req, err := http.NewRequest("GET", rw.endpoint+"/__gtg", nil)
	if err != nil {
		log.WithError(err).Error("Error in creating the HTTP request to annotations RW GTG")
		return fmt.Errorf("GTG: %w", err)
	}

	resp, err := rw.httpClient.Do(req)
	if err != nil {
		log.WithError(err).Error("Error making the HTTP request to annotations RW GTG")
		return fmt.Errorf("GTG: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("status %d: %w", resp.StatusCode, ErrGTGNotOK)
		}
		return fmt.Errorf("status %d %s: %w", resp.StatusCode, string(body), ErrGTGNotOK)
	}

	return nil
}
