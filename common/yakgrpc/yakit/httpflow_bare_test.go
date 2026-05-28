package yakit

import (
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jinzhu/gorm"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
)

func loadTestHTTPFlowBareResponse(t *testing.T, db *gorm.DB, flowID uint) []byte {
	t.Helper()
	raw, err := GetProjectKeyWithError(db, httpFlowBareResponseKey(flowID))
	require.NoError(t, err)
	return []byte(raw)
}

func testHTTPFlowBareResponseMissing(db *gorm.DB, flowID uint) bool {
	_, err := GetProjectKeyWithError(db, httpFlowBareResponseKey(flowID))
	return err != nil
}

func testHTTPFlowGBKWirePacket(t *testing.T) []byte {
	t.Helper()
	gbkBody, err := codec.Utf8ToGB18030([]byte("你好"))
	require.NoError(t, err)
	return []byte(fmt.Sprintf(
		"HTTP/1.1 200 OK\r\nContent-Type: text/html; charset=gbk\r\nContent-Length: %d\r\n\r\n%s",
		len(gbkBody), string(gbkBody),
	))
}

func testHTTPFlowJSONWirePacket(body string) []byte {
	return []byte(fmt.Sprintf(
		"HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nContent-Length: %d\r\n\r\n%s",
		len(body), body,
	))
}

func testHTTPFlowScanRequest(t *testing.T, token string) []byte {
	t.Helper()
	return lowhttp.FixHTTPRequest([]byte(fmt.Sprintf("GET /%s HTTP/1.1\r\nHost: example.com\r\n\r\n", token)))
}

func insertTestHTTPFlow(t *testing.T, db *gorm.DB, flow *schema.HTTPFlow, afterSave ...func(*schema.HTTPFlow)) *schema.HTTPFlow {
	t.Helper()
	if len(afterSave) > 0 {
		flow.AfterSaveHandlers = append(flow.AfterSaveHandlers, afterSave...)
	}
	require.NoError(t, InsertHTTPFlow(db, flow))
	t.Cleanup(func() { db.Where("id = ?", flow.ID).Delete(&schema.HTTPFlow{}) })
	return flow
}

func TestResolveHTTPFlowStoredResponse(t *testing.T) {
	wire := testHTTPFlowGBKWirePacket(t)
	fixed, _, err := lowhttp.FixHTTPResponse(wire)
	require.NoError(t, err)

	t.Run("auto_fix", func(t *testing.T) {
		stored := resolveHTTPFlowStoredResponse(wire, fixed, nil, false)
		require.Contains(t, string(stored), "charset=utf-8")
		require.Contains(t, string(stored), "你好")
	})

	t.Run("no_fix", func(t *testing.T) {
		stored := resolveHTTPFlowStoredResponse(wire, fixed, nil, true)
		require.Equal(t, string(wire), string(stored))
	})
}

func TestHTTPFlowAutoFixedCharset(t *testing.T) {
	t.Run("gbk_body", func(t *testing.T) {
		wire := testHTTPFlowGBKWirePacket(t)
		display, _, _ := lowhttp.FixHTTPResponse(wire)
		require.True(t, httpFlowAutoFixedCharset(wire, display))
	})

	t.Run("header_only_no_tag", func(t *testing.T) {
		token := uuid.NewString()
		wire := []byte("HTTP/1.1 200 OK\r\n\r\n" + token)
		display, _, err := lowhttp.FixHTTPResponse(wire)
		require.NoError(t, err)
		require.False(t, httpFlowAutoFixedCharset(wire, display))
	})

	t.Run("analyze_raw_packet_no_tag", func(t *testing.T) {
		token := uuid.NewString()
		wire := []byte(fmt.Sprintf("HTTP/1.1 200 OK\r\n\r\n%s", token))
		display, _, err := lowhttp.FixHTTPResponse(wire)
		require.NoError(t, err)
		require.False(t, httpFlowShouldStoreBareWire(wire, display, false))
	})

	t.Run("mitm_hijack_prefers_display", func(t *testing.T) {
		wire := []byte("HTTP/1.1 200 OK\r\n\r\noriginal-token")
		display := []byte("HTTP/1.1 200 OK\r\n\r\nreplaced-token")
		stored := resolveHTTPFlowStoredResponse(wire, display, nil, false)
		require.Contains(t, string(stored), "replaced-token")
		require.NotContains(t, string(stored), "original-token")
	})

	t.Run("too_large_placeholder_prefers_display", func(t *testing.T) {
		wire := []byte("HTTP/1.1 200 OK\r\nContent-Length: 100\r\n\r\n" + strings.Repeat("a", 100))
		display := []byte("HTTP/1.1 200 OK\r\nContent-Length: 50\r\n\r\n[[response too large(4MB), truncated]] find more in web fuzzer history!")
		stored := resolveHTTPFlowStoredResponse(wire, display, nil, false)
		require.Contains(t, string(stored), "[[response too large")
	})
}

