package yakit

import (
	"github.com/jinzhu/gorm"
	"strconv"
	"sync"
	"time"
	"yaklang.io/yaklang/common/utils"
	"yaklang.io/yaklang/common/utils/bizhelper"
	"yaklang.io/yaklang/common/yakgrpc/ypb"
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

var (
	updateExecHistoryLock = new(sync.Mutex)
)

func CreateOrUpdateExecHistory(db *gorm.DB, hash string, i interface{}) error {
	db = db.Model(&ExecHistory{})

	updateExecHistoryLock.Lock()
	defer updateExecHistoryLock.Unlock()
	if db := db.Where("hash = ?", hash).Assign(i).FirstOrCreate(&ExecHistory{}); db.Error != nil {
		return utils.Errorf("create/update ExecHistory failed: %s", db.Error)
	}

	return nil
}

func GetExecHistory(db *gorm.DB, id int64) (*ExecHistory, error) {
	var req ExecHistory
	if db := db.Model(&ExecHistory{}).Where("id = ?", id).First(&req); db.Error != nil {
		return nil, utils.Errorf("get ExecHistory failed: %s", db.Error)
	}

	return &req, nil
}

func DeleteExecHistoryByID(db *gorm.DB, id int64) error {
	if db := db.Model(&ExecHistory{}).Where(
		"id = ?", id,
	).Unscoped().Delete(&ExecHistory{}); db.Error != nil {
		return db.Error
	}
	return nil
}

func DeleteExecHistoryAll(db *gorm.DB) error {
	if db := db.Model(&ExecHistory{}).Where("id > 0").Unscoped().Delete(&ExecHistory{}); db.Error != nil {
		return db.Error
	}
	return nil
}

func QueryExecHistory(db *gorm.DB, params *ypb.ExecHistoryRequest) (*bizhelper.Paginator, []*ExecHistory, error) {
	if params == nil {
		params = &ypb.ExecHistoryRequest{}
	}

	originDB := db

	db = db.Select("id,created_at,updated_at,deleted_at,script,script_id,timestamp_nano," +
		"duration_ms,params,ok,reason,runtime_id,from_yak_module," +
		"length(stdout) as stdout_len,length(stderr) as stderr_len").Table("exec_histories")

	if params.Pagination == nil {
		params.Pagination = &ypb.Paging{
			Page:    1,
			Limit:   30,
			OrderBy: "updated_at",
			Order:   "desc",
		}
	}
	p := params.Pagination
	db = bizhelper.QueryOrder(db, p.OrderBy, p.Order)

	var scriptName = params.GetYakScriptName()
	if scriptName == "" && params.GetYakScriptId() > 0 {
		s, _ := GetYakScript(originDB, params.GetYakScriptId())
		if s != nil {
			scriptName = s.ScriptName
		}
	}
	db = bizhelper.ExactQueryString(db, "from_yak_module", scriptName)

	var ret []*ExecHistory
	paging, db := bizhelper.Paging(db, int(p.Page), int(p.Limit), &ret)
	if db.Error != nil {
		return nil, nil, utils.Errorf("paging failed: %s", db.Error)
	}

	return paging, ret, nil
}
