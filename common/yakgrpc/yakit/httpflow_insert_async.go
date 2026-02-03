package yakit

import (
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/schema"
)

const (
	httpFlowInsertAsyncBuffer = 8192
	httpFlowInsertBatchSize   = 1000
	httpFlowInsertFlushEvery  = 10 * time.Millisecond
)

type httpFlowInsertJob struct {
	flow   *schema.HTTPFlow
	finish []func()
	done   chan struct{}
}

type httpFlowInsertSpec struct {
	name  string
	value func(*schema.HTTPFlow) any
}

type httpFlowInsertPlan struct {
	table            string
	columns          []string
	specs            []httpFlowInsertSpec
	valuePlaceholder string
	maxRows          int
}

var (
	httpFlowInsertOnce sync.Once
	httpFlowInsertCh   = make(chan httpFlowInsertJob, httpFlowInsertAsyncBuffer)

	httpFlowInsertPlanOnce  sync.Once
	httpFlowInsertPlanValue httpFlowInsertPlan
	httpFlowInsertPlanErr   error
)

func enqueueHTTPFlowInsertAsync(flow *schema.HTTPFlow, finish []func()) error {
	if flow == nil {
		return nil
	}
	httpFlowInsertOnce.Do(startHTTPFlowInsertBatcher)
	handlers := make([]func(), len(finish))
	copy(handlers, finish)
	job := httpFlowInsertJob{flow: flow, finish: handlers}
	select {
	case httpFlowInsertCh <- job:
		return nil
	default:
		return enqueueHTTPFlowInsertFallback(flow, handlers)
	}
}

func enqueueHTTPFlowInsertFallback(flow *schema.HTTPFlow, finish []func()) error {
	DBSaveAsyncChannel <- func(db *gorm.DB) error {
		err := InsertHTTPFlow(db, flow)
		for _, h := range finish {
			h()
		}
		return err
	}
	return nil
}

func flushHTTPFlowInsertQueue() {
	httpFlowInsertOnce.Do(startHTTPFlowInsertBatcher)
	done := make(chan struct{})
	httpFlowInsertCh <- httpFlowInsertJob{done: done}
	<-done
}

func startHTTPFlowInsertBatcher() {
	go func() {
		batch := make([]httpFlowInsertJob, 0, httpFlowInsertBatchSize)
		timer := time.NewTimer(httpFlowInsertFlushEvery)
		defer timer.Stop()
		flush := func() {
			if len(batch) == 0 {
				return
			}
			jobs := make([]httpFlowInsertJob, len(batch))
			copy(jobs, batch)
			batch = batch[:0]
			DBSaveAsyncChannel <- func(db *gorm.DB) error {
				return insertHTTPFlowBatch(db, jobs)
			}
		}
		for {
			select {
			case job := <-httpFlowInsertCh:
				if job.done != nil {
					flush()
					close(job.done)
					continue
				}
				batch = append(batch, job)
				if len(batch) >= httpFlowInsertBatchSize {
					if !timer.Stop() {
						select {
						case <-timer.C:
						default:
						}
					}
					flush()
					timer.Reset(httpFlowInsertFlushEvery)
				}
			case <-timer.C:
				flush()
				timer.Reset(httpFlowInsertFlushEvery)
			}
		}
	}()
}

func insertHTTPFlowBatch(db *gorm.DB, jobs []httpFlowInsertJob) error {
	if db == nil || len(jobs) == 0 {
		return nil
	}
	if !isSQLiteDialect(db) {
		return insertHTTPFlowBatchWithGorm(db, jobs)
	}
	plan, err := getHTTPFlowInsertPlan(db)
	if err != nil || len(plan.specs) == 0 {
		return insertHTTPFlowBatchWithGorm(db, jobs)
	}
	return insertHTTPFlowBatchBulk(db, plan, jobs)
}

func insertHTTPFlowBatchWithGorm(db *gorm.DB, jobs []httpFlowInsertJob) error {
	tx := db.Begin()
	if tx.Error != nil {
		return tx.Error
	}
	for _, job := range jobs {
		if job.flow == nil {
			continue
		}
		job.flow.ID = 0
		if err := tx.Model(&schema.HTTPFlow{}).Save(job.flow).Error; err != nil {
			_ = tx.Rollback().Error
			return err
		}
	}
	if err := tx.Commit().Error; err != nil {
		return err
	}
	for _, job := range jobs {
		if job.flow == nil || job.flow.ID == 0 {
			continue
		}
		enqueueHTTPFlowFTSUpdate(job.flow)
		for _, h := range job.finish {
			h()
		}
	}
	return nil
}

