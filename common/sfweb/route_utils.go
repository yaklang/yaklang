package sfweb

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/yaklang/yaklang/common/log"
)

var SfWebLogger = log.GetLogger("sfweb")

type ErrorResponse struct {
	Message string `json:"message"`
}

func writeErrorJson(w http.ResponseWriter, err error) {
	errBody, _ := json.Marshal(&ErrorResponse{err.Error()})
	w.WriteHeader(http.StatusInternalServerError)
	w.Write(errBody)
}

func writeJson(w http.ResponseWriter, data any) {
	body, err := json.Marshal(data)
	if err != nil {
		writeErrorJson(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(200)
	w.Write(body)
}

type LogHTTPResponseWriter struct {
	http.ResponseWriter
	http.Hijacker
	wroteHeader bool
	raw         *bytes.Buffer
}

func NewLogHTTPResponseWriter(w http.ResponseWriter) *LogHTTPResponseWriter {
	nw := &LogHTTPResponseWriter{ResponseWriter: w, raw: &bytes.Buffer{}}
	if hijacker, ok := w.(http.Hijacker); ok {
		nw.Hijacker = hijacker
	}
	return nw
}

func (w *LogHTTPResponseWriter) Write(b []byte) (int, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	w.raw.Write(b)
	return w.ResponseWriter.Write(b)
}

func (w *LogHTTPResponseWriter) Raw() []byte {
	return w.raw.Bytes()
}

func (w *LogHTTPResponseWriter) WriteHeader(statusCode int) {
	if !w.wroteHeader {
		w.wroteHeader = true
		statusLine := fmt.Sprintf("HTTP/1.1 %d %s\r\n", statusCode, http.StatusText(statusCode))
		w.raw.WriteString(statusLine)

		w.ResponseWriter.Header().Write(w.raw)
		w.raw.WriteString("\r\n")
	}
	w.ResponseWriter.WriteHeader(statusCode)
}
