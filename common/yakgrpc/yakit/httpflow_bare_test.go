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

func TestCreateHTTPFlow_PersistsBareWhenFixDiffers(t *testing.T) {
	db := consts.GetGormProjectDatabase()
	token := uuid.NewString()

	gbkBody, err := codec.Utf8ToGB18030([]byte("你好"))
	require.NoError(t, err)
	wire := []byte(fmt.Sprintf("HTTP/1.1 200 OK\r\nContent-Type: text/html; charset=gbk\r\nContent-Length: %d\r\n\r\n%s", len(gbkBody), string(gbkBody)))
	req := lowhttp.FixHTTPRequest([]byte(fmt.Sprintf("GET /%s HTTP/1.1\r\nHost: example.com\r\n\r\n", token)))

	var savedFlow *schema.HTTPFlow
	flow, err := CreateHTTPFlowFromHTTPWithBodySavedFromRaw(
		false,
		req,
		wire,
		"scan",
		fmt.Sprintf("http://example.com/%s", token),
		"127.0.0.1:80",
		CreateHTTPFlowWithBareResponseRaw(wire),
		CreateHTTPFlowWithAfterSave(func(f *schema.HTTPFlow) {
			savedFlow = f
		}),
	)
	require.NoError(t, err)
	require.NoError(t, InsertHTTPFlow(db, flow))
	require.NotNil(t, savedFlow)
	require.Greater(t, int(savedFlow.ID), 0)
	defer db.Where("id = ?", savedFlow.ID).Delete(&schema.HTTPFlow{})

	stored := savedFlow.GetResponse()
	require.Contains(t, stored, "charset=utf-8")
	require.Contains(t, stored, "你好")
	require.Contains(t, savedFlow.Tags, HTTPFlowTagAutoFixResponse)

	bare := loadTestHTTPFlowBareResponse(t, db, savedFlow.ID)
	require.Contains(t, string(bare), "charset=gbk")
	require.NotContains(t, string(bare), "charset=utf-8")
}

func TestCreateHTTPFlow_SkipsBareKVWhenSame(t *testing.T) {
	db := consts.GetGormProjectDatabase()
	token := uuid.NewString()

	body := `{"ok":true}`
	wire := []byte(fmt.Sprintf("HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nContent-Length: %d\r\n\r\n%s", len(body), body))
	req := lowhttp.FixHTTPRequest([]byte(fmt.Sprintf("GET /%s HTTP/1.1\r\nHost: example.com\r\n\r\n", token)))

	flow, err := CreateHTTPFlowFromHTTPWithBodySavedFromRaw(
		false,
		req,
		wire,
		"scan",
		fmt.Sprintf("http://example.com/%s", token),
		"127.0.0.1:80",
		CreateHTTPFlowWithBareResponseRaw(wire),
	)
	require.NoError(t, err)
	require.NoError(t, InsertHTTPFlow(db, flow))
	defer db.Where("id = ?", flow.ID).Delete(&schema.HTTPFlow{})

	require.True(t, testHTTPFlowBareResponseMissing(db, flow.ID))
	require.NotContains(t, flow.Tags, HTTPFlowTagAutoFixResponse)
}

