package aihttp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeProtoJSON(w http.ResponseWriter, status int, msg proto.Message) {
	data, err := protojson.MarshalOptions{UseProtoNames: true}.Marshal(msg)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "marshal proto response failed: "+err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(data)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, ErrorResponse{
		Error:   http.StatusText(status),
		Code:    status,
		Message: msg,
	})
}

func readJSON(r *http.Request, v any) error {
	data, err := readRawBody(r)
	if err != nil {
		return err
	}
	return readJSONBytes(data, v)
}

func readProtoJSON(r *http.Request, msg proto.Message) error {
	data, err := readRawBody(r)
	if err != nil {
		return err
	}
	return readProtoJSONBytes(data, msg)
}

func readRawBody(r *http.Request) ([]byte, error) {
	data, err := readOptionalRawBody(r)
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("request body is empty")
	}
	return data, nil
}

func readOptionalRawBody(r *http.Request) ([]byte, error) {
	if r.Body == nil {
		return nil, nil
	}
	defer r.Body.Close()
	data, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, err
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return nil, nil
	}
	return data, nil
}

func readJSONBytes(data []byte, v any) error {
	return json.Unmarshal(data, v)
}

func readProtoJSONBytes(data []byte, msg proto.Message) error {
	return protojson.UnmarshalOptions{DiscardUnknown: false}.Unmarshal(data, msg)
}

func writeSSE(w http.ResponseWriter, event, data string) {
	fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, data)
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
}

func writeSSEData(w http.ResponseWriter, data string) {
	fmt.Fprintf(w, "data: %s\n\n", data)
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
}

func writeProtoSSEData(w http.ResponseWriter, msg proto.Message) error {
	data, err := protojson.MarshalOptions{UseProtoNames: true}.Marshal(msg)
	if err != nil {
		return err
	}
	writeSSEData(w, string(data))
	return nil
}
