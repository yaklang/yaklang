package yakit

import (
	"database/sql"
	"strings"
	"sync"
	"time"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

const (
	httpFlowFTSAsyncBuffer      = 8192
	httpFlowFTSBatchSize        = 200
	httpFlowFTSFlushEvery       = 100 * time.Millisecond
	httpFlowFTSSlowLogThreshold = 2 * time.Second
	httpFlowFTSDeferThreshold   = 200
	httpFlowFTSDeferFlushEvery  = 500 * time.Millisecond
	httpFlowFTSDeferBatchSize   = 500
	httpFlowFTSDeferMaxIDs      = 200000
)

type httpFlowFTSJob struct {
	id         uint
	request    string
	response   string
	url        string
	path       string
	tags       string
	remoteAddr string
	hasPayload bool
}

var (
	httpFlowFTSAsyncOnce sync.Once
	httpFlowFTSAsyncCh   = make(chan httpFlowFTSJob, httpFlowFTSAsyncBuffer)
	httpFlowFTSDeferOnce sync.Once

	httpFlowFTSDeferMu       sync.Mutex
	httpFlowFTSDeferred      = make(map[uint]struct{})
	httpFlowFTSDeferThrottle = utils.NewThrottle(5.0)
)

func enqueueHTTPFlowFTSUpdate(flow *schema.HTTPFlow) {
	if !consts.IsHTTPFlowFTSAsyncEnabled() {
		return
	}
	if flow == nil || flow.ID == 0 {
		return
	}
	httpFlowFTSAsyncOnce.Do(startHTTPFlowFTSAsyncWorker)
	job := buildHTTPFlowFTSJob(flow)
	if shouldDeferHTTPFlowFTSUpdate() {
		deferHTTPFlowFTSUpdate(job.id)
		return
	}
	select {
	case httpFlowFTSAsyncCh <- job:
	default:
		log.Warnf("httpflow fts async queue full, drop update (id=%d)", flow.ID)
	}
}

func shouldDeferHTTPFlowFTSUpdate() bool {
	return len(DBSaveAsyncChannel) > httpFlowFTSDeferThreshold
}

func deferHTTPFlowFTSUpdate(rowID uint) {
	httpFlowFTSDeferOnce.Do(startHTTPFlowFTSDeferredWorker)
	httpFlowFTSDeferMu.Lock()
	defer httpFlowFTSDeferMu.Unlock()
	if len(httpFlowFTSDeferred) >= httpFlowFTSDeferMaxIDs {
		httpFlowFTSDeferThrottle(func() {
			log.Warnf("httpflow fts deferred queue full, dropping updates (size=%d)", len(httpFlowFTSDeferred))
		})
		return
	}
	httpFlowFTSDeferred[rowID] = struct{}{}
}

func startHTTPFlowFTSDeferredWorker() {
	go func() {
		ticker := time.NewTicker(httpFlowFTSDeferFlushEvery)
		defer ticker.Stop()
		for range ticker.C {
			if len(DBSaveAsyncChannel) > httpFlowFTSDeferThreshold {
				continue
			}
			flushDeferredFTSUpdates()
		}
	}()
}

func flushDeferredFTSUpdates() {
	ids := make([]uint, 0, httpFlowFTSDeferBatchSize)
	httpFlowFTSDeferMu.Lock()
	for id := range httpFlowFTSDeferred {
		ids = append(ids, id)
		delete(httpFlowFTSDeferred, id)
		if len(ids) >= httpFlowFTSDeferBatchSize {
			break
		}
	}
	httpFlowFTSDeferMu.Unlock()
	if len(ids) == 0 {
		return
	}
	for _, id := range ids {
		job := httpFlowFTSJob{id: id}
		select {
		case httpFlowFTSAsyncCh <- job:
		default:
			httpFlowFTSDeferMu.Lock()
			httpFlowFTSDeferred[id] = struct{}{}
			httpFlowFTSDeferMu.Unlock()
			return
		}
	}
}

func startHTTPFlowFTSAsyncWorker() {
	go func() {
		batch := make([]httpFlowFTSJob, 0, httpFlowFTSBatchSize)
		timer := time.NewTimer(httpFlowFTSFlushEvery)
		defer timer.Stop()
		flush := func() {
			if len(batch) == 0 {
				return
			}
			rows := make([]httpFlowFTSJob, len(batch))
			copy(rows, batch)
			batch = batch[:0]
			enqueueHTTPFlowFTSBatch(rows)
		}
		for {
			select {
			case job := <-httpFlowFTSAsyncCh:
				batch = append(batch, job)
				if len(batch) >= httpFlowFTSBatchSize {
					if !timer.Stop() {
						select {
						case <-timer.C:
						default:
						}
					}
					flush()
					timer.Reset(httpFlowFTSFlushEvery)
				}
			case <-timer.C:
				flush()
				timer.Reset(httpFlowFTSFlushEvery)
			}
		}
	}()
}

func enqueueHTTPFlowFTSBatch(rows []httpFlowFTSJob) {
	select {
	case DBSaveAsyncChannel <- func(db *gorm.DB) error {
		return applyHTTPFlowFTSBatch(db, rows)
	}:
	default:
		log.Warnf("httpflow fts async db queue full, drop batch (size=%d)", len(rows))
	}
}

func applyHTTPFlowFTSBatch(db *gorm.DB, rows []httpFlowFTSJob) error {
	if db == nil || len(rows) == 0 {
		return nil
	}
	start := time.Now()
	tx := db.Begin()
	if tx.Error != nil {
		return tx.Error
	}
	withPayload := make([]httpFlowFTSJob, 0, len(rows))
	withoutPayload := make([]uint, 0, len(rows))
	for _, row := range rows {
		if row.hasPayload {
			withPayload = append(withPayload, row)
		} else if row.id > 0 {
			withoutPayload = append(withoutPayload, row.id)
		}
	}
	if len(withPayload) > 0 {
		if err := applyHTTPFlowFTSBatchPayload(tx, withPayload); err != nil {
			_ = tx.Rollback().Error
			return err
		}
	}
	if len(withoutPayload) > 0 {
		if err := applyHTTPFlowFTSBatchSelect(tx, withoutPayload); err != nil {
			_ = tx.Rollback().Error
			return err
		}
	}
	elapsed := time.Since(start)
	if elapsed > httpFlowFTSSlowLogThreshold {
		stats := fetchHTTPFlowBatchStats(tx, withoutPayload)
		log.Warnf(
			"httpflow fts batch slow: %s rows=%d fts_queue=%d db_queue=%d req_max=%d rsp_max=%d req_avg=%.0f rsp_avg=%.0f",
			elapsed,
			len(rows),
			len(httpFlowFTSAsyncCh),
			len(DBSaveAsyncChannel),
			stats.reqMax,
			stats.rspMax,
			stats.reqAvg,
			stats.rspAvg,
		)
	}
	return tx.Commit().Error
}

func applyHTTPFlowFTSBatchPayload(tx *gorm.DB, rows []httpFlowFTSJob) error {
	if tx == nil || len(rows) == 0 {
		return nil
	}
	var builder strings.Builder
	builder.WriteString(`INSERT OR REPLACE INTO "http_flows_fts"(rowid, request, response, url, path, tags, remote_addr) VALUES `)
	args := make([]interface{}, 0, len(rows)*7)
	count := 0
	for _, row := range rows {
		if row.id == 0 {
			continue
		}
		if count > 0 {
			builder.WriteString(",")
		}
		builder.WriteString("(?,?,?,?,?,?,?)")
		args = append(args, row.id, row.request, row.response, row.url, row.path, row.tags, row.remoteAddr)
		count++
	}
	if count == 0 {
		return nil
	}
	builder.WriteString(";")
	return tx.Exec(builder.String(), args...).Error
}

func applyHTTPFlowFTSBatchSelect(tx *gorm.DB, rows []uint) error {
	if tx == nil || len(rows) == 0 {
		return nil
	}
	var builder strings.Builder
	builder.WriteString(`INSERT OR REPLACE INTO "http_flows_fts"(rowid, request, response, url, path, tags, remote_addr)
SELECT id, request, response, url, path, tags, remote_addr FROM "http_flows" WHERE id IN (`)
	args := make([]interface{}, 0, len(rows))
	for i, id := range rows {
		if i > 0 {
			builder.WriteString(",")
		}
		builder.WriteString("?")
		args = append(args, id)
	}
	builder.WriteString(");")
	return tx.Exec(builder.String(), args...).Error
}

type ftsBatchStats struct {
	reqMax int64
	rspMax int64
	reqAvg float64
	rspAvg float64
}

func fetchHTTPFlowBatchStats(tx *gorm.DB, rows []uint) ftsBatchStats {
	stats := ftsBatchStats{}
	if tx == nil || len(rows) == 0 {
		return stats
	}
	var reqMax, rspMax sql.NullInt64
	var reqAvg, rspAvg sql.NullFloat64
	row := tx.Raw(
		`SELECT max(length(request)), max(length(response)), avg(length(request)), avg(length(response))
FROM http_flows WHERE id IN (?)`,
		rows,
	).Row()
	if err := row.Scan(&reqMax, &rspMax, &reqAvg, &rspAvg); err != nil {
		return stats
	}
	if reqMax.Valid {
		stats.reqMax = reqMax.Int64
	}
	if rspMax.Valid {
		stats.rspMax = rspMax.Int64
	}
	if reqAvg.Valid {
		stats.reqAvg = reqAvg.Float64
	}
	if rspAvg.Valid {
		stats.rspAvg = rspAvg.Float64
	}
	return stats
}

func buildHTTPFlowFTSJob(flow *schema.HTTPFlow) httpFlowFTSJob {
	job := httpFlowFTSJob{id: flow.ID}
	if flow == nil || flow.ID == 0 {
		return job
	}
	// Use flow payload when request is present; otherwise fall back to SELECT.
	if flow.Request != "" {
		job.request = flow.Request
		job.response = flow.Response
		job.url = flow.Url
		job.path = flow.Path
		job.tags = flow.Tags
		job.remoteAddr = flow.RemoteAddr
		job.hasPayload = true
	}
	return job
}
