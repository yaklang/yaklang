package aihttp

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/yaklang/yaklang/common/log"
)

// writeJSON writes a JSON response
func writeJSON(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.Errorf("Failed to encode JSON response: %v", err)
	}
}

// writeError writes an error response
func writeError(w http.ResponseWriter, statusCode int, errType string, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	writeJSON(w, ErrorResponse{
		Error:   errType,
		Message: message,
		Code:    statusCode,
	})
}

// readJSON reads JSON from request body
func readJSON(r *http.Request, v interface{}) error {
	if r.Body == nil {
		return fmt.Errorf("empty request body")
	}
	defer r.Body.Close()
	return json.NewDecoder(r.Body).Decode(v)
}

// writeSSE writes a Server-Sent Event
func writeSSE(w http.ResponseWriter, event *SSEEvent) error {
	if event.ID != "" {
		fmt.Fprintf(w, "id: %s\n", event.ID)
	}
	if event.Event != "" {
		fmt.Fprintf(w, "event: %s\n", event.Event)
	}
	if event.Retry > 0 {
		fmt.Fprintf(w, "retry: %d\n", event.Retry)
	}

	data, err := json.Marshal(event.Data)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "data: %s\n\n", data)

	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}
	return nil
}

// setupSSE sets up SSE headers
func setupSSE(w http.ResponseWriter) http.Flusher {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	flusher, ok := w.(http.Flusher)
	if !ok {
		return nil
	}
	flusher.Flush()
	return flusher
}
