package annotations

import (
	"context"
	"encoding/json"
	"fmt"
	tidUtils "github.com/Financial-Times/transactionid-utils-go"
	log "github.com/sirupsen/logrus"
	"net/http"
)

type RW interface {
	Read(ctx context.Context, contentUUID string) ([]*Annotation, bool, error)
	//Endpoint() string TODO implement healthcheck
	//GTG() error
}

type annotationsRW struct {
	endpoint   string
	httpClient *http.Client
}

func NewRW(endpoint string) RW {
	return &annotationsRW{endpoint, &http.Client{}}
}

func (rw *annotationsRW) Read(ctx context.Context, contentUUID string) ([]*Annotation, bool, error) {
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

	req, err := http.NewRequest("GET", rw.endpoint+"/drafts/content/"+contentUUID+"/annotations", nil)
	if err != nil {
		readLog.WithError(err).Error("Error in creating the HTTP request to annotations RW")
		return nil, false, err
	}
	req.Header.Set(tidUtils.TransactionIDHeader, tid)

	resp, err := rw.httpClient.Do(req)
	if err != nil {
		readLog.WithError(err).Error("Error making the HTTP request to annotations RW")
		return nil, false, err
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		var annotations []*Annotation
		err = json.NewDecoder(resp.Body).Decode(&annotations)
		if err != nil {
			readLog.WithError(err).Error("Error in unmarshalling the HTTP response from annotations RW")
			return nil, false, err
		}
		return annotations, true, nil
	case http.StatusNotFound:
		return nil, false, nil
	default:
		return nil, false, fmt.Errorf("annotations RW returned an unexpected HTTP status code: %v", resp.StatusCode)
	}
}
