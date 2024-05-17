package yakgrpc

import (
	"encoding/json"
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

const (
	KEY_ProgressManager     = "JznQXuFDSepeNWHbiLGEwONiaBxhvj_PROGRESS_MANAGER"
	KEY_SimpleDetectManager = "JznQXuFDSepeNWHbiLGEwONiaBxhvj_SIMPLE_DETECT_MANAGER"
)

//type ProgressManager struct {
//	db       *gorm.DB
//	poolSize int
//}

//type Progress struct {
//	Uid                  string
//	CreatedAt            int64
//	CurrentProgress      float64
//	YakScriptOnlineGroup string
//	// 记录指针
//	LastRecordPtr int64
//	TaskName      string
//	// 额外信息
//	ExtraInfo string
//}

//func NewProgressManager(db *gorm.DB) *ProgressManager {
//	return &ProgressManager{db: db, poolSize: 30}
//}

func AddExecBatchTask(runtimeId string, percent float64, yakScriptOnlineGroup, taskName string, req *ypb.ExecBatchYakScriptRequest) {
	paramJson, err := json.Marshal(req)
	if err != nil {
		log.Errorf("marshal request failed: %s", err)
		return
	}
	progress := &yakit.Progress{
		RuntimeId:            runtimeId,
		CurrentProgress:      percent,
		YakScriptOnlineGroup: yakScriptOnlineGroup,
		TaskName:             taskName,
		ProgressTaskParam:    paramJson,
		ProgressSource:       KEY_ProgressManager,
		Target:               req.Target,
	}

	err = yakit.CreateOrUpdateProgress(consts.GetGormProjectDatabase(), runtimeId, progress)
	if err != nil {
		log.Errorf("Create or Update progress fail:%v", err)
		return
	}
}

func AddSimpleDetectTask(runtimeId string, req *ypb.RecordPortScanRequest) {
	paramJson, err := json.Marshal(req)
	if err != nil {
		log.Errorf("marshal request failed: %s", err)
		return
	}
	progress := &yakit.Progress{
		RuntimeId:            runtimeId,
		CurrentProgress:      req.LastRecord.GetPercent(),
		YakScriptOnlineGroup: req.LastRecord.GetYakScriptOnlineGroup(),
		TaskName:             req.PortScanRequest.GetTaskName(),
		LastRecordPtr:        req.LastRecord.GetLastRecordPtr(),
		ExtraInfo:            req.LastRecord.GetExtraInfo(),
		ProgressTaskParam:    paramJson,
		ProgressSource:       KEY_SimpleDetectManager,
		Target:               req.GetPortScanRequest().GetTargets(),
	}
	err = yakit.CreateOrUpdateProgress(consts.GetGormProjectDatabase(), runtimeId, progress)
	if err != nil {
		log.Errorf("Create or Update progress fail:%v", err)
		return
	}
}

//func (p *ProgressManager) GetProgressFromDatabase(KEY string) []*Progress {
//	if p.db == nil {
//		return nil
//	}
//
//	list := yakit.GetKey(p.db, KEY)
//	var progress []*Progress
//	_ = json.Unmarshal([]byte(list), &progress)
//	return progress
//}
//
//func (p *ProgressManager) SaveProgressToDatabase(KEY string, progress []*Progress) {
//	raw, err := json.Marshal(progress)
//	if err != nil {
//		log.Errorf("marshal progress failed: %s", err)
//		return
//	}
//	yakit.SetKey(p.db, KEY, string(raw))
//}

func GetBatchYakScriptRequestByRuntimeId(db *gorm.DB, runtimeId string) (*ypb.ExecBatchYakScriptRequest, error) {
	p, err := yakit.GetProgressByRuntimeId(db, runtimeId)
	if err != nil {
		return nil, err
	}
	var req ypb.ExecBatchYakScriptRequest
	err = json.Unmarshal(p.ProgressTaskParam, &req)
	if err != nil {
		return nil, err
	}
	return &req, nil
}

func DeleteBatchYakScriptRequestByRuntimeId(db *gorm.DB, runtimeId string) (*ypb.ExecBatchYakScriptRequest, error) {
	p, err := yakit.DeleteProgressByRuntimeId(db, runtimeId)
	if err != nil {
		return nil, err
	}
	var req ypb.ExecBatchYakScriptRequest
	err = json.Unmarshal(p.ProgressTaskParam, &req)
	if err != nil {
		return nil, err
	}
	return &req, nil
}

func GetSimpleDetectUnfinishedTaskByUid(db *gorm.DB, runtimeId string) (*ypb.RecordPortScanRequest, error) {
	progress, err := yakit.GetProgressByRuntimeId(db, runtimeId)
	if err != nil {
		return nil, err
	}
	var reqRecord ypb.RecordPortScanRequest
	err = json.Unmarshal(progress.ProgressTaskParam, &reqRecord)
	if err != nil {
		return nil, err
	}
	return &reqRecord, nil
}

func DeleteSimpleDetectUnfinishedTaskByUid(db *gorm.DB, runtimeId string) (*ypb.RecordPortScanRequest, error) {
	progress, err := yakit.DeleteProgressByRuntimeId(db, runtimeId)
	if err != nil {
		return nil, err
	}
	var reqRecord ypb.RecordPortScanRequest
	err = json.Unmarshal(progress.ProgressTaskParam, &reqRecord)
	if err != nil {
		return nil, err
	}
	return &reqRecord, nil
}

//func (p *ProgressManager) GetProgressByUid(uid string, removeOld bool) (*ypb.ExecBatchYakScriptRequest, error) {
//	var progress = p.GetProgressFromDatabase(KEY_ProgressManager)
//	progress = funk.Filter(progress, func(i *Progress) bool {
//		return i.Uid != uid
//	}).([]*Progress)
//
//	str := yakit.GetKey(p.db, uid)
//	if str == "" {
//		return nil, utils.Errorf("empty cache for uid[%s]", uid)
//	}
//
//	var req ypb.ExecBatchYakScriptRequest
//	err := json.Unmarshal([]byte(str), &req)
//	if err != nil {
//		return nil, err
//	}
//
//	if removeOld {
//		p.SaveProgressToDatabase(KEY_ProgressManager, progress)
//		// 同时也删除 uid 对应的任务
//		yakit.DelKey(p.db, uid)
//	}
//
//	return &req, nil
//}

//func (p *ProgressManager) GetSimpleProgressByUid(uid string, removeOld, isPop bool) (*ypb.RecordPortScanRequest, error) {
//	var progress = p.GetProgressFromDatabase(KEY_SimpleDetectManager)
//	progress = funk.Filter(progress, func(i *Progress) bool {
//		return i.Uid != uid
//	}).([]*Progress)
//
//	str := yakit.GetKey(p.db, uid)
//	if str == "" {
//		return nil, utils.Errorf("empty cache for uid[%s]", uid)
//	}
//
//	var req ypb.RecordPortScanRequest
//	err := json.Unmarshal([]byte(str), &req)
//	if err != nil {
//		return nil, err
//	}
//
//	if removeOld {
//		p.SaveProgressToDatabase(KEY_SimpleDetectManager, progress)
//		if isPop {
//			runtimeId := gjson.Get(req.LastRecord.ExtraInfo, `Params.#(Key="runtime_id").Value`).String()
//			os.Remove(filepath.Join(consts.GetDefaultYakitBaseTempDir(), runtimeId))
//			// 同时也删除 uid 对应的任务
//			yakit.DelKey(p.db, uid)
//		}
//	}
//
//	return &req, nil
//}
