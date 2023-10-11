package basicauth

import (
	"encoding/base64"
	"errors"
	"strings"
	"testing"
)

const (
	TestBasicAuthUsername = "testUsername"
	TestBasicAuthPassword = "testPassword"
	AuthorizationHeader   = "Authorization"
)

var ErrCredentialsLength = errors.New("credentials array length should only have two entries")

func GetBasicAuth(basicAuth string) ([]string, error) {
	basicAuthCredentials := strings.Split(basicAuth, ":")
	if len(basicAuthCredentials) != 2 {
		return nil, ErrCredentialsLength
	}
	return basicAuthCredentials, nil
}

func EncodeBasicAuthForTests(t *testing.T) string {
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(strings.Join([]string{TestBasicAuthUsername, TestBasicAuthPassword}, ":")))
}
