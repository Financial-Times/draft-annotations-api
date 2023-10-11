package basicauth

import (
	"errors"
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

var LessFunc = cmpopts.SortSlices(func(x, y interface{}) bool {
	return fmt.Sprint(x) < fmt.Sprint(y)
})

func TestBasicAuth(t *testing.T) {
	tests := []struct {
		name              string
		input             string
		expectedBasicAuth []string
		expectedError     error
	}{
		{
			name:              "successful parse",
			input:             "username:password",
			expectedBasicAuth: []string{"username", "password"},
		},
		{
			name:          "unsuccessful parse",
			input:         "missingsemicolon",
			expectedError: ErrCredentialsLength,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			basicAuth, err := GetBasicAuth(test.input)
			if err != nil {
				if test.expectedError == nil {
					t.Fatalf("unexpected error occurred: %v", err)
				}

				if !errors.Is(err, test.expectedError) {
					t.Fatalf("expected error: %v, got: %v", test.expectedError, err)
				}

				return
			}

			if test.expectedError != nil {
				t.Fatalf("expected error did not occur: %v", test.expectedError)
			}

			if !cmp.Equal(test.expectedBasicAuth, basicAuth, LessFunc) {
				diff := cmp.Diff(test.expectedBasicAuth, basicAuth)
				t.Errorf("unexpected differences occurred: %v", diff)
			}
		})
	}
}
