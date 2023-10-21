package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
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

func TestValidateFormat(t *testing.T) {
	e := echo.New()

	// Helper function to create a new request context with the specified format query parameter
	newContext := func(param string) echo.Context {
		req := httptest.NewRequest(echo.GET, "/?f="+param, nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		return c
	}

	tests := []struct {
		name       string
		param      string
		shouldFail bool
	}{
		{"Valid Format: Empty", "", false},
		{"Valid Format: JSON", "json", false},
		{"Valid Format: HTML", "html", false},
		{"Invalid Format: XML", "xml", true},
		// ... add more test cases as needed.
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newContext(tt.param)
			err := validateFormat(c)
			if tt.shouldFail && err == nil {
				t.Errorf("Expected failure for format: %v", tt.param)
			} else if !tt.shouldFail && err != nil {
				t.Errorf("Unexpected failure for format: %v", tt.param)
			}
		})
	}
}

// Mock for echo.Context to be used in the test
func newEchoContext(req *http.Request, res *httptest.ResponseRecorder) echo.Context {
	e := echo.New()
	return e.NewContext(req, res)
}

func TestPrepareResponse(t *testing.T) {
	tests := []struct {
		name         string
		queryFormat  string
		acceptHeader string
		httpStatus   int
		expectedType string
		expectedBody string
	}{
		{
			name:         "Query Format JSON",
			queryFormat:  "json",
			acceptHeader: "",
			httpStatus:   http.StatusOK,
			expectedType: "application/json; charset=UTF-8",
			expectedBody: `{"message":"test message"}`,
		},
		{
			name:         "Query Format HTML",
			queryFormat:  "html",
			acceptHeader: "",
			httpStatus:   http.StatusOK,
			expectedType: "text/html; charset=UTF-8",
			expectedBody: "HTML_RENDERED_OUTPUT", // You'll have to replace this with expected HTML output
		},
		{
			name:         "Accept Header JSON",
			queryFormat:  "",
			acceptHeader: "application/json",
			httpStatus:   http.StatusOK,
			expectedType: "application/json; charset=UTF-8",
			expectedBody: `{"message":"test message"}`,
		},
		{
			name:         "Accept Header HTML",
			queryFormat:  "",
			acceptHeader: "text/html",
			httpStatus:   http.StatusOK,
			expectedType: "text/html; charset=UTF-8",
			expectedBody: "HTML_RENDERED_OUTPUT", // You'll have to replace this with expected HTML output
		},
		// Add more test cases if needed...
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/?f="+tt.queryFormat, nil)
			req.Header.Set("Accept", tt.acceptHeader)
			res := httptest.NewRecorder()

			c := newEchoContext(req, res)

			// Assuming output is a simple message for demonstration purposes.
			output := map[string]string{"message": "test message"}

			err := prepareResponse(c, tt.httpStatus, "renderName", output) // "renderName" is a placeholder
			if err != nil {
				t.Fatalf("prepareResponse failed: %v", err)
			}

			if res.Header().Get("Content-Type") != tt.expectedType {
				t.Errorf("Expected Content-Type: %s, Got: %s", tt.expectedType, res.Header().Get("Content-Type"))
			}

			body := res.Body.String()
			if strings.TrimSpace(body) != tt.expectedBody {
				t.Errorf("Expected Body: %s, Got: %s", tt.expectedBody, body)
			}
		})
	}
}

func TestLandingPage(t *testing.T) {
	rh := &RESTHandler{
		Title:       "Test Title",
		Description: "Test Description",
	}

	tests := []struct {
		name         string
		queryFormat  string
		expectedBody string
	}{
		{
			name:         "Default Format",
			queryFormat:  "",
			expectedBody: `{"description":"Test Description","title":"Test Title"}`,
		},
		// Add other test cases for different formats, if needed...
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/?f="+tt.queryFormat, nil)
			res := httptest.NewRecorder()

			c := newEchoContext(req, res)

			if err := rh.LandingPage(c); err != nil {
				t.Fatalf("LandingPage failed: %v", err)
			}

			body := res.Body.String()
			if strings.TrimSpace(body) != tt.expectedBody {
				t.Errorf("Expected Body: %s, Got: %s", tt.expectedBody, body)
				t.Logf("Expected length: %d, Got length: %d", len(tt.expectedBody), len(body))
			}
		})
	}
}

func TestConformance(t *testing.T) {
	rh := &RESTHandler{
		ConformsTo: []string{
			"Test Conform 1",
			"Test Conform 2",
		},
	}

	tests := []struct {
		name         string
		queryFormat  string
		expectedBody string
	}{
		{
			name:         "Default Format",
			queryFormat:  "",
			expectedBody: `{"conformsTo":["Test Conform 1","Test Conform 2"]}`,
		},
		// Add other test cases for different formats, if needed...
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/conformance?f="+tt.queryFormat, nil)
			res := httptest.NewRecorder()

			c := newEchoContext(req, res)

			if err := rh.Conformance(c); err != nil {
				t.Fatalf("Conformance failed: %v", err)
			}

			body := res.Body.String()
			if strings.TrimSpace(body) != tt.expectedBody {
				t.Errorf("Expected Body: %s, Got: %s", tt.expectedBody, body)
			}
		})
	}
}
