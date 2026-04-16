//go:build hids

package model

import "time"

const (
	EventTypeProcessExec    = "process.exec"
	EventTypeProcessExit    = "process.exit"
	EventTypeNetworkAccept  = "network.accept"
	EventTypeNetworkConnect = "network.connect"
	EventTypeNetworkClose   = "network.close"
	EventTypeNetworkState   = "network.state"
	EventTypeFileChange     = "file.change"
	EventTypeAudit          = "audit.event"
	EventTypeAuditLoss      = "audit.loss"
)

type Event struct {
	Type      string            `json:"type"`
	Source    string            `json:"source"`
	Timestamp time.Time         `json:"timestamp"`
	Tags      []string          `json:"tags,omitempty"`
	Labels    map[string]string `json:"labels,omitempty"`
	Process   *Process          `json:"process,omitempty"`
	Network   *Network          `json:"network,omitempty"`
	File      *File             `json:"file,omitempty"`
	Audit     *Audit            `json:"audit,omitempty"`
	Data      map[string]any    `json:"data,omitempty"`
}

type Process struct {
	PID        int       `json:"pid,omitempty"`
	ParentPID  int       `json:"parent_pid,omitempty"`
	Name       string    `json:"name,omitempty"`
	Username   string    `json:"username,omitempty"`
	Image      string    `json:"image,omitempty"`
	Command    string    `json:"command,omitempty"`
	ParentName string    `json:"parent_name,omitempty"`
	Artifact   *Artifact `json:"artifact,omitempty"`
}

type Network struct {
	Protocol        string `json:"protocol,omitempty"`
	SourceAddress   string `json:"source_address,omitempty"`
	DestAddress     string `json:"dest_address,omitempty"`
	SourcePort      int    `json:"source_port,omitempty"`
	DestPort        int    `json:"dest_port,omitempty"`
	ConnectionState string `json:"connection_state,omitempty"`
}

type File struct {
	Path      string    `json:"path,omitempty"`
	Operation string    `json:"operation,omitempty"`
	IsDir     bool      `json:"is_dir,omitempty"`
	Mode      string    `json:"mode,omitempty"`
	UID       string    `json:"uid,omitempty"`
	GID       string    `json:"gid,omitempty"`
	Owner     string    `json:"owner,omitempty"`
	Group     string    `json:"group,omitempty"`
	Artifact  *Artifact `json:"artifact,omitempty"`
}

type Audit struct {
	Sequence          uint32   `json:"sequence,omitempty"`
	RecordTypes       []string `json:"record_types,omitempty"`
	Family            string   `json:"family,omitempty"`
	Category          string   `json:"category,omitempty"`
	RecordType        string   `json:"record_type,omitempty"`
	Result            string   `json:"result,omitempty"`
	SessionID         string   `json:"session_id,omitempty"`
	Action            string   `json:"action,omitempty"`
	ObjectType        string   `json:"object_type,omitempty"`
	ObjectPrimary     string   `json:"object_primary,omitempty"`
	ObjectSecondary   string   `json:"object_secondary,omitempty"`
	How               string   `json:"how,omitempty"`
	Username          string   `json:"username,omitempty"`
	UID               string   `json:"uid,omitempty"`
	LoginUser         string   `json:"login_user,omitempty"`
	AUID              string   `json:"auid,omitempty"`
	Terminal          string   `json:"terminal,omitempty"`
	RemoteIP          string   `json:"remote_ip,omitempty"`
	RemotePort        string   `json:"remote_port,omitempty"`
	RemoteHost        string   `json:"remote_host,omitempty"`
	ProcessCWD        string   `json:"process_cwd,omitempty"`
	FileMode          string   `json:"file_mode,omitempty"`
	FileUID           string   `json:"file_uid,omitempty"`
	FileGID           string   `json:"file_gid,omitempty"`
	FileOwner         string   `json:"file_owner,omitempty"`
	FileGroup         string   `json:"file_group,omitempty"`
	PreviousFileMode  string   `json:"previous_file_mode,omitempty"`
	PreviousFileUID   string   `json:"previous_file_uid,omitempty"`
	PreviousFileGID   string   `json:"previous_file_gid,omitempty"`
	PreviousFileOwner string   `json:"previous_file_owner,omitempty"`
	PreviousFileGroup string   `json:"previous_file_group,omitempty"`
}
