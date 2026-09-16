package browser

import (
	"fmt"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
	"github.com/yaklang/yaklang/common/log"
)

// JavaScriptDialog describes a native alert/confirm/prompt/beforeunload waiting for a decision.
type JavaScriptDialog struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

func enableJavaScriptDialogWatcher(page *rod.Page, bp *BrowserPage) {
	// Subscribe before navigation; brief yield so CDP listener is active.
	go page.EachEvent(func(e *proto.PageJavascriptDialogOpening) {
		bp.setPendingDialog(&JavaScriptDialog{
			Type:    string(e.Type),
			Message: e.Message,
		})
		log.Infof("javascript dialog opened (awaiting decision): type=%s message=%q", e.Type, e.Message)
	})()
	time.Sleep(100 * time.Millisecond)
}

func (p *BrowserPage) setPendingDialog(d *JavaScriptDialog) {
	p.dialogMu.Lock()
	defer p.dialogMu.Unlock()
	p.pendingDialog = d
}

// GetPendingDialog returns the dialog blocking page JS, if any.
func (p *BrowserPage) GetPendingDialog() (*JavaScriptDialog, bool) {
	p.dialogMu.Lock()
	defer p.dialogMu.Unlock()
	if p.pendingDialog == nil {
		return nil, false
	}
	d := *p.pendingDialog
	return &d, true
}

// HandleJavaScriptDialog applies AI/user decision: accept=true clicks OK/Yes, false clicks Cancel.
// promptText is used when dialog type is prompt.
func (p *BrowserPage) HandleJavaScriptDialog(accept bool, promptText string) error {
	p.dialogMu.Lock()
	if p.pendingDialog == nil {
		p.dialogMu.Unlock()
		return fmt.Errorf("no pending javascript dialog on this page")
	}
	handled := p.pendingDialog
	d := *handled
	p.dialogMu.Unlock()

	err := proto.PageHandleJavaScriptDialog{
		Accept:     accept,
		PromptText: promptText,
	}.Call(p.page)
	if err != nil {
		return fmt.Errorf("handle javascript dialog (type=%s): %w", d.Type, err)
	}

	p.dialogMu.Lock()
	if p.pendingDialog == handled {
		p.pendingDialog = nil
	}
	p.dialogMu.Unlock()
	log.Infof("javascript dialog handled: type=%s accept=%v", d.Type, accept)
	return nil
}

// DialogBlockingError signals that the operation failed but the page can be
// recovered by handling the pending dialog. It is not evidence of a vulnerability.
type DialogBlockingError struct {
	Dialog JavaScriptDialog
	Err    error
}

func (e *DialogBlockingError) Error() string {
	return fmt.Sprintf("blocked by javascript dialog (type=%s message=%q): call HandleJavaScriptDialog then retry: %v", e.Dialog.Type, e.Dialog.Message, e.Err)
}

func (e *DialogBlockingError) Unwrap() error { return e.Err }

func dialogBlockingError(d *JavaScriptDialog, wrapped error) error {
	if d == nil {
		return wrapped
	}
	return &DialogBlockingError{Dialog: *d, Err: wrapped}
}

func (p *BrowserPage) requireNoDialog(op string) error {
	if d, ok := p.GetPendingDialog(); ok {
		return dialogBlockingError(d, fmt.Errorf("%s blocked", op))
	}
	return nil
}
