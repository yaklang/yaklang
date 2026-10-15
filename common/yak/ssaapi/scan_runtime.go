package ssaapi

import (
	"io"
	"sync"

	"github.com/google/uuid"
	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

// ScanRuntime decides which finding survives, then hands that one decision to
// its consumers. Database saving and reporting are ordinary handlers; neither
// has a dedicated path or persistence flag on the runtime.
type ScanRuntime struct {
	ID string

	collect            *schema.RiskCollect
	riskUpdateHandlers []schema.RiskUpdateHandler
	mu                 sync.Mutex
	closeOnce          sync.Once
	closeErr           error
}

func NewScanRuntime() *ScanRuntime {
	return &ScanRuntime{ID: uuid.NewString(), collect: schema.NewRiskCollect()}
}

func (r *ScanRuntime) SetRuleLevelFunc(fn func(ruleName string) int) {
	if r != nil && r.collect != nil {
		r.collect.SetRuleLevelFunc(fn)
	}
}

// RegisterRiskUpdateHandler registers a consumer before scanning starts.
func (r *ScanRuntime) RegisterRiskUpdateHandler(handler schema.RiskUpdateHandler) {
	if r == nil || utils.IsNil(handler) {
		return
	}
	r.riskUpdateHandlers = append(r.riskUpdateHandlers, handler)
}

// SubmitRisk reports whether this runtime owns the decision, including a
// dropped finding. Serialize decision and delivery together so concurrent
// rules cannot deliver a cover before the decision it replaces. audits are
// optional graph writers; they travel on the update item, never on the row.
func (r *ScanRuntime) SubmitRisk(risk *schema.SSARisk, audits ...func(*gorm.DB) error) bool {
	if r == nil || risk == nil || r.collect == nil {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	// The item is finished here. Handlers trust it and do not prepare,
	// coalesce, or guess a mode from a zero id. This call does not wait for a
	// handler to persist anything.
	prepareRiskRow(risk)
	item, ok := r.collect.Submit(risk)
	if ok {
		item.Audit = audits
		for _, handler := range r.riskUpdateHandlers {
			if err := handler.ApplyRiskUpdate(item); err != nil {
				log.Errorf("apply risk update failed: %v", err)
			}
		}
	}
	return true
}

// Flush waits for handlers with buffered work. Call it before publishing a
// durable task status; handlers without buffers have nothing to flush.
func (r *ScanRuntime) Flush() (err error) {
	if r != nil {
		for _, handler := range r.riskUpdateHandlers {
			if flusher, ok := handler.(interface{ Flush() error }); ok {
				err = utils.JoinErrors(err, flusher.Flush())
			}
		}
	}
	return
}

// Close finishes the registered consumers. The call that creates a runtime
// owns this lifecycle; nested stages just borrow it.
func (r *ScanRuntime) Close() error {
	if r == nil {
		return nil
	}
	r.closeOnce.Do(func() {
		for _, handler := range r.riskUpdateHandlers {
			if closer, ok := handler.(io.Closer); ok {
				r.closeErr = utils.JoinErrors(r.closeErr, closer.Close())
			}
		}
	})
	return r.closeErr
}
