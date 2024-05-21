package yakit

import (
	"context"
	"encoding/json"
	"github.com/jinzhu/gorm"
	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/go-funk"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"strconv"
	"strings"
)

// Deprecated
func QueryFirst50WebFuzzerTask(db *gorm.DB) []*ypb.HistoryHTTPFuzzerTask {
	var task []*schema.WebFuzzerTask
	if db := db.Model(&schema.WebFuzzerTask{}).Where("id = retry_root_id or retry_root_id is null or retry_root_id = 0").Order("created_at desc").Find(&task); db.Error != nil {
		log.Errorf("query web fuzzer task failed: %s", db.Error)
		return nil
	} else {
		return funk.Map(task, func(i *schema.WebFuzzerTask) *ypb.HistoryHTTPFuzzerTask {
			return i.ToGRPCModel()
		}).([]*ypb.HistoryHTTPFuzzerTask)
	}
}

func QueryFuzzerHistoryTasks(db *gorm.DB, req *ypb.QueryHistoryHTTPFuzzerTaskExParams) (*bizhelper.Paginator, []*schema.WebFuzzerTask, error) {
	oldDB := db

	var keywords []string
	if req.GetKeyword() != "" {
		keywords = append(keywords, req.GetKeyword())
		keywords = append(keywords, strings.Trim(strconv.Quote(req.GetKeyword()), `" \r\n`))
		db = bizhelper.FuzzSearchWithStringArrayOrEx(db, []string{
			"id", "raw_fuzz_task_request", "host",
		}, keywords, false)
	}

	if req.GetFuzzerTabIndex() != "" {
		db = db.Where("fuzzer_tab_index = ?", req.GetFuzzerTabIndex())
	}
	pagination := req.GetPagination()
	order, orderby := pagination.Order, pagination.OrderBy
	if order == "" {
		order = "asc"
	}
	if orderby == "" {
		orderby = "id"
	}

	// 返回的任务跳过重试的任务
	db = db.Where("id = retry_root_id or retry_root_id is null or retry_root_id = 0")

	var returnTasks, tasks []*schema.WebFuzzerTask

	db = bizhelper.QueryOrder(db, orderby, order)
	paging, db := bizhelper.Paging(db, int(pagination.GetPage()), int(pagination.GetLimit()), &returnTasks)
	if db.Error != nil {
		return nil, nil, utils.Errorf("pagination failed: %s", db.Error)
	}

	// 对重试任务进行处理

	// 先获取所有task的id
	ids := lo.Map(returnTasks, func(i *schema.WebFuzzerTask, _ int) int64 {
		return int64(i.ID)
	})
	db = oldDB.Model(&schema.WebFuzzerTask{}).Select([]string{"id", "retry_root_id", "http_flow_total", "http_flow_success_count"})
	db = bizhelper.ExactQueryInt64ArrayOr(db, "retry_root_id", ids)
	// 找到重试任务，计算总共成功的数量
	if db.Find(&tasks); db.Error != nil {
		return nil, nil, utils.Errorf("search by retry_root_id failed: %s", db.Error)
	}
	successCountMap := make(map[uint]int)
	for _, task := range tasks {
		if _, ok := successCountMap[task.RetryRootID]; !ok {
			successCountMap[task.RetryRootID] = 0
		}
		successCountMap[task.RetryRootID] += task.HTTPFlowSuccessCount
	}
	// 更新返回的任务
	for _, task := range returnTasks {
		if successCount, ok := successCountMap[uint(task.ID)]; ok {
			task.HTTPFlowSuccessCount = successCount
			task.HTTPFlowFailedCount = task.HTTPFlowTotal - successCount
		}
	}

	return paging, returnTasks, nil
}

func SaveWebFuzzerTask(db *gorm.DB, req *ypb.FuzzerRequest, total int, ok bool, reason string) (*schema.WebFuzzerTask, error) {
	if req.Verbose == "" {
		if req.Request == "" && req.RequestRaw != nil {
			req.Verbose = utils.EscapeInvalidUTF8Byte(req.RequestRaw)
		}
	}
	raw, err := json.Marshal(req)
	if err != nil {
		return nil, utils.Errorf("marshal fuzzer request failed: %s", err)
	}

	t := &schema.WebFuzzerTask{
		RawFuzzTaskRequest: string(raw),
		HTTPFlowTotal:      total,
		Ok:                 ok,
		Reason:             reason,
	}
	if db := db.Save(t); db.Error != nil {
		return nil, db.Error
	}
	return t, nil
}

func DeleteWebFuzzerTaskAll(db *gorm.DB) error {
	if db := db.Model(&schema.WebFuzzerTask{}).Where("true").Unscoped().Delete(&schema.WebFuzzerTask{}); db.Error != nil {
		return utils.Errorf("delete web fuzzer all failed: %s", db.Error)
	}
	return nil
}

func DeleteWebFuzzerTask(db *gorm.DB, id int64) error {
	if db := db.Model(&schema.WebFuzzerTask{}).Where("id = ?", id).Unscoped().Delete(&schema.WebFuzzerTask{}); db.Error != nil {
		return utils.Errorf("delete web fuzzer by id failed: %s", db.Error)
	}
	return nil
}

func DeleteWebFuzzerTaskByWebFuzzerIndex(db *gorm.DB, index string) error {
	if db := db.Model(&schema.WebFuzzerTask{}).Where("fuzzer_tab_index = ?", index).Unscoped().Delete(&schema.WebFuzzerTask{}); db.Error != nil {
		return utils.Errorf("delete web fuzzer by fuzzer_tab_index failed: %s", db.Error)
	}
	return nil
}

