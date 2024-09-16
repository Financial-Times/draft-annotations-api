// Package synchronise: This package is responsible for synchronising the draft annotations between the PAC and the publishing cluster.
// And it's a temporary solution part of the PAC decommissioning process.
package synchronise

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strings"

	tidutils "github.com/Financial-Times/transactionid-utils-go"
	log "github.com/sirupsen/logrus"
)

const (
	publishOrigin        = "draft-annotations-publishing"
	pacOrigin            = "draft-annotations-pac"
	forwardedHeader      = "X-Forwarded-By"
	originSystemIDHeader = "X-Origin-System-Id"
	PACOriginSystemID    = "http://cmdb.ft.com/systems/pac"
)

type API struct {
	client   *http.Client
	username string
	password string
	endpoint string
}

func NewAPI(client *http.Client, username string, password string, endpoint string) *API {
	return &API{
		client:   client,
		username: username,
		password: password,
		endpoint: endpoint,
	}
}

// SyncWithPublishingCluster forwards the request to the publishing cluster.
func (api *API) SyncWithPublishingCluster(req *http.Request) error {
	tID := tidutils.GetTransactionIDFromRequest(req)

	// Check if the request is already forwarded by publishing cluster to avoid infinite loop
	if req.Header.Get(forwardedHeader) == publishOrigin {
		return nil
	}

	// Copy the request
	bodyBytes, err := io.ReadAll(req.Body)
	if err != nil {
		return err
	}

	// Restore the io.ReadCloser after reading from it
	req.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

	// Create a new request
	path := strings.Replace(req.URL.Path, "/drafts", "/draft-annotations", 1)
	newReq, err := http.NewRequest(req.Method, fmt.Sprintf("%s%s", api.endpoint, path), bytes.NewBuffer(bodyBytes))
	if err != nil {
		return err
	}

	// Copy the headers
	for name, values := range req.Header {
		for _, value := range values {
			newReq.Header.Add(name, value)
		}
	}

	// Add the X-Forwarded-By header
	newReq.Header.Add(forwardedHeader, pacOrigin)

	// Add the X-Origin-System-Id header as it is mandatory in the publishing cluster
	if newReq.Header.Get(originSystemIDHeader) == "" {
		newReq.Header.Add(originSystemIDHeader, PACOriginSystemID)
	}

	// Set basic auth
	newReq.SetBasicAuth(api.username, api.password)

	log.WithFields(map[string]interface{}{
		"method":          newReq.Method,
		"url":             newReq.URL.String(),
		"forwardedHeader": newReq.Header.Get(forwardedHeader),
		"originHeader":    newReq.Header.Get(originSystemIDHeader),
		"host":            newReq.URL.Host,
		"transaction_id":  tID,
	}).Info("Sending request to publishing cluster")

	// Send the request
	resp, err := api.client.Do(newReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	return nil
}