func insertHTTPFlowBatchBulk(db *gorm.DB, plan httpFlowInsertPlan, jobs []httpFlowInsertJob) error {
	validJobs := make([]httpFlowInsertJob, 0, len(jobs))
	now := gorm.NowFunc()
	for _, job := range jobs {
		if job.flow == nil {
			continue
		}
		job.flow.ID = 0
		if err := job.flow.BeforeSave(); err != nil {
			return err
		}
		if job.flow.CreatedAt.IsZero() {
			job.flow.CreatedAt = now
		}
		if job.flow.UpdatedAt.IsZero() {
			job.flow.UpdatedAt = job.flow.CreatedAt
		}
		validJobs = append(validJobs, job)
	}
	if len(validJobs) == 0 {
		return nil
	}
	tx := db.Begin()
	if tx.Error != nil {
		return tx.Error
	}
	chunkSize := plan.maxRows
	if chunkSize <= 0 {
		chunkSize = 1
	}
	for start := 0; start < len(validJobs); start += chunkSize {
		end := start + chunkSize
		if end > len(validJobs) {
			end = len(validJobs)
		}
		if err := insertHTTPFlowChunk(tx, plan, validJobs[start:end]); err != nil {
			_ = tx.Rollback().Error
			return err
		}
	}
	if err := tx.Commit().Error; err != nil {
		return err
	}
	for _, job := range validJobs {
		if job.flow == nil || job.flow.ID == 0 {
			continue
		}
		enqueueHTTPFlowFTSUpdate(job.flow)
		_ = job.flow.AfterCreate(nil)
		for _, h := range job.finish {
			h()
		}
	}
	return nil
}

func insertHTTPFlowChunk(tx *gorm.DB, plan httpFlowInsertPlan, jobs []httpFlowInsertJob) error {
	if tx == nil || len(jobs) == 0 {
		return nil
	}
	placeholders := make([]string, len(jobs))
	args := make([]any, 0, len(jobs)*len(plan.specs))
	for i, job := range jobs {
		placeholders[i] = plan.valuePlaceholder
		for _, spec := range plan.specs {
			args = append(args, spec.value(job.flow))
		}
	}
	stmt := fmt.Sprintf(
		"INSERT INTO %s (%s) VALUES %s",
		plan.table,
		strings.Join(plan.columns, ","),
		strings.Join(placeholders, ","),
	)
	if err := tx.Exec(stmt, args...).Error; err != nil {
		return err
	}
	var lastID int64
	if err := tx.Raw("SELECT last_insert_rowid()").Row().Scan(&lastID); err != nil {
		return err
	}
	if lastID <= 0 {
		return fmt.Errorf("last_insert_rowid returned %d", lastID)
	}
	firstID := lastID - int64(len(jobs)) + 1
	for i, job := range jobs {
		if job.flow == nil {
			continue
		}
		job.flow.ID = uint(firstID + int64(i))
	}
	return nil
}

func getHTTPFlowInsertPlan(db *gorm.DB) (httpFlowInsertPlan, error) {
	httpFlowInsertPlanOnce.Do(func() {
		if db == nil {
			httpFlowInsertPlanErr = fmt.Errorf("nil db")
			return
		}
		columns, err := fetchHTTPFlowColumnsForInsert(db)
		if err != nil {
			httpFlowInsertPlanErr = err
			return
		}
		specs := httpFlowInsertSpecsForInsert(columns)
		if len(specs) == 0 {
			httpFlowInsertPlanErr = fmt.Errorf("no insertable columns for http_flows")
			return
		}
		scope := db.NewScope(&schema.HTTPFlow{})
		quotedColumns := make([]string, 0, len(specs))
		for _, spec := range specs {
			quotedColumns = append(quotedColumns, scope.Quote(spec.name))
		}
		valuePlaceholder := "(" + strings.TrimRight(strings.Repeat("?,", len(specs)), ",") + ")"
		maxVars := fetchSQLiteMaxVariables(db)
		maxRows := maxVars / len(specs)
		if maxRows < 1 {
			maxRows = 1
		}
		httpFlowInsertPlanValue = httpFlowInsertPlan{
			table:            scope.QuotedTableName(),
			columns:          quotedColumns,
			specs:            specs,
			valuePlaceholder: valuePlaceholder,
			maxRows:          maxRows,
		}
	})
	return httpFlowInsertPlanValue, httpFlowInsertPlanErr
}

func fetchSQLiteMaxVariables(db *gorm.DB) int {
	if db == nil {
		return 999
	}
	rows, err := db.Raw("PRAGMA compile_options;").Rows()
	if err != nil {
		return 999
	}
	defer rows.Close()
	for rows.Next() {
		var opt string
		if err := rows.Scan(&opt); err != nil {
			return 999
		}
		if strings.HasPrefix(opt, "MAX_VARIABLE_NUMBER=") {
			raw := strings.TrimPrefix(opt, "MAX_VARIABLE_NUMBER=")
			if v, err := strconv.Atoi(raw); err == nil && v > 0 {
				return v
			}
		}
	}
	return 999
}

