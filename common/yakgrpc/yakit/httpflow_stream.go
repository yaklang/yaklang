package yakit

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/segmentio/ksuid"
	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/utils/lowhttp/httpctx"
)

const httpFlowStreamProgressInterval = 500 * time.Millisecond

// HTTPFlowStreamRecorder persists response metadata as soon as headers arrive
// and appends the streaming body to a spill file while the connection remains
// open. It intentionally keeps database writes off the response hot path.
type HTTPFlowStreamRecorder struct {
	mu sync.Mutex

	db          *gorm.DB
	insertFlow  func(*gorm.DB, *schema.HTTPFlow) error
	req         *http.Request
	initialFlow *schema.HTTPFlow
	bodyFile    *os.File
	headerFile  string
	bodyPath    string
	startedAt   time.Time
	bodyLength  int64
	writeErr    error
	closed      bool
	insertErr   error

	insertDone chan struct{}
	stopCh     chan struct{}
	doneCh     chan struct{}
	closeOnce  sync.Once
	closeErr   error
}

func NewHTTPFlowStreamRecorder(
	db *gorm.DB,
	isHTTPS bool,
	req *http.Request,
	rsp *http.Response,
	headerBytes []byte,
) (_ *HTTPFlowStreamRecorder, retErr error) {
	return newHTTPFlowStreamRecorder(db, isHTTPS, req, rsp, headerBytes, InsertHTTPFlow)
}

func newHTTPFlowStreamRecorder(
	db *gorm.DB,
	isHTTPS bool,
	req *http.Request,
	rsp *http.Response,
	headerBytes []byte,
	insertFlow func(*gorm.DB, *schema.HTTPFlow) error,
) (_ *HTTPFlowStreamRecorder, retErr error) {
	if db == nil {
		return nil, utils.Error("create HTTP stream recorder: project database is nil")
	}
	if req == nil || rsp == nil {
		return nil, utils.Error("create HTTP stream recorder: request or response is nil")
	}
	if insertFlow == nil {
		return nil, utils.Error("create HTTP stream recorder: insert function is nil")
	}

	suffix := fmt.Sprintf("%s-%s", time.Now().Format(utils.DatetimePretty()), ksuid.New().String())
	headerFP, err := utils.OpenTempFile(fmt.Sprintf("stream-response-header-%s.txt", suffix))
	if err != nil {
		return nil, utils.Wrap(err, "create HTTP stream response header file")
	}
	headerPath := headerFP.Name()
	defer func() {
		_ = headerFP.Close()
		if retErr != nil {
			_ = os.Remove(headerPath)
		}
	}()
	if _, err := headerFP.Write(headerBytes); err != nil {
		return nil, utils.Wrap(err, "write HTTP stream response header")
	}
	if err := headerFP.Sync(); err != nil {
		return nil, utils.Wrap(err, "sync HTTP stream response header")
	}

	bodyFP, err := utils.OpenTempFile(fmt.Sprintf("stream-response-body-%s.bin", suffix))
	if err != nil {
		return nil, utils.Wrap(err, "create HTTP stream response body file")
	}
	bodyPath := bodyFP.Name()
	defer func() {
		if retErr != nil {
			_ = bodyFP.Close()
			_ = os.Remove(bodyPath)
		}
	}()

	requestURL := httpctx.GetRequestURL(req)
	if requestURL == "" {
		if parsed, err := lowhttp.ExtractURLFromHTTPRequest(req, isHTTPS); err == nil && parsed != nil {
			requestURL = parsed.String()
		}
	}
	remoteAddr := httpctx.GetRemoteAddr(req)
	flow, err := CreateHTTPFlowFromHTTPWithNoRspSaved(isHTTPS, req, "mitm", requestURL, remoteAddr)
	if err != nil {
		return nil, utils.Wrap(err, "create initial HTTP stream flow")
	}
	flow.SetResponse(string(headerBytes))
	flow.StatusCode = int64(rsp.StatusCode)
	flow.ContentType = rsp.Header.Get("Content-Type")
	flow.BodyLength = 0
	flow.Duration = 0
	flow.IsReadTooSlowResponse = true
	flow.IsTooLargeResponse = true
	flow.TooLargeResponseHeaderFile = headerPath
	flow.TooLargeResponseBodyFile = bodyPath
	if name := httpctx.GetProcessName(req); name != "" {
		flow.ProcessName.String = filepath.Base(name)
		flow.ProcessName.Valid = true
	}
	flow.Hash = flow.CalcHash()

	httpctx.SetResponseReadTooSlow(req, true)
	httpctx.SetResponseTooLarge(req, true)
	httpctx.SetResponseTooLargeHeaderFile(req, headerPath)
	httpctx.SetResponseTooLargeBodyFile(req, bodyPath)

	recorder := &HTTPFlowStreamRecorder{
		db:          db,
		insertFlow:  insertFlow,
		req:         req,
		initialFlow: flow,
		bodyFile:    bodyFP,
		headerFile:  headerPath,
		bodyPath:    bodyPath,
		startedAt:   time.Now(),
		insertDone:  make(chan struct{}),
		stopCh:      make(chan struct{}),
		doneCh:      make(chan struct{}),
	}
	go recorder.insertInitialFlow()
	go recorder.runProgressUpdater()
	return recorder, nil
}

