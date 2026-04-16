//go:build hids && linux

package auditd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"syscall"
	"time"

	libaudit "github.com/elastic/go-libaudit/v2"
	"github.com/elastic/go-libaudit/v2/auparse"

	hidscollector "github.com/yaklang/yaklang/common/hids/collector"
	"github.com/yaklang/yaklang/common/hids/enrich"
	"github.com/yaklang/yaklang/common/hids/model"
)

type Collector struct {
	systemd enrich.SystemdContext

	mu          sync.RWMutex
	client      *libaudit.AuditClient
	control     *libaudit.AuditClient
	reassembler *libaudit.Reassembler
	sink        chan<- model.Event
	rules       [][]byte
	state       auditCollectorState
	fileState   *sensitiveFileStateCache
}

func New() hidscollector.Instance {
	return &Collector{
		systemd:   enrich.DetectSystemdContext(),
		state:     newAuditCollectorState(),
		fileState: newSensitiveFileStateCache(),
	}
}

func (c *Collector) Name() string {
	return "auditd"
}

func (c *Collector) Start(ctx context.Context, sink chan<- model.Event) error {
	if c.fileState != nil {
		c.fileState.Seed()
	}
	control, err := libaudit.NewAuditClient(nil)
	if err != nil {
		return wrapAuditControlStartupError(err)
	}
	ruleInstall, err := installHIDSAuditRules(control)
	if err != nil {
		_ = control.Close()
		return err
	}

	client, err := libaudit.NewMulticastAuditClient(nil)
	if err != nil {
		_ = deleteManagedAuditRules(control, ruleInstall.rules)
		_ = control.Close()
		return wrapAuditStartupError(err)
	}

	reassembler, err := libaudit.NewReassembler(128, 3*time.Second, c)
	if err != nil {
		_ = client.Close()
		_ = deleteManagedAuditRules(control, ruleInstall.rules)
		_ = control.Close()
		return fmt.Errorf("create audit reassembler: %w", err)
	}

	c.mu.Lock()
	c.client = client
	c.control = control
	c.reassembler = reassembler
	c.sink = sink
	c.rules = cloneRuleWires(ruleInstall.rules)
	c.mu.Unlock()
	c.state.setRuleInstallResult(ruleInstall)
	c.state.setStatus("running", auditRunningMessage(ruleInstall))

	go c.maintainLoop(ctx)
	go c.receiveLoop(ctx)
	return nil
}

func (c *Collector) Close() error {
	c.mu.Lock()
	client := c.client
	control := c.control
	reassembler := c.reassembler
	rules := cloneRuleWires(c.rules)
	c.client = nil
	c.control = nil
	c.reassembler = nil
	c.sink = nil
	c.rules = nil
	c.mu.Unlock()
	c.state.setStatus("stopped", "audit collector is stopped")

	var errs []error
	if reassembler != nil {
		_ = reassembler.Close()
	}
	if control != nil {
		errs = append(errs, deleteManagedAuditRules(control, rules))
		errs = append(errs, control.Close())
	}
	if client != nil {
		errs = append(errs, client.Close())
	}
	return errors.Join(errs...)
}

func (c *Collector) ReassemblyComplete(msgs []*auparse.AuditMessage) {
	if len(msgs) == 0 {
		return
	}

	c.state.observeReceived()
	event, outcome := buildAuditObservation(msgs, c.systemd.JournalAvailable)
	if outcome.keep && event.Type == model.EventTypeAudit && c.fileState != nil {
		event = c.fileState.Enrich(event)
		outcome.event = event
	}
	c.state.observeOutcome(outcome, event.Timestamp)
	if !outcome.keep {
		return
	}
	c.publish(event)
}

func (c *Collector) EventsLost(count int) {
	if count <= 0 {
		return
	}
	event := buildAuditLossObservation(count, c.systemd.JournalAvailable)
	c.state.observeLoss(event.Timestamp, "events-lost")
	c.publish(event)
}