func fetchHTTPFlowColumnsForInsert(db *gorm.DB) (map[string]struct{}, error) {
	rows, err := db.Raw(`PRAGMA table_info(http_flows);`).Rows()
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	columns := make(map[string]struct{})
	for rows.Next() {
		var cid, notnull, pk int
		var name, colType string
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &colType, &notnull, &dflt, &pk); err != nil {
			return nil, err
		}
		columns[name] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return columns, nil
}

func httpFlowInsertSpecsForInsert(columns map[string]struct{}) []httpFlowInsertSpec {
	candidates := []httpFlowInsertSpec{
		{name: "created_at", value: func(f *schema.HTTPFlow) any { return f.CreatedAt }},
		{name: "updated_at", value: func(f *schema.HTTPFlow) any { return f.UpdatedAt }},
		{name: "deleted_at", value: func(f *schema.HTTPFlow) any { return f.DeletedAt }},
		{name: "hidden_index", value: func(f *schema.HTTPFlow) any { return f.HiddenIndex }},
		{name: "no_fix_content_length", value: func(f *schema.HTTPFlow) any { return f.NoFixContentLength }},
		{name: "hash", value: func(f *schema.HTTPFlow) any { return f.Hash }},
		{name: "is_https", value: func(f *schema.HTTPFlow) any { return f.IsHTTPS }},
		{name: "url", value: func(f *schema.HTTPFlow) any { return f.Url }},
		{name: "path", value: func(f *schema.HTTPFlow) any { return f.Path }},
		{name: "method", value: func(f *schema.HTTPFlow) any { return f.Method }},
		{name: "request_length", value: func(f *schema.HTTPFlow) any { return f.RequestLength }},
		{name: "body_length", value: func(f *schema.HTTPFlow) any { return f.BodyLength }},
		{name: "content_type", value: func(f *schema.HTTPFlow) any { return f.ContentType }},
		{name: "status_code", value: func(f *schema.HTTPFlow) any { return f.StatusCode }},
		{name: "source_type", value: func(f *schema.HTTPFlow) any { return f.SourceType }},
		{name: "request", value: func(f *schema.HTTPFlow) any { return f.Request }},
		{name: "response", value: func(f *schema.HTTPFlow) any { return f.Response }},
		{name: "response_length", value: func(f *schema.HTTPFlow) any { return f.ResponseLength }},
		{name: "duration", value: func(f *schema.HTTPFlow) any { return f.Duration }},
		{name: "get_params_total", value: func(f *schema.HTTPFlow) any { return f.GetParamsTotal }},
		{name: "post_params_total", value: func(f *schema.HTTPFlow) any { return f.PostParamsTotal }},
		{name: "cookie_params_total", value: func(f *schema.HTTPFlow) any { return f.CookieParamsTotal }},
		{name: "ip_address", value: func(f *schema.HTTPFlow) any { return f.IPAddress }},
		{name: "remote_addr", value: func(f *schema.HTTPFlow) any { return f.RemoteAddr }},
		{name: "ip_integer", value: func(f *schema.HTTPFlow) any { return f.IPInteger }},
		{name: "tags", value: func(f *schema.HTTPFlow) any { return f.Tags }},
		{name: "payload", value: func(f *schema.HTTPFlow) any { return f.Payload }},
		{name: "is_websocket", value: func(f *schema.HTTPFlow) any { return f.IsWebsocket }},
		{name: "websocket_hash", value: func(f *schema.HTTPFlow) any { return f.WebsocketHash }},
		{name: "runtime_id", value: func(f *schema.HTTPFlow) any { return f.RuntimeId }},
		{name: "from_plugin", value: func(f *schema.HTTPFlow) any { return f.FromPlugin }},
		{name: "process_name", value: func(f *schema.HTTPFlow) any { return f.ProcessName }},
		{name: "is_read_too_slow_response", value: func(f *schema.HTTPFlow) any { return f.IsReadTooSlowResponse }},
		{name: "is_too_large_response", value: func(f *schema.HTTPFlow) any { return f.IsTooLargeResponse }},
		{name: "too_large_response_header_file", value: func(f *schema.HTTPFlow) any { return f.TooLargeResponseHeaderFile }},
		{name: "too_large_response_body_file", value: func(f *schema.HTTPFlow) any { return f.TooLargeResponseBodyFile }},
		{name: "upload_online", value: func(f *schema.HTTPFlow) any { return f.UploadOnline }},
		{name: "host", value: func(f *schema.HTTPFlow) any { return f.Host }},
	}
	specs := make([]httpFlowInsertSpec, 0, len(candidates))
	for _, spec := range candidates {
		if _, ok := columns[spec.name]; ok {
			specs = append(specs, spec)
		}
	}
	return specs
}
