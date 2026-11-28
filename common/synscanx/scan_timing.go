package synscanx

import (
	"context"
	"errors"
	"os"
	"sync/atomic"
	"time"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/netstackvm"
	"github.com/yaklang/yaklang/common/synscan"
	"github.com/yaklang/yaklang/common/utils/diagnostics"
)

type synScanTiming struct {
	lastProbeEnd      diagnostics.LatestTimestamp
	tcpProbes         atomic.Int64
	tcpProbeTime      atomic.Int64
	tcpAdmissionSend  atomic.Int64
	tcpAfterFirstSend atomic.Int64
	tcpOpen           atomic.Int64
	tcpRefused        atomic.Int64
	tcpNoResponse     atomic.Int64
	tcpDeadline       atomic.Int64
	tcpCanceled       atomic.Int64
	tcpFailed         atomic.Int64
	resultSends       atomic.Int64
	resultSendTime    atomic.Int64
	resultSendCancel  atomic.Int64
}

func newSynScanTiming() *synScanTiming {
	if !diagnostics.Enabled(diagnostics.LevelNormal) {
		return nil
	}
	return &synScanTiming{}
}

func (t *synScanTiming) recordTCPProbe(started time.Time, stages netstackvm.TCPProbeTiming, err error) {
	ended := time.Now()
	t.lastProbeEnd.Record(ended)
	t.tcpProbes.Add(1)
	t.tcpProbeTime.Add(ended.Sub(started).Nanoseconds())
	t.tcpAdmissionSend.Add(stages.AdmissionAndSend.Nanoseconds())
	t.tcpAfterFirstSend.Add(stages.AfterFirstSend.Nanoseconds())
	switch {
	case err == nil:
		t.tcpOpen.Add(1)
	case errors.Is(err, netstackvm.ErrProbeRefused):
		t.tcpRefused.Add(1)
	case errors.Is(err, netstackvm.ErrProbeNoResponse):
		t.tcpNoResponse.Add(1)
	case errors.Is(err, context.DeadlineExceeded):
		t.tcpDeadline.Add(1)
	case errors.Is(err, context.Canceled):
		t.tcpCanceled.Add(1)
	default:
		t.tcpFailed.Add(1)
	}
}

func (t *synScanTiming) sendResult(ctx context.Context, out chan<- *synscan.SynScanResult, result *synscan.SynScanResult) {
	var started time.Time
	if t != nil {
		started = time.Now()
		t.resultSends.Add(1)
	}
	select {
	case <-ctx.Done():
		if t != nil {
			t.resultSendCancel.Add(1)
		}
	case out <- result:
	}
	if t != nil {
		t.resultSendTime.Add(time.Since(started).Nanoseconds())
	}
}

func (t *synScanTiming) logSummary(runtimeID string, elapsed time.Duration) {
	if t == nil {
		return
	}
	if runtimeID == "" {
		runtimeID = os.Getenv(consts.YAK_RUNTIME_ID)
	}
	// Probe time includes the session's pacing and reply wait. Result send
	// time covers the channel send, including any wait for a consumer.
	log.Infof("synscanx timing: runtime_id=%q elapsed=%s last_probe_finished_at_ns=%d syn_probes=%d syn_probe_time_sum=%s syn_no_response=%d syn_deadline=%d result_sends=%d result_send_time_sum=%s result_send_canceled=%d"+
		" syn_admission_send_sum=%s syn_after_first_send_sum=%s syn_open=%d syn_refused=%d syn_canceled=%d syn_failed=%d",
		runtimeID, elapsed, t.lastProbeEnd.UnixNano(), t.tcpProbes.Load(), time.Duration(t.tcpProbeTime.Load()), t.tcpNoResponse.Load(),
		t.tcpDeadline.Load(), t.resultSends.Load(), time.Duration(t.resultSendTime.Load()), t.resultSendCancel.Load(),
		time.Duration(t.tcpAdmissionSend.Load()), time.Duration(t.tcpAfterFirstSend.Load()), t.tcpOpen.Load(), t.tcpRefused.Load(), t.tcpCanceled.Load(), t.tcpFailed.Load())
}
