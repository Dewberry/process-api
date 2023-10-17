package handlers

import (
	"net/http"
	"testing"
)

func TestGetHTTPStatusText(t *testing.T) {
	tests := []struct {
		name     string
		status   int
		expected string
	}{
		{
			name:     "Status OK",
			status:   http.StatusOK,
			expected: "OK",
		},
		{
			name:     "Status Not Found",
			status:   http.StatusNotFound,
			expected: "Not Found",
		},
		{
			name:     "Status Internal Server Error",
			status:   http.StatusInternalServerError,
			expected: "Internal Server Error",
		},
		// ... add more test cases for other status codes if needed.
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errResp := errResponse{HTTPStatus: tt.status}
			if got := errResp.GetHTTPStatusText(); got != tt.expected {
				t.Errorf("errResponse.GetHTTPStatusText() = %v, want %v", got, tt.expected)
			}
		})
	}
}
