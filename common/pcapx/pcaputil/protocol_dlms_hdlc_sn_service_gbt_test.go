package pcaputil

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
)

func hdlcSNServiceGBTCases(t *testing.T, name, pin string, count int) []hdlcOpCase {
	t.Helper()
	raw := finsReadBatch(t, "dlms-hdlc-sn-service-gbt-source", "DLMS_HDLC_SN_SERVICE_GBT_ROOT", name)
	require.Equal(t, pin, fmt.Sprintf("%x", sha256.Sum256(raw)))
	var d struct{ Cases []hdlcOpCase }
	require.NoError(t, json.Unmarshal(raw, &d))
	require.Len(t, d.Cases, count)
	return d.Cases
}
func hdlcSNServiceGBTTypes(t *testing.T, f map[string]any) {
	t.Helper()
	hdlcSNGBTTypes(t, f)
	snWriteTypes(t, f)
	if raw, ok := f["GBT"]; ok {
		q := raw.(map[string]any)
		if inner, ok := q["assembled_sn_apdu"]; ok {
			snWriteTypes(t, inner.(map[string]any))
		}
	}
}

// Kind is independently fixed by gold. Mutations of each returned error must
// leave the retained event's next query error kind and complete message intact.
func hdlcSNServiceGBTErrorOwnership(t *testing.T, e *ProtocolEvent, kind string) {
	t.Helper()
	fields, err := e.GetFields()
	require.Nil(t, fields)
	rocTypedError(t, kind, err)
	var pe *ProtocolError
	require.ErrorAs(t, err, &pe)
	message := pe.Message
	require.NotEmpty(t, message)
	if kind == "ContextRequired" {
		require.Equal(t, "DLMS HDLC SN GBT cannot consume an observed LN owner or information group", message)
	}
	for i := 0; i < 2; i++ {
		pe.Kind, pe.Message = ErrNeedMore, "caller mutation"
		decoded, next := e.Decode()
		require.Nil(t, decoded)
		rocTypedError(t, kind, next)
		require.ErrorAs(t, next, &pe)
		require.Equal(t, message, pe.Message)
		pe.Kind, pe.Message = ErrNeedMore, "caller mutation"
		fields, next = e.GetFields()
		require.Nil(t, fields)
		rocTypedError(t, kind, next)
		require.ErrorAs(t, next, &pe)
		require.Equal(t, message, pe.Message)
	}
}
func TestDLMSHDLCSNServiceGBTNative(t *testing.T) {
	hdlcSNGBTPublicAPINativeWithChecks(t, hdlcSNServiceGBTCases(t, "controls.json", "3f6854c5e2b4bd440b6a493dcdfdba7b99ec8c0871fdca4ea248946518a7a9c6", 14), hdlcSNServiceGBTTypes, hdlcSNServiceGBTErrorOwnership, hdlcSNServiceGBTCloseCheck)
}
func TestDLMSHDLCSNServiceGBTEchoNative(t *testing.T) {
	hdlcSNGBTPublicAPINativeWithChecks(t, hdlcSNServiceGBTCases(t, "echo-supplement.json", "442356d6946531cbf570742323b1b1f611d083cee0945618bf540233e67f4987", 3), hdlcSNServiceGBTTypes, hdlcSNServiceGBTErrorOwnership, hdlcSNServiceGBTCloseCheck)
}
func hdlcSNServiceGBTCloseCheck(t *testing.T, c hdlcOpCase, events []*ProtocolEvent, tr, reason string) {
	t.Helper()
	require.Len(t, events, len(c.Close.Events))
	for i, e := range events {
		w := c.Close.Events[i]
		require.Equal(t, "dlms", e.Protocol)
		require.Equal(t, tr, e.Transport)
		require.Equal(t, CaptureDomain{}, e.Domain)
		require.Equal(t, w.Profile, e.Profile)
		require.Equal(t, w.Status, e.Status)
		require.Equal(t, w.Completeness, e.Completeness)
		require.Equal(t, w.Direction, e.Direction)
		require.NotZero(t, e.ID)
		require.NotZero(t, e.FlowID)
		require.Zero(t, e.TransactionID)
		require.Zero(t, e.ResponseTo)
		require.Nil(t, e.Raw)
		require.Zero(t, e.Length)
		summary, at, offset := w.SummaryUDP, w.TimeUDP, w.OffsetUDP
		if tr == "tcp" {
			// The independent native gold fixes the caller reason to test-close.
			// CaptureAnalyzer.Close supplies ctx-canceled; only that caller
			// prefix varies, never the protocol diagnostic or its state.
			require.True(t, strings.HasPrefix(w.SummaryTCP, "test-close: "))
			summary, at, offset = reason+strings.TrimPrefix(w.SummaryTCP, "test-close"), w.TimeTCP, w.OffsetTCP
		}
		require.Equal(t, summary, e.Summary)
		require.Equal(t, (&ProtocolError{Kind: ErrNeedMore, Message: summary}).Error(), e.Error)
		require.Equal(t, at, e.Timestamp.Unix())
		require.Equal(t, offset, e.Offset)
		hdlcOpEqual(t, w.Session, e.Session)
		require.Len(t, e.Session, 6)
		counter, length := "Outstanding", "assembled_data_length"
		if _, ok := w.Session["Buffered SN Groups"]; ok {
			counter = "Buffered SN Groups"
		}
		if _, ok := w.Session["assembled_body_length"]; ok {
			length = "assembled_body_length"
		}
		for _, k := range []string{counter, "observed_block_count", length} {
			require.IsType(t, int(0), e.Session[k])
		}
		for _, k := range []string{"source_address", "destination_address"} {
			require.IsType(t, uint32(0), e.Session[k])
		}
		require.IsType(t, uint16(0), e.Session["block_number"])
		ends := []string{"192.0.2.1:41000", "192.0.2.2:4059"}
		require.Equal(t, ends[w.Direction], e.Source)
		require.Equal(t, ends[1-w.Direction], e.Destination)
		for n := 0; n < 2; n++ {
			fields, err := e.GetFields()
			require.Nil(t, fields)
			rocTypedError(t, "NeedMore", err)
			var pe *ProtocolError
			require.ErrorAs(t, err, &pe)
			require.Equal(t, summary, pe.Message)
			pe.Kind, pe.Message = ErrMalformedMessage, "caller mutation"
			decoded, err := e.Decode()
			require.Nil(t, decoded)
			rocTypedError(t, "NeedMore", err)
			require.ErrorAs(t, err, &pe)
			require.Equal(t, summary, pe.Message)
			pe.Kind, pe.Message = ErrMalformedMessage, "caller mutation"
			actionPoison(e)
		}
	}
}