func (r *HTTPFlowStreamRecorder) insertInitialFlow() {
	err := r.insertFlow(r.db, r.initialFlow)
	r.mu.Lock()
	r.insertErr = err
	flowID := r.initialFlow.ID
	r.mu.Unlock()
	requestPath := ""
	if r.req != nil && r.req.URL != nil {
		requestPath = r.req.URL.Path
	}
	if err != nil {
		log.Warnf("insert initial HTTP stream flow failed: path=%s error=%v", requestPath, err)
	} else {
		log.Debugf("MITM SSE flow persisted before EOF: flow_id=%d path=%s", flowID, requestPath)
	}
	close(r.insertDone)
}

func (r *HTTPFlowStreamRecorder) waitForInsert() error {
	if r == nil {
		return nil
	}
	<-r.insertDone
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.insertErr
}

func (r *HTTPFlowStreamRecorder) Write(p []byte) (int, error) {
	if r == nil || len(p) == 0 {
		return len(p), nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed || r.bodyFile == nil || r.writeErr != nil {
		return len(p), nil
	}
	n, err := r.bodyFile.Write(p)
	r.bodyLength += int64(n)
	if err == nil && n != len(p) {
		err = io.ErrShortWrite
	}
	if err != nil {
		r.writeErr = err
		log.Warnf("append HTTP stream response body failed: path=%s error=%v", r.bodyPath, err)
	}

	// Capturing must never break or delay forwarding with a recorder error.
	return len(p), nil
}

func (r *HTTPFlowStreamRecorder) runProgressUpdater() {
	defer close(r.doneCh)
	ticker := time.NewTicker(httpFlowStreamProgressInterval)
	defer ticker.Stop()
	lastSync := time.Now()
	for {
		select {
		case <-ticker.C:
			syncFile := time.Since(lastSync) >= time.Second
			r.updateProgress(syncFile)
			if syncFile {
				lastSync = time.Now()
			}
		case <-r.stopCh:
			return
		}
	}
}

func (r *HTTPFlowStreamRecorder) updateProgress(syncFile bool) {
	if r == nil || r.db == nil || r.initialFlow == nil {
		return
	}
	select {
	case <-r.insertDone:
	default:
		return
	}
	r.mu.Lock()
	if r.insertErr != nil || r.initialFlow.ID == 0 {
		r.mu.Unlock()
		return
	}
	if syncFile && r.bodyFile != nil && r.writeErr == nil {
		if err := r.bodyFile.Sync(); err != nil {
			r.writeErr = err
			log.Warnf("sync HTTP stream response body failed: flow_id=%d error=%v", r.initialFlow.ID, err)
		}
	}
	bodyLength := r.bodyLength
	duration := time.Since(r.startedAt)
	flowID := r.initialFlow.ID
	r.mu.Unlock()

	err := r.db.Model(&schema.HTTPFlow{}).Where("id = ?", flowID).UpdateColumns(map[string]any{
		"body_length":                    bodyLength,
		"duration":                       int64(duration),
		"is_read_too_slow_response":      true,
		"is_too_large_response":           true,
		"too_large_response_header_file": r.headerFile,
		"too_large_response_body_file":   r.bodyPath,
		"updated_at":                     time.Now(),
	}).Error
	if err != nil {
		log.Warnf("update HTTP stream flow progress failed: flow_id=%d error=%v", flowID, err)
	}
}

func (r *HTTPFlowStreamRecorder) Close() error {
	if r == nil {
		return nil
	}
	r.closeOnce.Do(func() {
		close(r.stopCh)
		<-r.doneCh

		r.mu.Lock()
		r.closed = true
		if r.bodyFile != nil {
			if err := r.bodyFile.Sync(); err != nil && r.writeErr == nil {
				r.writeErr = err
			}
			if err := r.bodyFile.Close(); err != nil && r.writeErr == nil {
				r.writeErr = err
			}
			r.bodyFile = nil
		}
		bodyLength := r.bodyLength
		r.closeErr = r.writeErr
		r.mu.Unlock()

		httpctx.SetResponseTooLargeSize(r.req, bodyLength)
		r.updateProgress(false)
	})
	return r.closeErr
}

func (r *HTTPFlowStreamRecorder) FlowID() uint {
	if r == nil || r.initialFlow == nil {
		return 0
	}
	if err := r.waitForInsert(); err != nil {
		return 0
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.initialFlow.ID
}

func (r *HTTPFlowStreamRecorder) BodyFile() string {
	if r == nil {
		return ""
	}
	return r.bodyPath
}

func (r *HTTPFlowStreamRecorder) HeaderFile() string {
	if r == nil {
		return ""
	}
	return r.headerFile
}

func (r *HTTPFlowStreamRecorder) BodyLength() int64 {
	if r == nil {
		return 0
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.bodyLength
}

// Finalize updates the header-first row instead of inserting a duplicate after
// the ordinary MITM response mirror finally observes EOF.
func (r *HTTPFlowStreamRecorder) Finalize(flow *schema.HTTPFlow) error {
	if r == nil || flow == nil || r.initialFlow == nil {
		return utils.Error("finalize HTTP stream flow: invalid recorder or flow")
	}
	_ = r.Close()
	if err := r.waitForInsert(); err != nil {
		return utils.Wrap(err, "finalize HTTP stream flow after initial insert")
	}

	r.mu.Lock()
	bodyLength := r.bodyLength
	duration := time.Since(r.startedAt)
	r.mu.Unlock()

	flow.Model = r.initialFlow.Model
	flow.HiddenIndex = r.initialFlow.HiddenIndex
	flow.Response = r.initialFlow.Response
	flow.StatusCode = r.initialFlow.StatusCode
	flow.ContentType = r.initialFlow.ContentType
	flow.BodyLength = bodyLength
	flow.Duration = int64(duration)
	flow.IsReadTooSlowResponse = true
	flow.IsTooLargeResponse = true
	flow.TooLargeResponseHeaderFile = r.headerFile
	flow.TooLargeResponseBodyFile = r.bodyPath
	return SaveHTTPFlow(r.db, flow)
}

func (r *HTTPFlowStreamRecorder) Drop() error {
	if r == nil || r.initialFlow == nil {
		return nil
	}
	_ = r.Close()
	if err := r.waitForInsert(); err != nil {
		_ = os.Remove(r.headerFile)
		_ = os.Remove(r.bodyPath)
		return err
	}
	err := DeleteHTTPFlowByID(r.db, int64(r.initialFlow.ID))
	_ = os.Remove(r.headerFile)
	_ = os.Remove(r.bodyPath)
	return err
}
