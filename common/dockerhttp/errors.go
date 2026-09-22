package dockerhttp

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// ErrorResponse is the restricted JSON error body returned by the Engine API.
type ErrorResponse struct {
	Message string `json:"message"`
}

// APIError is a non-success Engine HTTP response.
type APIError struct {
	StatusCode int
	Message    string
}

func (e *APIError) Error() string {
	if e.Message != "" {
		return fmt.Sprintf("Error response from daemon: %s", e.Message)
	}
	return fmt.Sprintf("Error response from daemon: status %d", e.StatusCode)
}

// IsNotFound reports whether err is an API 404.
func IsNotFound(err error) bool {
	var ae *APIError
	if AsAPIError(err, &ae) {
		return ae.StatusCode == http.StatusNotFound
	}
	return false
}

// IsConflict reports whether err is an API 409.
func IsConflict(err error) bool {
	var ae *APIError
	if AsAPIError(err, &ae) {
		return ae.StatusCode == http.StatusConflict
	}
	return false
}

// AsAPIError extracts *APIError from err (including wrapped).
func AsAPIError(err error, target **APIError) bool {
	if err == nil {
		return false
	}
	if ae, ok := err.(*APIError); ok {
		*target = ae
		return true
	}
	type unwrapper interface{ Unwrap() error }
	if u, ok := err.(unwrapper); ok {
		return AsAPIError(u.Unwrap(), target)
	}
	return false
}

// parseErrorBody reads a limited amount of the response and extracts a message.
// It never panics on bad JSON.
func parseErrorBody(resp *http.Response) error {
	const maxErrBody = 8 << 10 // 8 KiB
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrBody))
	msg := strings.TrimSpace(string(body))
	if len(body) > 0 {
		var er ErrorResponse
		if json.Unmarshal(body, &er) == nil && er.Message != "" {
			msg = er.Message
		}
	}
	if msg == "" {
		msg = resp.Status
	}
	return &APIError{StatusCode: resp.StatusCode, Message: msg}
}

// VersionIncompatibleError indicates client/daemon API mismatch after negotiation.
type VersionIncompatibleError struct {
	Client string
	Server string
	Detail string
}

func (e *VersionIncompatibleError) Error() string {
	if e.Detail != "" {
		return e.Detail
	}
	return fmt.Sprintf("API version incompatible: client=%s server=%s", e.Client, e.Server)
}
