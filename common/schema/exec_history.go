package schema

import (
	"github.com/yaklang/gorm"
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

	// ===== 插件执行历史扩展字段（B 方案：前端执行结束 POST 回来）=====
	// 这些字段由前端 SavePluginExecutionHistory 接口写入，用于 MenuPlugin/插件详情页的
	// "最近执行/恢复现场/使用次数排行"功能。恢复现场时通过 RuntimeId 回查后端已落库的
	// HTTPFlow/Risk/Port（A 通道），并通过 StreamInfo 快照恢复 log/自定义 table/text/card（B 通道）。

	PluginId            int64  `json:"plugin_id" gorm:"index"`
	PluginUUID          string `json:"plugin_uuid" gorm:"index"`
	PluginType          string `json:"plugin_type" gorm:"index"`
	Source              string `json:"source" gorm:"index"` // plugin-op | plugin-hub
	Input               string `json:"input"`
	ExecParams          string `json:"exec_params"`          // JSON: 序列化后的 []*ypb.KVPair
	FormValue           string `json:"form_value"`           // JSON
	ExtraParamsValue    string `json:"extra_params_value"`   // JSON
	HTTPRequestTemplate string `json:"http_request_template"` // JSON
	LinkPluginConfig    string `json:"link_plugin_config"`    // JSON
	StreamInfo          string `json:"stream_info"`          // JSON: 前端聚合的 HoldGRPCStreamInfo 快照
	ResultStatus        string `json:"result_status"`         // finished | stopped
	HeadImg             string `json:"head_img"`
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
		// 扩展字段
		Source:       f.Source,
		StreamInfo:   f.StreamInfo,
		ResultStatus: f.ResultStatus,
	}
}

func (f *ExecHistory) CalcHash() string {
	return utils.CalcSha1(f.Script, f.RuntimeId)
}

func (f *ExecHistory) BeforeSave() error {
	f.Hash = f.CalcHash()
	return nil
}