func (c *Collector) receiveLoop(ctx context.Context) {
	defer c.Close()

	for {
		if ctx.Err() != nil {
			return
		}

		c.mu.RLock()
		client := c.client
		reassembler := c.reassembler
		c.mu.RUnlock()
		if client == nil || reassembler == nil {
			return
		}

		rawMsg, err := client.Receive(true)
		if err != nil {
			if ctx.Err() != nil || errors.Is(err, io.EOF) {
				return
			}
			if shouldStopReceiveLoop(err) {
				c.state.setStatus("degraded", fmt.Sprintf("audit collector receive loop stopped: %v", err))
				event := buildAuditRuntimeLossObservation("receive-error", err, c.systemd.JournalAvailable)
				c.state.observeLoss(event.Timestamp, "runtime.receive-error")
				c.publish(event)
				return
			}
			time.Sleep(100 * time.Millisecond)
			continue
		}
		if rawMsg == nil {
			continue
		}

		data := append([]byte(nil), rawMsg.Data...)
		if err := reassembler.Push(rawMsg.Type, data); err != nil {
			time.Sleep(50 * time.Millisecond)
		}
	}
}

func wrapAuditStartupError(err error) error {
	if err == nil {
		return nil
	}

	lowercase := strings.ToLower(err.Error())
	switch {
	case errors.Is(err, syscall.EPERM),
		errors.Is(err, syscall.EACCES),
		strings.Contains(lowercase, "operation not permitted"),
		strings.Contains(lowercase, "permission denied"):
		return fmt.Errorf(
			"open audit multicast socket: %w; HIDS audit collector needs root or CAP_AUDIT_READ, and some containers / desktop kernels block NETLINK_AUDIT entirely",
			err,
		)
	case strings.Contains(lowercase, "audit not supported by kernel"):
		return fmt.Errorf("audit subsystem is not supported by this kernel: %w", err)
	default:
		return fmt.Errorf("open audit multicast socket: %w", err)
	}
}

func wrapAuditControlStartupError(err error) error {
	if err == nil {
		return nil
	}

	if isAuditPermissionError(err) {
		return fmt.Errorf(
			"open audit control socket: %w; HIDS audit collector needs root or CAP_AUDIT_CONTROL to provision command and sensitive-file audit rules",
			err,
		)
	}
	return fmt.Errorf("open audit control socket: %w", err)
}

func shouldStopReceiveLoop(err error) bool {
	if err == nil {
		return false
	}

	lowercase := strings.ToLower(err.Error())
	return errors.Is(err, syscall.EPERM) ||
		errors.Is(err, syscall.EACCES) ||
		errors.Is(err, syscall.EBADF) ||
		strings.Contains(lowercase, "operation not permitted") ||
		strings.Contains(lowercase, "permission denied") ||
		strings.Contains(lowercase, "use of closed network connection") ||
		strings.Contains(lowercase, "bad file descriptor")
}

func (c *Collector) maintainLoop(ctx context.Context) {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.mu.RLock()
			reassembler := c.reassembler
			c.mu.RUnlock()
			if reassembler != nil {
				_ = reassembler.Maintain()
			}
		}
	}
}

func (c *Collector) publish(event model.Event) {
	c.mu.RLock()
	sink := c.sink
	c.mu.RUnlock()
	if sink == nil {
		return
	}

	select {
	case sink <- event:
	default:
	}
}

func (c *Collector) HealthSnapshot() hidscollector.HealthSnapshot {
	return c.state.snapshot(c.systemd.JournalAvailable)
}

func auditRunningMessage(result auditRuleInstallResult) string {
	switch {
	case result.total == 0:
		return "audit collector is running"
	case result.added > 0 && (result.existing > 0 || result.skipped > 0):
		return fmt.Sprintf(
			"audit collector is running; managed audit rules added=%d existing=%d skipped=%d",
			result.added,
			result.existing,
			result.skipped,
		)
	case result.added > 0:
		return fmt.Sprintf("audit collector is running; managed audit rules added=%d", result.added)
	case result.existing > 0 || result.skipped > 0:
		return fmt.Sprintf(
			"audit collector is running; managed audit rules already present=%d skipped=%d",
			result.existing,
			result.skipped,
		)
	default:
		return "audit collector is running"
	}
}

func cloneRuleWires(rules [][]byte) [][]byte {
	if len(rules) == 0 {
		return nil
	}
	cloned := make([][]byte, 0, len(rules))
	for _, rule := range rules {
		cloned = append(cloned, cloneRuleWire(rule))
	}
	return cloned
}