func TestSaveLowHTTPFlow_PersistsBareWhenLowhttpFixed(t *testing.T) {
	db := consts.GetGormProjectDatabase()
	token := uuid.NewString()

	gbkBody, err := codec.Utf8ToGB18030([]byte("你好"))
	require.NoError(t, err)
	wire := []byte(fmt.Sprintf("HTTP/1.1 200 OK\r\nContent-Type: text/html; charset=gbk\r\nContent-Length: %d\r\n\r\n%s", len(gbkBody), string(gbkBody)))
	fixed, _, err := lowhttp.FixHTTPResponse(wire)
	require.NoError(t, err)
	require.NotEqual(t, string(wire), string(fixed))

	req := lowhttp.FixHTTPRequest([]byte(fmt.Sprintf("GET /%s HTTP/1.1\r\nHost: example.com\r\n\r\n", token)))
	var savedFlow *schema.HTTPFlow
	SaveLowHTTPFlow(&lowhttp.LowhttpResponse{
		Https:        false,
		RawRequest:   req,
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
	defer db.Where("id = ?", savedFlow.ID).Delete(&schema.HTTPFlow{})

	require.Contains(t, savedFlow.GetResponse(), "charset=utf-8")
	require.Contains(t, savedFlow.Tags, HTTPFlowTagAutoFixResponse)
	bare := loadTestHTTPFlowBareResponse(t, db, savedFlow.ID)
	require.True(t, strings.Contains(string(bare), "charset=gbk"))
}

func TestSaveLowHTTPFlow_NoBareKVWhenNoFixAndSameWire(t *testing.T) {
	db := consts.GetGormProjectDatabase()
	token := uuid.NewString()

	body := `{"ok":true}`
	wire := []byte(fmt.Sprintf("HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nContent-Length: %d\r\n\r\n%s", len(body), body))
	req := lowhttp.FixHTTPRequest([]byte(fmt.Sprintf("GET /%s HTTP/1.1\r\nHost: example.com\r\n\r\n", token)))

	var savedFlow *schema.HTTPFlow
	SaveLowHTTPFlow(&lowhttp.LowhttpResponse{
		Https:        false,
		RawRequest:   req,
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
	defer db.Where("id = ?", savedFlow.ID).Delete(&schema.HTTPFlow{})

	require.True(t, testHTTPFlowBareResponseMissing(db, savedFlow.ID))
}

func TestCreateHTTPFlow_FixesEvenWhenRspRawDiffersFromWire(t *testing.T) {
	db := consts.GetGormProjectDatabase()
	token := uuid.NewString()

	gbkBody, err := codec.Utf8ToGB18030([]byte("你好"))
	require.NoError(t, err)
	wire := []byte(fmt.Sprintf("HTTP/1.1 200 OK\r\nContent-Type: text/html; charset=gbk\r\nContent-Length: %d\r\n\r\n%s", len(gbkBody), string(gbkBody)))
	// simulate MITM plainResponse: same bytes as wire but passed as rspRaw (not charset-fixed)
	plain := append([]byte(nil), wire...)
	req := lowhttp.FixHTTPRequest([]byte(fmt.Sprintf("GET /%s HTTP/1.1\r\nHost: example.com\r\n\r\n", token)))

	flow, err := CreateHTTPFlowFromHTTPWithBodySavedFromRaw(
		false, req, plain, "scan", fmt.Sprintf("http://example.com/%s", token), "127.0.0.1:80",
		CreateHTTPFlowWithBareResponseRaw(wire),
	)
	require.NoError(t, err)
	require.NoError(t, InsertHTTPFlow(db, flow))
	defer db.Where("id = ?", flow.ID).Delete(&schema.HTTPFlow{})

	require.Contains(t, flow.GetResponse(), "charset=utf-8")
	require.Contains(t, flow.GetResponse(), "你好")
}

func TestResolveHTTPFlowStoredResponse(t *testing.T) {
	gbkBody, _ := codec.Utf8ToGB18030([]byte("你好"))
	wire := []byte(fmt.Sprintf("HTTP/1.1 200 OK\r\nContent-Type: text/html; charset=gbk\r\nContent-Length: %d\r\n\r\n%s", len(gbkBody), string(gbkBody)))
	fixed, _, _ := lowhttp.FixHTTPResponse(wire)

	stored := resolveHTTPFlowStoredResponse(wire, fixed, nil, false)
	require.Contains(t, string(stored), "charset=utf-8")

	skip := resolveHTTPFlowStoredResponse(wire, fixed, nil, true)
	require.Equal(t, string(wire), string(skip))
}
