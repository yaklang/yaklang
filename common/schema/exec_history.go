package schema

import (
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"strconv"
	"time"
)

type ExecHistory struct {
	gorm.Model

	Hash string `gorm:"unique_index"`

	RuntimeId     string `json:"runtime_id" gorm:"unique_index"`
	Script        string `json:"script"`
	ScriptId      string `json:"script_id" gorm:"index"`
	TimestampNano int64  `json:"timestamp"`
	FromYakModule string `json:"from_yak_module" gorm:"index"`
	DurationMs    int64  `json:"duration_ms"`
	Params        string `json:"params"`
	Stdout        string `json:"stdout"`
	Stderr        string `json:"stderr"`
	Ok            bool   `json:"ok"`
	Reason        string `json:"reason"`

	StdoutLen int64 `json:"stdout_len" gorm:"-"`
	StderrLen int64 `json:"stderr_len" gorm:"-"`

	// json
	Messages string `json:"messages"`
}

func (f *ExecHistory) ToGRPCModel() *ypb.ExecHistoryRecord {
	stdout, _ := strconv.Unquote(f.Stdout)
	stderr, _ := strconv.Unquote(f.Stderr)
	if stdout == "" {
		stdout = f.Stdout
	}
	if stderr == "" {
		stderr = f.Stderr
	}

	rawMsg, _ := strconv.Unquote(f.Messages)
	if rawMsg == "" {
		rawMsg = f.Messages
	}
	return &ypb.ExecHistoryRecord{
		Script:        f.Script,
		ScriptId:      f.ScriptId,
		Timestamp:     time.Unix(0, f.TimestampNano).Unix(),
		DurationMs:    f.DurationMs,
		Params:        f.Params,
		Stderr:        []byte(stderr),
		Stdout:        []byte(stdout),
		Ok:            f.Ok,
		Reason:        f.Reason,
		Id:            f.Hash,
		RuntimeId:     f.RuntimeId,
		FromYakModule: f.FromYakModule,
		StdoutLen:     f.StdoutLen,
		StderrLen:     f.StderrLen,
		Messages:      []byte(rawMsg),
	}
}

func (f *ExecHistory) CalcHash() string {
	return utils.CalcSha1(f.Script, f.RuntimeId)
}

func (f *ExecHistory) BeforeSave() error {
	f.Hash = f.CalcHash()
	return nil
}
