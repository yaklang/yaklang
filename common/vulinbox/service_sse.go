package vulinbox

import (
	_ "embed"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/yaklang/yaklang/common/log"
)

//go:embed html/sse_stream.html
var sseStreamPage []byte

func (s *VulinServer) registerSSE() {
	group := s.router.Name("SSE Streaming Demo").Subrouter()

	addRouteWithVulInfo(group, &VulInfo{
		Path:    "/sse/",
		Title:   "SSE Streaming Demo",
		Handler: s.ssePageHandler,
	})

	group.HandleFunc("/sse/stream", s.sseStreamHandler).Methods(http.MethodGet)
}

func (s *VulinServer) ssePageHandler(writer http.ResponseWriter, request *http.Request) {
	writer.Header().Set("Content-Type", "text/html; charset=utf-8")
	if _, err := writer.Write(sseStreamPage); err != nil {
		log.Errorf("write sse demo page failed: %v", err)
	}
}

func (s *VulinServer) sseStreamHandler(writer http.ResponseWriter, request *http.Request) {
	flusher, ok := writer.(http.Flusher)
	if !ok {
		http.Error(writer, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	query := request.URL.Query()
	intervalMs := parsePositiveInt(query.Get("interval"), 500, 50, 60000)
	count := parseNonNegativeInt(query.Get("count"), 10)
	retryMs := parsePositiveInt(query.Get("retry"), 3000, 100, 60000)
	message := query.Get("message")
	if message == "" {
		message = "yaklang sse message"
	}
	eventName := query.Get("event")

	writer.Header().Set("Content-Type", "text/event-stream")
	writer.Header().Set("Cache-Control", "no-cache")
	writer.Header().Set("Connection", "keep-alive")
	writer.Header().Set("X-Accel-Buffering", "no")
	writer.Header().Set("Access-Control-Allow-Origin", "*")

	fmt.Fprintf(writer, "retry: %d\n\n", retryMs)
	fmt.Fprint(writer, ": stream start\n\n")
	flusher.Flush()

	ticker := time.NewTicker(time.Duration(intervalMs) * time.Millisecond)
	defer ticker.Stop()

	sent := 0
	for {
		if count > 0 && sent >= count {
			fmt.Fprint(writer, "event: end\n")
			fmt.Fprint(writer, "data: stream finished\n\n")
			flusher.Flush()
			return
		}

		select {
		case <-request.Context().Done():
			return
		case now := <-ticker.C:
			sent++
			fmt.Fprintf(writer, "id: %d\n", sent)
			if eventName != "" {
				fmt.Fprintf(writer, "event: %s\n", eventName)
			}
			fmt.Fprintf(writer, "data: %s #%d at %s\n\n", message, sent, now.Format(time.RFC3339Nano))
			flusher.Flush()
		}
	}
}

func parsePositiveInt(raw string, fallback, min, max int) int {
	if raw == "" {
		return fallback
	}
	val, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	if val < min {
		return min
	}
	if val > max {
		return max
	}
	return val
}

func parseNonNegativeInt(raw string, fallback int) int {
	if raw == "" {
		return fallback
	}
	val, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	if val < 0 {
		return 0
	}
	return val
}
