package yakgrpc

import (
	"encoding/json"
	"github.com/jinzhu/gorm"
	"github.com/tidwall/gjson"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/go-funk"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"os"
	"path/filepath"
	"time"
)

const (
	KEY_ProgressManager     = "JznQXuFDSepeNWHbiLGEwONiaBxhvj_PROGRESS_MANAGER"
	KEY_SimpleDetectManager = "JznQXuFDSepeNWHbiLGEwONiaBxhvj_SIMPLE_DETECT_MANAGER"
)

type ProgressManager struct {
	db       *gorm.DB
	poolSize int
}

type Progress struct {
	Uid                  string
	CreatedAt            int64
	CurrentProgress      float64
	YakScriptOnlineGroup string
	// 记录指针
	LastRecordPtr int64
	TaskName      string
	// 额外信息
	ExtraInfo string
}

func NewProgressManager(db *gorm.DB) *ProgressManager {
	return &ProgressManager{db: db, poolSize: 30}
}

func (p *ProgressManager) AddExecBatchTaskToPool(uid string, percent float64, yakScriptOnlineGroup, taskName string, req *ypb.ExecBatchYakScriptRequest) {
	progress := p.GetProgressFromDatabase(KEY_ProgressManager)
	if len(progress) > p.poolSize {
		var removed *Progress
		removed, progress = progress[1], progress[1:]
		if removed != nil {
			yakit.DelKey(p.db, removed.Uid)
		}
	}
	progress = append(progress, &Progress{
		Uid:                  uid,
		CreatedAt:            time.Now().Unix(),
		CurrentProgress:      percent,
		YakScriptOnlineGroup: yakScriptOnlineGroup,
		TaskName:             taskName,
	})
	p.SaveProgressToDatabase(KEY_ProgressManager, progress)
	paramJson, err := json.Marshal(req)
	if err != nil {
		log.Errorf("marshal request failed: %s", err)
		return
	}
	yakit.SetKey(p.db, uid, string(paramJson))
}

func (p *ProgressManager) AddSimpleDetectTaskToPool(uid string, req *ypb.RecordPortScanRequest) {
	progress := p.GetProgressFromDatabase(KEY_SimpleDetectManager)
	if len(progress) > p.poolSize {
		var removed *Progress
		removed, progress = progress[1], progress[1:]
		if removed != nil {
			yakit.DelKey(p.db, removed.Uid)
		}
	}
	progress = append(progress, &Progress{
		Uid:                  uid,
		CreatedAt:            time.Now().Unix(),
		CurrentProgress:      req.LastRecord.GetPercent(),
		YakScriptOnlineGroup: req.LastRecord.GetYakScriptOnlineGroup(),
		TaskName:             req.PortScanRequest.GetTaskName(),
		LastRecordPtr:        req.LastRecord.GetLastRecordPtr(),
		ExtraInfo:            req.LastRecord.GetExtraInfo(),
	})
	p.SaveProgressToDatabase(KEY_SimpleDetectManager, progress)
	paramJson, err := json.Marshal(req)
	if err != nil {
		log.Errorf("marshal request failed: %s", err)
		return
	}
	yakit.SetKey(p.db, uid, string(paramJson))
}

func (p *ProgressManager) GetProgressFromDatabase(KEY string) []*Progress {
	if p.db == nil {
		return nil
	}

	list := yakit.GetKey(p.db, KEY)
	var progress []*Progress
	_ = json.Unmarshal([]byte(list), &progress)
	return progress
}

func (p *ProgressManager) SaveProgressToDatabase(KEY string, progress []*Progress) {
	raw, err := json.Marshal(progress)
	if err != nil {
		log.Errorf("marshal progress failed: %s", err)
		return
	}
	yakit.SetKey(p.db, KEY, string(raw))
}

func (p *ProgressManager) GetProgressByUid(uid string, removeOld bool) (*ypb.ExecBatchYakScriptRequest, error) {
	var progress = p.GetProgressFromDatabase(KEY_ProgressManager)
	progress = funk.Filter(progress, func(i *Progress) bool {
		return i.Uid != uid
	}).([]*Progress)

	str := yakit.GetKey(p.db, uid)
	if str == "" {
		return nil, utils.Errorf("empty cache for uid[%s]", uid)
	}

	var req ypb.ExecBatchYakScriptRequest
	err := json.Unmarshal([]byte(str), &req)
	if err != nil {
		return nil, err
	}

	if removeOld {
		p.SaveProgressToDatabase(KEY_ProgressManager, progress)
		// 同时也删除 uid 对应的任务
		yakit.DelKey(p.db, uid)
	}

	return &req, nil
}

func (p *ProgressManager) GetSimpleProgressByUid(uid string, removeOld, isPop bool) (*ypb.RecordPortScanRequest, error) {
	var progress = p.GetProgressFromDatabase(KEY_SimpleDetectManager)
	progress = funk.Filter(progress, func(i *Progress) bool {
		return i.Uid != uid
	}).([]*Progress)

	str := yakit.GetKey(p.db, uid)
	if str == "" {
		return nil, utils.Errorf("empty cache for uid[%s]", uid)
	}

	var req ypb.RecordPortScanRequest
	err := json.Unmarshal([]byte(str), &req)
	if err != nil {
		return nil, err
	}

	if removeOld {
		p.SaveProgressToDatabase(KEY_SimpleDetectManager, progress)
		if isPop {
			runtimeId := gjson.Get(req.LastRecord.ExtraInfo, `Params.#(Key="runtime_id").Value`).String()
			os.Remove(filepath.Join(consts.GetDefaultYakitBaseTempDir(), runtimeId))
			// 同时也删除 uid 对应的任务
			yakit.DelKey(p.db, uid)
		}
	}

	return &req, nil
}