func TestCreateHTTPFlowBareSidecar(t *testing.T) {
	db := consts.GetGormProjectDatabase()

	t.Run("persists_kv_and_tag_when_wire_differs", func(t *testing.T) {
		token := uuid.NewString()
		wire := testHTTPFlowGBKWirePacket(t)
		req := testHTTPFlowScanRequest(t, token)

		var savedFlow *schema.HTTPFlow
		flow, err := CreateHTTPFlow(
			CreateHTTPFlowWithHTTPS(false),
			CreateHTTPFlowWithRequestRaw(req),
			CreateHTTPFlowWithResponseRaw(wire),
			CreateHTTPFlowWithBareResponseRaw(wire),
			CreateHTTPFlowWithSource("scan"),
			CreateHTTPFlowWithURL(fmt.Sprintf("http://example.com/%s", token)),
			CreateHTTPFlowWithRemoteAddr("127.0.0.1:80"),
			CreateHTTPFlowWithAfterSave(func(f *schema.HTTPFlow) { savedFlow = f }),
		)
		require.NoError(t, err)
		insertTestHTTPFlow(t, db, flow)
		require.NotNil(t, savedFlow)

		require.Contains(t, savedFlow.GetResponse(), "charset=utf-8")
		require.Contains(t, savedFlow.Tags, HTTPFlowTagAutoFixResponse)
		bare := loadTestHTTPFlowBareResponse(t, db, savedFlow.ID)
		require.Contains(t, string(bare), "charset=gbk")
		require.NotContains(t, string(bare), "charset=utf-8")
	})

	t.Run("no_kv_when_wire_equals_display", func(t *testing.T) {
		token := uuid.NewString()
		wire := testHTTPFlowJSONWirePacket(`{"ok":true}`)
		req := testHTTPFlowScanRequest(t, token)

		flow, err := CreateHTTPFlow(
			CreateHTTPFlowWithHTTPS(false),
			CreateHTTPFlowWithRequestRaw(req),
			CreateHTTPFlowWithResponseRaw(wire),
			CreateHTTPFlowWithBareResponseRaw(wire),
			CreateHTTPFlowWithSource("scan"),
			CreateHTTPFlowWithURL(fmt.Sprintf("http://example.com/%s", token)),
			CreateHTTPFlowWithRemoteAddr("127.0.0.1:80"),
		)
		require.NoError(t, err)
		insertTestHTTPFlow(t, db, flow)

		require.True(t, testHTTPFlowBareResponseMissing(db, flow.ID))
		require.NotContains(t, flow.Tags, HTTPFlowTagAutoFixResponse)
	})

	t.Run("fixes_from_wire_when_rsp_hint_is_plain_only", func(t *testing.T) {
		token := uuid.NewString()
		wire := testHTTPFlowGBKWirePacket(t)
		req := testHTTPFlowScanRequest(t, token)
		plain := append([]byte(nil), wire...) // MITM: rsp hint equals wire, not pre-fixed

		flow, err := CreateHTTPFlow(
			CreateHTTPFlowWithHTTPS(false),
			CreateHTTPFlowWithRequestRaw(req),
			CreateHTTPFlowWithResponseRaw(plain),
			CreateHTTPFlowWithBareResponseRaw(wire),
			CreateHTTPFlowWithSource("scan"),
			CreateHTTPFlowWithURL(fmt.Sprintf("http://example.com/%s", token)),
			CreateHTTPFlowWithRemoteAddr("127.0.0.1:80"),
		)
		require.NoError(t, err)
		insertTestHTTPFlow(t, db, flow)

		require.Contains(t, flow.GetResponse(), "charset=utf-8")
		require.Contains(t, flow.GetResponse(), "你好")
	})
}

func TestSaveLowHTTPFlowBareSidecar(t *testing.T) {
	db := consts.GetGormProjectDatabase()

	t.Run("persists_kv_when_bare_differs_from_fixed_packet", func(t *testing.T) {
		token := uuid.NewString()
		wire := testHTTPFlowGBKWirePacket(t)
		fixed, _, err := lowhttp.FixHTTPResponse(wire)
		require.NoError(t, err)
		require.NotEqual(t, string(wire), string(fixed))

		var savedFlow *schema.HTTPFlow
		SaveLowHTTPFlow(&lowhttp.LowhttpResponse{
			Https:        false,
			RawRequest:   testHTTPFlowScanRequest(t, token),
			RawPacket:    fixed,
			BareResponse: wire,
			Url:          fmt.Sprintf("http://example.com/%s", token),
			RemoteAddr:   "127.0.0.1:80",
			Source:       "scan",
			RuntimeId:    "runtime-" + token,
			TraceInfo:    &lowhttp.LowhttpTraceInfo{},
			AfterSaveHTTPFlowHandler: []func(*schema.HTTPFlow){
				func(f *schema.HTTPFlow) { savedFlow = f },
			},
		}, true)
		require.NotNil(t, savedFlow)
		t.Cleanup(func() { db.Where("id = ?", savedFlow.ID).Delete(&schema.HTTPFlow{}) })

		require.Contains(t, savedFlow.GetResponse(), "charset=utf-8")
		require.Contains(t, savedFlow.Tags, HTTPFlowTagAutoFixResponse)
		require.True(t, strings.Contains(string(loadTestHTTPFlowBareResponse(t, db, savedFlow.ID)), "charset=gbk"))
	})

	t.Run("no_kv_when_no_fix", func(t *testing.T) {
		token := uuid.NewString()
		wire := testHTTPFlowJSONWirePacket(`{"ok":true}`)

		var savedFlow *schema.HTTPFlow
		SaveLowHTTPFlow(&lowhttp.LowhttpResponse{
			Https:        false,
			RawRequest:   testHTTPFlowScanRequest(t, token),
			RawPacket:    wire,
			BareResponse: wire,
			Url:          fmt.Sprintf("http://example.com/%s", token),
			RemoteAddr:   "127.0.0.1:80",
			Source:       "scan",
			TraceInfo:    &lowhttp.LowhttpTraceInfo{},
			AfterSaveHTTPFlowHandler: []func(*schema.HTTPFlow){
				func(f *schema.HTTPFlow) { savedFlow = f },
			},
		}, true)
		require.NotNil(t, savedFlow)
		require.True(t, savedFlow.NoFixContentLength)
		t.Cleanup(func() { db.Where("id = ?", savedFlow.ID).Delete(&schema.HTTPFlow{}) })

		require.True(t, testHTTPFlowBareResponseMissing(db, savedFlow.ID))
	})
}
