package pcaputil

import (
	"bytes"
	"encoding/binary"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func dohBudgetSession(t *testing.T) *captureSession {
	t.Helper()
	budget := DefaultParserBudget()
	budget.MaxMessageBytes, budget.MaxFrameBytes, budget.MaxBufferedBytes = 32768, 16384, 1<<20
	session, err := NewProtocolSession(budget)
	require.NoError(t, err)
	s := session.(*captureSession)
	for _, step := range dohExchangeH2Start() {
		require.Nil(t, s.Feed(step.dir, time.Unix(1, 0), step.wire).Err)
	}
	t.Cleanup(func() { s.Close("FIN"); require.Zero(t, s.f.a.buffered.Load()) })
	return s
}

func dohBudgetPOST(t *testing.T, s *captureSession, id uint32) {
	t.Helper()
	headers := h2TestHeaders(t, ":method", "POST", ":scheme", "https", ":path", "/dns-query", ":authority", "dns.example.test", "content-type", "application/dns-message")
	require.Nil(t, s.Feed(0, time.Unix(1, 0), h2TestFrame(1, 4, id, headers)).Err)
}

func assertDoHBudgetAccounting(t *testing.T, f *binFlow) {
	t.Helper()
	d := f.doh
	require.NotNil(t, d)
	var streamBytes int64
	if f.h2 != nil {
		for _, stream := range f.h2.streams {
			streamBytes += stream.dohRetained
		}
	}
	require.Equal(t, streamBytes, d.streamBytes)
	var names int64
	for _, name := range d.pending {
		names += int64(len(name))
	}
	pending := int64(0)
	if d.pending != nil {
		pending = 128 + int64(d.pendingSlots)*64 + names
	}
	require.Equal(t, pending, d.pendingBytes)
	require.Equal(t, d.retainedBytes(), d.reserved)
	buffers := int64(cap(f.directions[0].buffer) + cap(f.directions[1].buffer))
	require.Equal(t, f.sessionBytes+d.reserved+buffers, f.a.buffered.Load())
	require.LessOrEqual(t, f.a.buffered.Load(), int64(f.a.config.MaxBufferedBytes))
}

func TestDoHHTTP2AggregateBodyBudgetRejectsBeforeAllocation(t *testing.T) {
	s := dohBudgetSession(t)
	f := s.f
	for _, id := range []uint32{1, 3, 5} {
		dohBudgetPOST(t, s, id)
	}
	require.NoError(t, f.reserveSession(f.h2.sessionStorageBytes()))
	baseline := f.a.buffered.Load()
	const payloadBytes = 8192
	f.a.config.MaxBufferedBytes = int(baseline) + 2*payloadBytes + payloadBytes/2
	payload := bytes.Repeat([]byte{0}, payloadBytes)
	for _, id := range []uint32{1, 3} {
		require.Nil(t, s.Feed(0, time.Unix(1, 0), h2TestFrame(0, 0, id, payload)).Err)
		require.Equal(t, payloadBytes, cap(f.h2.streams[id].dohBuf[0]))
		assertDoHBudgetAccounting(t, f)
	}
	require.Equal(t, baseline+2*payloadBytes, f.a.buffered.Load())
	limited := s.Feed(0, time.Unix(1, 0), h2TestFrame(0, 0, 5, payload))
	require.NotNil(t, limited.Err)
	require.Equal(t, ErrResourceExceeded, limited.Err.Kind)
	require.Contains(t, limited.Err.Message, "DoH retained state")
	require.Equal(t, "limited", limited.Events[0].Status)
	require.Zero(t, cap(f.h2.streams[5].dohBuf[0]), "rejected stream must not allocate its 8 KiB body")
	require.True(t, f.h2.streams[5].dohFailed)
	require.LessOrEqual(t, f.a.peak.Load(), int64(f.a.config.MaxBufferedBytes))
	assertDoHBudgetAccounting(t, f)
	t.Logf("per-frame=%d per-message-limit=%d baseline=%d global-limit=%d retained=%d peak=%d rejected-capacity=%d", payloadBytes, f.a.config.MaxMessageBytes, baseline, f.a.config.MaxBufferedBytes, f.a.buffered.Load(), f.a.peak.Load(), cap(f.h2.streams[5].dohBuf[0]))
	for _, id := range []uint32{1, 3} {
		require.Nil(t, s.Feed(1, time.Unix(1, 0), h2TestFrame(3, 0, id, []byte{0, 0, 0, 0})).Err)
		assertDoHBudgetAccounting(t, f)
	}
	require.Equal(t, int64(256), f.doh.reserved)
	s.Close("FIN")
	require.Zero(t, f.a.buffered.Load())
}

func TestDoHHTTP2GrowthChargesOldAndNewBodyArrays(t *testing.T) {
	s := dohBudgetSession(t)
	f := s.f
	dohBudgetPOST(t, s, 1)
	require.NoError(t, f.reserveSession(f.h2.sessionStorageBytes()))
	baseline := f.a.buffered.Load()
	payload := make([]byte, 8192)
	require.Nil(t, s.Feed(0, time.Unix(1, 0), h2TestFrame(0, 0, 1, payload)).Err)
	f.a.config.MaxBufferedBytes = int(baseline) + 16384 + 128
	require.Less(t, baseline+16384, int64(f.a.config.MaxBufferedBytes), "the final 16 KiB body would fit")
	require.Greater(t, baseline+8192+16384, int64(f.a.config.MaxBufferedBytes), "the live old+new arrays would not fit")
	r := s.Feed(0, time.Unix(1, 0), h2TestFrame(0, 0, 1, payload))
	require.NotNil(t, r.Err)
	require.Equal(t, ErrResourceExceeded, r.Err.Kind)
	require.Zero(t, cap(f.h2.streams[1].dohBuf[0]))
	require.Equal(t, int64(256), f.doh.reserved)
	require.LessOrEqual(t, f.a.peak.Load(), int64(f.a.config.MaxBufferedBytes))
	assertDoHBudgetAccounting(t, f)
	t.Logf("growth 8192->16384 rejected before allocation: baseline=%d limit=%d peak=%d", baseline, f.a.config.MaxBufferedBytes, f.a.peak.Load())
}

func TestDoHHTTP2BudgetReleasesResetGoAwayAndEOF(t *testing.T) {
	s := dohBudgetSession(t)
	f := s.f
	for _, id := range []uint32{1, 3} {
		dohBudgetPOST(t, s, id)
		require.Nil(t, s.Feed(0, time.Unix(1, 0), h2TestFrame(0, 0, id, make([]byte, 1024))).Err)
	}
	get := dohExchangeH2GET(t, 5, "abandoned.example")
	require.Nil(t, s.Feed(get.dir, time.Unix(1, 0), get.wire).Err)
	require.Len(t, f.doh.pending, 1)
	assertDoHBudgetAccounting(t, f)
	require.Nil(t, s.Feed(1, time.Unix(1, 0), h2TestFrame(3, 0, 1, []byte{0, 0, 0, 0})).Err)
	assertDoHBudgetAccounting(t, f)
	goaway := make([]byte, 8)
	binary.BigEndian.PutUint32(goaway, 3)
	require.Nil(t, s.Feed(1, time.Unix(1, 0), h2TestFrame(7, 0, 0, goaway)).Err)
	require.Nil(t, f.doh.pending)
	require.Zero(t, f.doh.pendingSlots)
	require.Zero(t, f.h2.streams[5].dohRetained)
	require.Equal(t, 1024, cap(f.h2.streams[3].dohBuf[0]), "GOAWAY preserves streams at or below the last accepted ID")
	assertDoHBudgetAccounting(t, f)
	d := f.doh
	s.Close("EOF")
	require.Zero(t, d.reserved)
	require.Zero(t, f.a.buffered.Load())
	f.releaseDoH()
	f.closeSession()
	require.Zero(t, f.a.buffered.Load(), "repeated close must not double-release")
}

func TestDoHHTTP1PendingMapRetainsSlotChargeUntilEmpty(t *testing.T) {
	session, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	s := session.(*captureSession)
	defer s.Close("FIN")
	for _, name := range []string{"one.example", "two.example"} {
		require.Nil(t, s.Feed(0, time.Unix(1, 0), dohGET("dns.example.test", name, 0)).Err)
	}
	d := s.f.doh
	require.Equal(t, 2, d.pendingSlots)
	require.Equal(t, int64(128+2*64+len("one.example")+len("two.example")), d.pendingBytes)
	for i, name := range []string{"one.example", "two.example"} {
		require.Nil(t, s.Feed(1, time.Unix(1, 0), dohHTTPResp(200, dnsWire(dnsAResponse(0, name, [4]byte{1, 2, 3, 4})))).Err)
		if i == 0 {
			require.Equal(t, 2, d.pendingSlots)
			require.Equal(t, int64(128+2*64+len("two.example")), d.pendingBytes)
		}
	}
	require.Nil(t, d.pending)
	require.Zero(t, d.pendingBytes)
	require.Equal(t, int64(256), d.reserved)
	s.Close("FIN")
	require.Zero(t, s.f.a.buffered.Load())
}

func TestDoHHTTP2BaselineAndMetadataRequireReservation(t *testing.T) {
	t.Run("fixed baseline", func(t *testing.T) {
		s := dohBudgetSession(t)
		baseline := s.f.a.buffered.Load()
		s.f.a.config.MaxBufferedBytes = int(baseline) + 255
		headers := h2TestHeaders(t, ":method", "POST", ":scheme", "https", ":path", "/dns-query", ":authority", "dns.example.test", "content-type", "application/dns-message")
		r := s.Feed(0, time.Unix(1, 0), h2TestFrame(1, 4, 1, headers))
		require.NotNil(t, r.Err)
		require.Equal(t, ErrResourceExceeded, r.Err.Kind)
		require.Nil(t, s.f.doh, "failed admission must not retain uncharged DoH state")
		require.Equal(t, baseline, s.f.a.buffered.Load())
	})
	t.Run("media string", func(t *testing.T) {
		s := dohBudgetSession(t)
		get := dohExchangeH2GET(t, 1, "one.example")
		require.Nil(t, s.Feed(get.dir, time.Unix(1, 0), get.wire).Err)
		require.NoError(t, s.f.reserveSession(s.f.h2.sessionStorageBytes()))
		s.f.a.config.MaxBufferedBytes = int(s.f.a.buffered.Load()) + 254
		r := s.Feed(1, time.Unix(1, 0), h2TestFrame(1, 4, 1, h2TestHeaders(t, ":status", "200", "content-type", "x/"+strings.Repeat("a", 253))))
		require.NotNil(t, r.Err)
		require.Equal(t, ErrResourceExceeded, r.Err.Kind)
		require.Empty(t, s.f.h2.streams[1].dohContentType)
		require.Empty(t, s.f.doh.pending)
		assertDoHBudgetAccounting(t, s.f)
	})
}

func TestDoHHTTP2BudgetReleasesGenericBodyFailure(t *testing.T) {
	s := dohBudgetSession(t)
	headers := h2TestHeaders(t, ":method", "POST", ":scheme", "https", ":path", "/dns-query", ":authority", "dns.example.test", "content-type", "application/dns-message", "content-length", "4")
	require.Nil(t, s.Feed(0, time.Unix(1, 0), h2TestFrame(1, 4, 1, headers)).Err)
	require.Nil(t, s.Feed(0, time.Unix(1, 0), h2TestFrame(0, 0, 1, []byte{0, 0})).Err)
	require.Equal(t, 2, cap(s.f.h2.streams[1].dohBuf[0]))
	r := s.Feed(0, time.Unix(1, 0), h2TestFrame(0, 1, 1, []byte{0, 0, 0}))
	require.NotNil(t, r.Err)
	require.Equal(t, "stream", r.Events[0].Session["Error Scope"])
	require.Zero(t, s.f.h2.streams[1].dohRetained)
	require.Equal(t, int64(256), s.f.doh.reserved)
	assertDoHBudgetAccounting(t, s.f)
}
