package yakit

import (
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"sync"
	"time"
)

var (
	updateExecHistoryLock = new(sync.Mutex)
)

func CreateOrUpdateExecHistory(db *gorm.DB, hash string, i interface{}) error {
	db = db.Model(&schema.ExecHistory{})

	updateExecHistoryLock.Lock()
	defer updateExecHistoryLock.Unlock()
	if db := db.Where("hash = ?", hash).Assign(i).FirstOrCreate(&schema.ExecHistory{}); db.Error != nil {
		return utils.Errorf("create/update ExecHistory failed: %s", db.Error)
	}

	return nil
}

func GetExecHistory(db *gorm.DB, id int64) (*schema.ExecHistory, error) {
	var req schema.ExecHistory
	if db := db.Model(&schema.ExecHistory{}).Where("id = ?", id).First(&req); db.Error != nil {
		return nil, utils.Errorf("get ExecHistory failed: %s", db.Error)
	}

	return &req, nil
}

func DeleteExecHistoryByID(db *gorm.DB, id int64) error {
	if db := db.Model(&schema.ExecHistory{}).Where(
		"id = ?", id,
	).Unscoped().Delete(&schema.ExecHistory{}); db.Error != nil {
		return db.Error
	}
	return nil
}

func DeleteExecHistoryAll(db *gorm.DB) error {
	if db := db.Model(&schema.ExecHistory{}).Where("id > 0").Unscoped().Delete(&schema.ExecHistory{}); db.Error != nil {
		return db.Error
	}
	return nil
}

func QueryExecHistory(db *gorm.DB, params *ypb.ExecHistoryRequest) (*bizhelper.Paginator, []*schema.ExecHistory, error) {
	if params == nil {
		params = &ypb.ExecHistoryRequest{}
	}

	originDB := db

	db = db.Select("id,created_at,updated_at,deleted_at,script,script_id,timestamp_nano," +
		"duration_ms,params,ok,reason,runtime_id,from_yak_module," +
		"length(stdout) as stdout_len,length(stderr) as stderr_len," +
		// 插件执行历史扩展字段（恢复现场需要 stream_info / result_status / source 等）
		"plugin_id,plugin_uuid,plugin_type,source,input,exec_params,form_value,extra_params_value," +
		"http_request_template,link_plugin_config,stream_info,result_status,head_img").Table("exec_histories")

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
	// 按来源过滤（plugin-op | plugin-hub），可选
	if source := params.GetSource(); source != "" {
		db = bizhelper.ExactQueryString(db, "source", source)
	}

	var ret []*schema.ExecHistory
	paging, db := bizhelper.Paging(db, int(p.Page), int(p.Limit), &ret)
	if db.Error != nil {
		return nil, nil, utils.Errorf("paging failed: %s", db.Error)
	}

	return paging, ret, nil
}

// SavePluginExecutionHistory 由前端执行结束（finished/stopped）时调用，把参数 + streamInfo 快照
// + runtimeId + resultStatus 一次性写入 project 库的 exec_histories。后端只存储不聚合。
func SavePluginExecutionHistory(db *gorm.DB, req *ypb.SavePluginExecutionHistoryRequest) error {
	if db == nil {
		return utils.Error("no set database")
	}
	if req.GetRuntimeId() == "" {
		return utils.Error("runtime id is empty")
	}

	history := &schema.ExecHistory{
		RuntimeId:     req.GetRuntimeId(),
		FromYakModule: req.GetPluginName(),
		ScriptId:      req.GetPluginUUID(),
		TimestampNano: time.Now().UnixNano(),
		Ok:            req.GetResultStatus() == "finished",
		Reason:        "", // 失败原因前端不传，保留空；如需要可后续扩展

		PluginId:            req.GetPluginId(),
		PluginUUID:          req.GetPluginUUID(),
		PluginType:          req.GetPluginType(),
		Source:              req.GetSource(),
		Input:               req.GetInput(),
		ExecParams:          req.GetExecParams(),
		FormValue:           req.GetFormValue(),
		ExtraParamsValue:    req.GetExtraParamsValue(),
		HTTPRequestTemplate: req.GetHTTPRequestTemplate(),
		LinkPluginConfig:    req.GetLinkPluginConfig(),
		StreamInfo:          req.GetStreamInfo(),
		ResultStatus:        req.GetResultStatus(),
		HeadImg:             req.GetHeadImg(),
	}

	return CreateOrUpdateExecHistory(db, history.CalcHash(), history)
}

// PluginExecutionUsageItem 用于按插件聚合使用次数排行的中间结构。
type PluginExecutionUsageItem struct {
	PluginId     int64  `gorm:"column:plugin_id"`
	PluginName   string `gorm:"column:plugin_name"`
	PluginUUID   string `gorm:"column:plugin_uuid"`
	PluginType   string `gorm:"column:plugin_type"`
	HeadImg      string `gorm:"column:head_img"`
	Count        int64  `gorm:"column:count"`
	LastExecuted int64  `gorm:"column:last_executed_at"`
}

// QueryPluginExecutionUsageRanking 按 plugin_id 分组统计执行次数，按次数降序返回排行。
// 只统计 plugin_id > 0 的记录（临时脚本/纯代码执行不计入插件使用次数）。
func QueryPluginExecutionUsageRanking(db *gorm.DB, limit int) ([]*ypb.PluginExecutionUsageItem, error) {
	if db == nil {
		return nil, utils.Error("no set database")
	}
	if limit <= 0 {
		limit = 50
	}

	var items []*PluginExecutionUsageItem
	if err := db.Table("exec_histories").
		Select("plugin_id, MAX(from_yak_module) as plugin_name, MAX(plugin_uuid) as plugin_uuid, "+
			"MAX(plugin_type) as plugin_type, MAX(head_img) as head_img, "+
			"COUNT(*) as count, MAX(timestamp_nano) as last_executed_at").
		Where("plugin_id > 0").
		Group("plugin_id").
		Order("count DESC").
		Limit(limit).
		Find(&items).Error; err != nil {
		return nil, utils.Errorf("query plugin execution usage ranking failed: %s", err)
	}

	result := make([]*ypb.PluginExecutionUsageItem, 0, len(items))
	for _, it := range items {
		lastAt := int64(0)
		if it.LastExecuted > 0 {
			lastAt = time.Unix(0, it.LastExecuted).Unix()
		}
		result = append(result, &ypb.PluginExecutionUsageItem{
			PluginId:       it.PluginId,
			PluginName:     it.PluginName,
			PluginUUID:     it.PluginUUID,
			PluginType:     it.PluginType,
			HeadImg:        it.HeadImg,
			Count:          it.Count,
			LastExecutedAt: lastAt,
		})
	}
	return result, nil
}
