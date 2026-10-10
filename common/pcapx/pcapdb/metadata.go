// Package pcapdb owns the PCAP library catalog, independent SQLite indexes and
// self-contained capture storage. The profile database contains catalog records only.
package pcapdb

import (
	"errors"
)

const SchemaVersion = 4

// String alias keeps state values directly comparable in Yak scripts.
type State = string

const (
	StateRegistered  State = "registered"
	StateImporting   State = "importing"
	StateIndexing    State = "indexing"
	StateAnalyzing   State = "analyzing"
	StateReady       State = "ready"
	StateFailed      State = "failed"
	StateInterrupted State = "interrupted"
	StateInvalid     State = "invalid"
)

var (
	ErrClosed            = errors.New("pcapdb: instance manager or database is closed")
	ErrBusy              = errors.New("pcapdb: dataset is being modified by another operation")
	ErrNotReady          = errors.New("pcapdb: dataset is not ready")
	ErrNotFound          = errors.New("pcapdb: dataset not found")
	ErrAmbiguous         = errors.New("pcapdb: source path refers to multiple datasets; use a dataset ID")
	ErrUnsupportedSchema = errors.New("pcapdb: unsupported index schema version")
)

type Progress struct {
	DatasetID        string `json:"dataset_id"`
	State            State  `json:"state"`
	BytesIndexed     int64  `json:"bytes_indexed"`
	TotalBytes       int64  `json:"total_bytes"`
	PacketCount      int64  `json:"packet_count"`
	ProtocolCount    int64  `json:"protocol_count"`
	SessionCount     int64  `json:"session_count"`
	StreamCount      int64  `json:"stream_count"`
	StreamChunkCount int64  `json:"stream_chunk_count"`
}

type ValidationResult struct {
	Metadata PCAPFileDBMetadata `json:"metadata"`
	Valid    bool               `json:"valid"`
	Busy     bool               `json:"busy"`
	Error    string             `json:"error,omitempty"`
}

func activeState(state State) bool {
	return state == StateRegistered || state == StateImporting || state == StateIndexing || state == StateAnalyzing
}

func knownState(state State) bool {
	return activeState(state) || state == StateReady || state == StateFailed || state == StateInterrupted || state == StateInvalid
}

func allowedTransition(from, to State) bool {
	if from == to {
		return true
	}
	if to == StateInvalid {
		return true
	}
	if activeState(from) && (to == StateFailed || to == StateInterrupted) {
		return true
	}
	switch from {
	case StateRegistered:
		return to == StateImporting
	case StateImporting:
		return to == StateIndexing
	case StateIndexing:
		return to == StateAnalyzing || to == StateReady
	case StateAnalyzing:
		return to == StateReady
	case StateReady:
		return to == StateAnalyzing
	case StateFailed, StateInterrupted, StateInvalid:
		return to == StateImporting || to == StateIndexing
	}
	return false
}