func GetWebFuzzerTaskById(db *gorm.DB, id int) (*schema.WebFuzzerTask, error) {
	var t schema.WebFuzzerTask
	if db := db.Model(&schema.WebFuzzerTask{}).Where("id = ?", id).First(&t); db.Error != nil {
		return nil, utils.Errorf("get web fuzzer task failed: %s", db.Error)
	}
	return &t, nil
}

func GetWebFuzzerRetryRootID(db *gorm.DB, id uint) (uint, error) {
	var t schema.WebFuzzerTask
	if db := db.Model(&schema.WebFuzzerTask{}).Select("retry_root_id").Where("id = ?", id).First(&t); db.Error != nil {
		return 0, utils.Errorf("get web fuzzer task retry_root_id failed: %s", db.Error)
	}
	return t.RetryRootID, nil
}

func GetWebFuzzerTasksIDByRetryRootID(db *gorm.DB, root_id uint) ([]uint, error) {
	var ids []uint
	if db := db.Model(&schema.WebFuzzerTask{}).Where("retry_root_id = ?", root_id).Pluck("id", &ids); db.Error != nil {
		return nil, utils.Errorf("get web fuzzer task id by retry_root_id failed: %s", db.Error)
	}
	return ids, nil
}

func DeleteWebFuzzerResponseByTaskID(db *gorm.DB, id int64) error {
	if db := db.Model(&schema.WebFuzzerResponse{}).Where(
		"web_fuzzer_task_id = ?", id,
	).Unscoped().Delete(&schema.WebFuzzerResponse{}); db.Error != nil {
		return db.Error
	}
	return nil
}

func QueryWebFuzzerResponse(db *gorm.DB, params *ypb.QueryHTTPFuzzerResponseByTaskIdRequest) (*bizhelper.Paginator, []*schema.WebFuzzerResponse, error) {
	db = db.Model(&schema.WebFuzzerResponse{})

	db = db.Where("web_fuzzer_task_id = ?", params.GetTaskId())

	p := params.GetPagination()
	db = bizhelper.QueryOrder(db, "created_at", "desc")

	var ret []*schema.WebFuzzerResponse
	paging, db := bizhelper.Paging(db, int(p.GetPage()), int(p.GetLimit()), &ret)
	if db.Error != nil {
		return nil, nil, utils.Errorf("paging failed: %s", db.Error)
	}

	return paging, ret, nil
}

func YieldWebFuzzerResponseByTaskIDs(db *gorm.DB, ctx context.Context, taskIDs []uint, oks ...bool) chan *schema.WebFuzzerResponse {
	int64TaskIDs := lo.Map(taskIDs, func(i uint, _ int) int64 { return int64(i) })

	db = db.Model(&schema.WebFuzzerResponse{})
	db = bizhelper.ExactQueryInt64ArrayOr(db, "web_fuzzer_task_id", int64TaskIDs)
	if len(oks) > 0 {
		db = db.Where("ok = ?", oks[0])
	}
	outC := make(chan *schema.WebFuzzerResponse)
	yieldWebFuzzerResponsesToChan(outC, db, ctx)
	return outC
}

func SaveWebFuzzerResponse(db *gorm.DB, taskId int, hiddenIndex string, rsp *ypb.FuzzerResponse) {
	raw, err := json.Marshal(rsp)
	if err != nil {
		log.Errorf("marshal FuzzerResponse failed: %s", err)
		return
	}
	r := &schema.WebFuzzerResponse{
		WebFuzzerTaskId: taskId,
		OK:              rsp.Ok,
		Request:         utils.UnsafeBytesToString(rsp.RequestRaw),
		Content:         utils.UnsafeBytesToString(raw),
		Payload:         strings.Join(rsp.Payloads, ","),
		Url:             rsp.Url,
		StatusCode:      int(rsp.StatusCode),
		DurationMs:      int(rsp.DurationMs),
		Timestamp:       rsp.GetTimestamp(),
		HiddenIndex:     hiddenIndex,
	}
	if db := db.Save(r); db.Error != nil {
		log.Errorf("save web fuzzer response to database failed: %s", db.Error)
		return
	}
}

func YieldWebFuzzerResponses(db *gorm.DB, ctx context.Context, id int) chan *schema.WebFuzzerResponse {
	db = db.Model(&schema.WebFuzzerResponse{}).Where("web_fuzzer_task_id = ?", id)
	outC := make(chan *schema.WebFuzzerResponse)
	yieldWebFuzzerResponsesToChan(outC, db, ctx)
	return outC
}

func yieldWebFuzzerResponsesToChan(outC chan *schema.WebFuzzerResponse, db *gorm.DB, ctx context.Context) {
	go func() {
		defer close(outC)

		page := 1
		for {
			var items []*schema.WebFuzzerResponse
			if _, b := bizhelper.NewPagination(&bizhelper.Param{
				DB:    db,
				Page:  page,
				Limit: 1000,
			}, &items); b.Error != nil {
				log.Errorf("paging failed: %s", b.Error)
				return
			}

			page++

			for _, d := range items {
				select {
				case <-ctx.Done():
					return
				case outC <- d:
				}
			}

			if len(items) < 1000 {
				return
			}
		}
	}()
}
