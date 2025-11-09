package httpctx

import (
	"context"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func GetContextInfoMap(r *http.Request) *sync.Map {
	if r == nil {
		return new(sync.Map)
	}
	if r.Context() == nil {
		*r = *r.WithContext(context.Background())
	}
	raw := r.Context().Value(REQUEST_CONTEXT_INFOMAP)
	if raw == nil {
		return _getContextInfoMap(r)
	}
	result, tOk := raw.(*sync.Map)
	if !tOk {
		return _getContextInfoMap(r)
	}
	return result
}

func _getContextInfoMap(r *http.Request) *sync.Map {
	value := r.Context().Value(REQUEST_CONTEXT_INFOMAP)
	var infoMap *sync.Map
	var uid string
	if value == nil {
		uid = uuid.New().String()
		ret := new(sync.Map)
		ret.Store("uuid", uid)
		*r = *r.WithContext(context.WithValue(r.Context(), REQUEST_CONTEXT_INFOMAP, ret))
		value = ret
		infoMap = ret
	} else {
		var ok bool
		infoMap, ok = value.(*sync.Map)
		if !ok {
			return nil
		}
	}
	if uid == "" {
		uidRaw, ok := infoMap.Load("uuid")
		if ok {
			uid = uidRaw.(string)
		}
	}
	return infoMap
}

func GetContextStringInfoFromRequest(r *http.Request, key string) string {
	infoMap := GetContextInfoMap(r)
	v, ok := infoMap.Load(key)
	if !ok {
		return ""
	}
	return codec.AnyToString(v)
}

func GetRequestBytes(r *http.Request) []byte {
	if r == nil {
		log.Warnf("GetRequestBytes: req is nil")
		return nil
	}

	if ret := GetHijackedRequestBytes(r); len(ret) > 0 {
		return ret
	}

	if ret := GetPlainRequestBytes(r); len(ret) > 0 {
		return ret
	}

	return GetBareRequestBytes(r)
}

func GetResponseBytes(r *http.Request) []byte {
	if r == nil {
		log.Warnf("GetRequestBytes: req is nil")
		return nil
	}

	if ret := GetHijackedResponseBytes(r); len(ret) > 0 {
		return ret
	}

	if ret := GetPlainResponseBytes(r); len(ret) > 0 {
		return ret
	}

	return GetBareResponseBytes(r)
}

func GetRequestHTTPS(r *http.Request) bool {
	return GetContextBoolInfoFromRequest(r, REQUEST_CONTEXT_KEY_IsHttps)
}

func SetRequestHTTPS(r *http.Request, b bool) {
	SetContextValueInfoFromRequest(r, REQUEST_CONTEXT_KEY_IsHttps, b)
}

func GetContextBoolInfoFromRequest(r *http.Request, key string) bool {
	infoMap := GetContextInfoMap(r)
	v, ok := infoMap.Load(key)
	if !ok {
		return false
	}
	switch ret := v.(type) {
	case bool:
		return ret
	default:
		result, err := strconv.ParseBool(codec.AnyToString(v))
		if err != nil {
			log.Warnf("GetContextBoolInfoFromRequest: %v", err)
		}
		return result
	}
}

func GetContextAnyFromRequest(r *http.Request, key string) any {
	infoMap := GetContextInfoMap(r)
	v, ok := infoMap.Load(key)
	if !ok {
		return nil
	}
	return v
}

func GetContextIntInfoFromRequest(r *http.Request, key string) int {
	infoMap := GetContextInfoMap(r)
	v, ok := infoMap.Load(key)
	if !ok {
		return 0
	}
	switch v.(type) {
	case int:
		return v.(int)
	case string:
		return codec.Atoi(v.(string))
	case int64:
		return int(v.(int64))
	case int32:
		return int(v.(int32))
	case int16:
		return int(v.(int16))
	case int8:
		return int(v.(int8))
	default:
		log.Errorf("GetContextIntInfoFromRequest: unknown type %T", v)
		return 0
	}
}

func SetContextValueInfoFromRequest(r *http.Request, key string, value any) {
	infoMap := GetContextInfoMap(r)
	infoMap.Store(key, value)
}

func SetBareRequestBytes(r *http.Request, bytes []byte) {
	//if len(GetBareRequestBytes(r)) != 0 {
	//	log.Debug("SetBareRequestBytes: bare request bytes already set, ignore")
	//	return
	//}
	SetContextValueInfoFromRequest(r, REQUEST_CONTEXT_KEY_RequestBareBytes, string(bytes))
}

func GetNoBodyBuffer(r *http.Request) bool {
	return GetContextBoolInfoFromRequest(r, REQUEST_CONTEXT_KEY_NoBodyBuffer)
}

func SetNoBodyBuffer(r *http.Request, b bool) {
	SetContextValueInfoFromRequest(r, REQUEST_CONTEXT_KEY_NoBodyBuffer, b)
}

func GetBareRequestBytes(r *http.Request) []byte {
	return []byte(GetContextStringInfoFromRequest(r, REQUEST_CONTEXT_KEY_RequestBareBytes))
}

func SetPlainRequestBytes(r *http.Request, bytes []byte) {
	SetContextValueInfoFromRequest(r, REQUEST_CONTEXT_KEY_RequestPlainBytes, string(bytes))
}

func SetHijackedRequestBytes(r *http.Request, bytes []byte) {
	SetContextValueInfoFromRequest(r, REQUEST_CONTEXT_KEY_RequestHijackedBytes, string(bytes))
}

func GetHijackedRequestBytes(r *http.Request) []byte {
	return []byte(GetContextStringInfoFromRequest(r, REQUEST_CONTEXT_KEY_RequestHijackedBytes))
}

func SetHijackedResponseBytes(r *http.Request, bytes []byte) {
	SetContextValueInfoFromRequest(r, REQUEST_CONTEXT_KEY_ResponseHijackedBytes, string(bytes))
}

func GetHijackedResponseBytes(r *http.Request) []byte {
	return []byte(GetContextStringInfoFromRequest(r, REQUEST_CONTEXT_KEY_ResponseHijackedBytes))
}

func SetRemoteAddr(r *http.Request, addr string) {
	SetContextValueInfoFromRequest(r, REQUEST_CONTEXT_KEY_RemoteAddr, addr)
}

func GetRemoteAddr(r *http.Request) string {
	if r.RemoteAddr == "" {
		return GetContextStringInfoFromRequest(r, REQUEST_CONTEXT_KEY_RemoteAddr)
	}
	return r.RemoteAddr
}

func GetPlainRequestBytes(r *http.Request) []byte {
	return []byte(GetContextStringInfoFromRequest(r, REQUEST_CONTEXT_KEY_RequestPlainBytes))
}

func GetBareResponseBytes(r *http.Request) []byte {
	return []byte(GetContextStringInfoFromRequest(r, REQUEST_CONTEXT_KEY_ResponseBareBytes))
}

func GetPlainResponseBytes(r *http.Request) []byte {
	return []byte(GetContextStringInfoFromRequest(r, REQUEST_CONTEXT_KEY_ResponsePlainBytes))
}

func SetPlainResponseBytes(r *http.Request, bytes []byte) {
	SetContextValueInfoFromRequest(r, REQUEST_CONTEXT_KEY_ResponsePlainBytes, string(bytes))
}

func SetBareResponseBytes(r *http.Request, bytes []byte) {
	if len(GetBareResponseBytes(r)) != 0 {
		log.Debug("SetBareResponseBytes: bare response bytes already set, ignore")
		return
	}
	SetContextValueInfoFromRequest(r, REQUEST_CONTEXT_KEY_ResponseBareBytes, string(bytes))
}

func SetBareResponseBytesForce(r *http.Request, bytes []byte) {
	SetContextValueInfoFromRequest(r, REQUEST_CONTEXT_KEY_ResponseBareBytes, string(bytes))
}

const REQUEST_CONTEXT_INFOMAP = "InfoMap"

const (
	REQUEST_CONTEXT_KEY_NoBodyBuffer                 = "noBodyBuffer"
	REQUEST_CONTEXT_KEY_IsHttps                      = "isHttps"
	REQUEST_CONTEXT_KEY_IsDropped                    = "isRequestDropped"
	RESPONSE_CONTEXT_KEY_IsDropped                   = "isResponseDropped"
	RESPONSE_CONTEXT_NOLOG                           = "isResponseNoLog"
	REQUEST_CONTEXT_KEY_AutoFoward                   = "isRequestAutoForward"
	RESPONSE_CONTEXT_KEY_AutoFoward                  = "isResponseAutoForward"
	REQUEST_CONTEXT_KEY_Url                          = "url"
	REQUEST_CONTEXT_KEY_Tags                         = "flowTags"
	REQUEST_CONTEET_KEY_Timestamp                    = "timestamp_request"
	REQUEST_CONTEXT_KEY_ReaderOffset                 = "req_reader_offset"
	RESPONSE_CONTEXT_KEY_ReaderOffset                = "rsp_reader_offset"
	RESPONSE_CONTEXT_KEY_Timestamp                   = "timestamp_response"
	REQUEST_CONTEXT_KEY_RequestIsModified            = "requestIsModified"
	REQUEST_CONTEXT_KEY_ResponseIsModified           = "responseIsModified"
	REQUEST_CONTEXT_KEY_RequestModifiedBy            = "requestIsModifiedBy"
	REQUEST_CONTEXT_KEY_ResponseModifiedBy           = "responseIsModifiedBy"
	REQUEST_CONTEXT_KEY_RequestIsFiltered            = "requestIsFiltered"
	RESPONSE_CONTEXT_KEY_ResponseIsFiltered          = "responseIsFiltered"
	REQUEST_CONTEXT_KEY_RequestIsViewedByUser        = "requestIsHijacked"
	REQUEST_CONTEXT_KEY_ResponseIsViewedByUser       = "responseIsHijacked"
	REQUEST_CONTEXT_KEY_RequestBareBytes             = "requestBareBytes"
	REQUEST_CONTEXT_KEY_RequestHijackedBytes         = "requestHijackedBytes"
	REQUEST_CONTEXT_KEY_RequestPlainBytes            = "requestPlainBytes"
	REQUEST_CONTEXT_KEY_ResponseBareBytes            = "responseBareBytes"
	REQUEST_CONTEXT_KEY_ResponsePlainBytes           = "responsePlainBytes"
	REQUEST_CONTEXT_KEY_ResponseHijackedBytes        = "responseHijackedBytes"
	REQUEST_CONTEXT_KEY_RequestIsStrippedGzip        = "requestIsStrippedGzip"
	RESPONSE_CONTEXT_KEY_ShouldBeHijackedFromRequest = "shouldBeHijackedFromRequest"
	REQUEST_CONTEXT_KEY_ProcessName                  = "ProcessName"
	REQUEST_CONTEXT_ConnectToHTTPS                   = "connectTOHTTPS" // used for CONNECT to HTTPS request
	REQUEST_CONTEXT_KEY_ConnectedTo                  = "connectedTo"
	REQUEST_CONTEXT_KEY_ConnectedToPort              = "connectedToPort"
	REQUEST_CONTEXT_KEY_ConnectedToHost              = "connectedToHost"
	REQUEST_CONTEXT_KEY_RemoteAddr                   = "remoteAddr"
	REQUEST_CONTEXT_KEY_ViaConnect                   = "viaConnect"
	REQUEST_CONTEXT_KEY_ResponseHeaderCallback       = "responseHeaderCallback"
	REQUEST_CONTEXT_KEY_ResponseHeaderWriter         = "responseHeaderWriter"
	REQUEST_CONTEXT_KEY_ResponseMaxContentLength     = "responseMaxContentLength"
	REQUEST_CONTEXT_KEY_ResponseTraceInfo            = "responseTraceInfo"
	REQUEST_CONTEXT_KEY_ResponseTooLarge             = "responseTooLarge"
	REQUEST_CONTEXT_KEY_ResponseTooSlow              = "responseReadTimeTooSlow"
	REQUEST_CONTEXT_KEY_RequestTooLarge              = "requestTooLarge"
	REQUEST_CONTEXT_KEY_ResponseHeaderParsed         = "responseHeaderParsed"
	REQUEST_CONTEXT_KEY_ResponseContentTypeFiltered  = "ResponseContentTypeFiltered"
	REQUEST_CONTEXT_KEY_MitmFrontendReadWriter       = "mitmFrontendReadWriter"
	REQUEST_CONTEXT_KEY_MitmSkipFrontendFeedback     = "mitmSkipFrontendFeedback"
	REQUEST_CONTEXT_KEY_ResponseFinishedCallback     = "responseFinishedCallback"
	REQUEST_CONTEXT_KEY_ResponseTooLargeHeaderFile   = "ResponseTooLargeHeaderFile"
	REQUEST_CONTEXT_KEY_ResponseTooLargeBodyFile     = "ResponseTooLargeBodyFile"
	REQUEST_CONTEXT_KEY_ResponseBodySize             = "ResponseBodySize"
	REQUEST_CONTEXT_KEY_MatchedRules                 = "MatchedRules"
	REQUEST_CONTEXT_KEY_WebsocketRequestHash         = "websocketRequestHash"
	REQUEST_CONTEXT_KEY_RequestProxyProtocol         = "requestProxyProtocol"
	REQUEST_CONTEXT_KEY_IsWebsocketRequest           = "isWebsocketRequest"
	REQUEST_CONTEXT_KEY_PluginContext                = "pluginContext"
	REQUEST_CONTEXT_KEY_PluginContextCancelFunc      = "pluginContextCancelFunc"
	REQUEST_CONTEXT_KEY_MITMTaskID                   = "mitmTaskID"
	REQUEST_CONTEXT_KEY_IsStrongHostMode             = "isStrongHostMode" // Used for transparent hijacking of tun-generated data
)

func SetRequestMITMTaskID(req *http.Request, id string) {
	SetContextValueInfoFromRequest(req, REQUEST_CONTEXT_KEY_MITMTaskID, id)
}

func GetRequestMITMTaskID(req *http.Request) string {
	return GetContextStringInfoFromRequest(req, REQUEST_CONTEXT_KEY_MITMTaskID)
}

func SetRequestProxyProtocol(req *http.Request, p string) {
	SetContextValueInfoFromRequest(req, REQUEST_CONTEXT_KEY_RequestProxyProtocol, p)
}

func GetRequestProxyProtocol(req *http.Request) string {
	return GetContextStringInfoFromRequest(req, REQUEST_CONTEXT_KEY_RequestProxyProtocol)
}

func SetResponseBodySize(req *http.Request, i int64) {
	SetContextValueInfoFromRequest(req, REQUEST_CONTEXT_KEY_ResponseBodySize, i)
}

func GetResponseBodySize(req *http.Request) int64 {
	return int64(GetContextIntInfoFromRequest(req, REQUEST_CONTEXT_KEY_ResponseBodySize))
}

func SetResponseTooLargeHeaderFile(req *http.Request, b string) {
	SetContextValueInfoFromRequest(req, REQUEST_CONTEXT_KEY_ResponseTooLargeHeaderFile, b)
}

func GetResponseTooLargeHeaderFile(req *http.Request) string {
	return GetContextStringInfoFromRequest(req, REQUEST_CONTEXT_KEY_ResponseTooLargeHeaderFile)
}

func GetResponseTooLargeBodyFile(req *http.Request) string {
	return GetContextStringInfoFromRequest(req, REQUEST_CONTEXT_KEY_ResponseTooLargeBodyFile)
}

func SetResponseTooLargeBodyFile(req *http.Request, b string) {
	SetContextValueInfoFromRequest(req, REQUEST_CONTEXT_KEY_ResponseTooLargeBodyFile, b)
}

func SetResponseContentTypeFiltered(req *http.Request, matcher func(contentType string) bool) {
	SetContextValueInfoFromRequest(req, REQUEST_CONTEXT_KEY_ResponseContentTypeFiltered, matcher)
}

func GetResponseContentTypeFiltered(req *http.Request) func(contentType string) bool {
	if req == nil {
		return nil
	}
	if ret := GetContextAnyFromRequest(req, REQUEST_CONTEXT_KEY_ResponseContentTypeFiltered); ret != nil {
		if rw, ok := ret.(func(contentType string) bool); ok {
			return rw
		}
	}
	return nil
}

// SetMITMFrontendReadWriter sets the mitm frontend read writer
func SetMITMFrontendReadWriter(r *http.Request, rw io.ReadWriter) {
	SetContextValueInfoFromRequest(r, REQUEST_CONTEXT_KEY_MitmFrontendReadWriter, rw)
}

// SetMITMSkipFrontendFeedback means: the frontend should skip feedback
func SetMITMSkipFrontendFeedback(r *http.Request, b bool) {
	SetContextValueInfoFromRequest(r, REQUEST_CONTEXT_KEY_MitmSkipFrontendFeedback, b)
}

// GetMITMSkipFrontendFeedback gets the mitm frontend read writer
func GetMITMSkipFrontendFeedback(r *http.Request) bool {
	if r == nil {
		return false
	}
	if ret := GetContextAnyFromRequest(r, REQUEST_CONTEXT_KEY_MitmSkipFrontendFeedback); ret != nil {
		if rw, ok := ret.(bool); ok {
			return rw
		}
	}
	return false
}

// GetMITMFrontendReadWriter gets the mitm frontend read writer
func GetMITMFrontendReadWriter(r *http.Request) io.ReadWriter {
	if r == nil {
		return nil
	}
	if ret := GetContextAnyFromRequest(r, REQUEST_CONTEXT_KEY_MitmFrontendReadWriter); ret != nil {
		if rw, ok := ret.(io.ReadWriter); ok {
			return rw
		}
	}
	return nil
}

// ResponseHeaderParsedCallback defines how response header is parsed for handling
type ResponseHeaderParsedCallback func(key string, value string)

func SetResponseHeaderParsed(r *http.Request, cb ResponseHeaderParsedCallback) {
	SetContextValueInfoFromRequest(r, REQUEST_CONTEXT_KEY_ResponseHeaderParsed, cb)
}

func GetResponseHeaderParsed(r *http.Request) ResponseHeaderParsedCallback {
	if r == nil {
		return nil
	}
	rs := GetContextAnyFromRequest(r, REQUEST_CONTEXT_KEY_ResponseHeaderParsed)
	if rs == nil {
		return nil
	}
	cb, ok := rs.(ResponseHeaderParsedCallback)
	if !ok {
		return nil
	}
	return cb
}

// IsFiltered returns true if the request is filtered out
// filtered request/response will not be logged into database
func IsFiltered(req *http.Request) bool {
	return GetContextBoolInfoFromRequest(req, REQUEST_CONTEXT_KEY_RequestIsFiltered) || GetContextBoolInfoFromRequest(req, RESPONSE_CONTEXT_KEY_ResponseIsFiltered)
}

func IsResponseFiltered(req *http.Request) bool {
	return GetContextBoolInfoFromRequest(req, RESPONSE_CONTEXT_KEY_ResponseIsFiltered)
}

func GetResponseTooLarge(req *http.Request) bool {
	return GetContextBoolInfoFromRequest(req, REQUEST_CONTEXT_KEY_ResponseTooLarge)
}

func SetResponseTooLarge(req *http.Request, b bool) {
	SetContextValueInfoFromRequest(req, REQUEST_CONTEXT_KEY_ResponseTooLarge, b)
}
func GetResponseReadTooSlow(req *http.Request) bool {
	return GetContextBoolInfoFromRequest(req, REQUEST_CONTEXT_KEY_ResponseTooSlow)
}

func SetResponseReadTooSlow(req *http.Request, b bool) {
	SetContextValueInfoFromRequest(req, REQUEST_CONTEXT_KEY_ResponseTooSlow, b)
}

func SetResponseTooLargeSize(req *http.Request, size int64) {
	SetContextValueInfoFromRequest(req, REQUEST_CONTEXT_KEY_ResponseMaxContentLength, size)
}

func GetResponseTooLargeSize(req *http.Request) int64 {
	return int64(GetContextIntInfoFromRequest(req, REQUEST_CONTEXT_KEY_ResponseMaxContentLength))
}

func GetRequestTooLarge(req *http.Request) bool {
	return GetContextBoolInfoFromRequest(req, REQUEST_CONTEXT_KEY_RequestTooLarge)
}

func SetRequestTooLarge(req *http.Request, b bool) {
	SetContextValueInfoFromRequest(req, REQUEST_CONTEXT_KEY_RequestTooLarge, b)
}

func GetResponseMaxContentLength(req *http.Request) int {
	return GetContextIntInfoFromRequest(req, REQUEST_CONTEXT_KEY_ResponseMaxContentLength)
}

func SetResponseMaxContentLength(req *http.Request, length int) {
	SetContextValueInfoFromRequest(req, REQUEST_CONTEXT_KEY_ResponseMaxContentLength, length)
}

type ResponseFinishedCallbackType func()

func GetResponseFinishedCallback(r *http.Request) ResponseFinishedCallbackType {
	if r == nil {
		return nil
	}
	rs := GetContextAnyFromRequest(r, REQUEST_CONTEXT_KEY_ResponseFinishedCallback)
	if rs == nil {
		return nil
	}
	cb, ok := rs.(ResponseFinishedCallbackType)
	if !ok {
		return nil
	}
	return cb
}

func SetResponseFinishedCallback(req *http.Request, h ResponseFinishedCallbackType) {
	SetContextValueInfoFromRequest(req, REQUEST_CONTEXT_KEY_ResponseFinishedCallback, h)
}

type ResponseHeaderCallbackType func(response *http.Response, headerBytes []byte, bodyReader io.Reader) (io.Reader, error)

func SetResponseHeaderCallback(req *http.Request, callback ResponseHeaderCallbackType) {
	SetContextValueInfoFromRequest(req, REQUEST_CONTEXT_KEY_ResponseHeaderCallback, callback)
}

func SetResponseHeaderWriter(req *http.Request, w io.Writer) {
	SetContextValueInfoFromRequest(req, REQUEST_CONTEXT_KEY_ResponseHeaderWriter, w)
}

func GetResponseHeaderWriter(req *http.Request) io.Writer {
	if req == nil {
		return nil
	}
	if ret := GetContextAnyFromRequest(req, REQUEST_CONTEXT_KEY_ResponseHeaderWriter); ret != nil {
		if w, ok := ret.(io.Writer); ok {
			return w
		}
	}
	return nil
}

func GetResponseHeaderCallback(req *http.Request) ResponseHeaderCallbackType {
	if req == nil {
		return nil
	}
	rs := GetContextAnyFromRequest(req, REQUEST_CONTEXT_KEY_ResponseHeaderCallback)
	if rs == nil {
		return nil
	}
	cb, ok := rs.(ResponseHeaderCallbackType)
	if !ok {
		return nil
	}
	return cb
}

func GetMatchedRule(req *http.Request) []*ypb.MITMContentReplacer {
	results, ok := GetContextAnyFromRequest(req, REQUEST_CONTEXT_KEY_MatchedRules).([]*ypb.MITMContentReplacer)
	if ok {
		return results
	}
	return nil
}

func AppendMatchedRule(req *http.Request, rule ...*ypb.MITMContentReplacer) {
	if len(rule) == 0 {
		return
	}
	results := GetMatchedRule(req)
	if results == nil {
		results = make([]*ypb.MITMContentReplacer, 0)
	}
	results = append(results, rule...)
	SetContextValueInfoFromRequest(req, REQUEST_CONTEXT_KEY_MatchedRules, results)
}

func SetMatchedRule(req *http.Request, rule []*ypb.MITMContentReplacer) {
	SetContextValueInfoFromRequest(req, REQUEST_CONTEXT_KEY_MatchedRules, rule)
}

func SetRequestModified(req *http.Request, by ...string) {
	SetContextValueInfoFromRequest(req, REQUEST_CONTEXT_KEY_RequestIsModified, true)
	modified := GetContextStringInfoFromRequest(req, REQUEST_CONTEXT_KEY_RequestModifiedBy)
	if modified != "" {
		by = append(by, modified)
	}
	if len(by) > 0 {
		SetContextValueInfoFromRequest(req, REQUEST_CONTEXT_KEY_RequestModifiedBy, strings.Join(by, "->"))
	}
}

func GetRequestIsModified(req *http.Request) bool {
	return GetContextBoolInfoFromRequest(req, REQUEST_CONTEXT_KEY_RequestIsModified)
}

func SetResponseModified(req *http.Request, by ...string) {
	SetContextValueInfoFromRequest(req, REQUEST_CONTEXT_KEY_ResponseIsModified, true)
	modified := GetContextStringInfoFromRequest(req, REQUEST_CONTEXT_KEY_ResponseModifiedBy)
	if modified != "" {
		by = append(by, modified)
	}
	if len(by) > 0 {
		SetContextValueInfoFromRequest(req, REQUEST_CONTEXT_KEY_ResponseModifiedBy, strings.Join(by, "->"))
	}
}

func GetResponseIsModified(req *http.Request) bool {
	return GetContextBoolInfoFromRequest(req, REQUEST_CONTEXT_KEY_ResponseIsModified)
}

func SetRequestURL(req *http.Request, urlStr string) {
	SetContextValueInfoFromRequest(req, REQUEST_CONTEXT_KEY_Url, urlStr)
}

func GetRequestURL(req *http.Request) string {
	return GetContextStringInfoFromRequest(req, REQUEST_CONTEXT_KEY_Url)
}

func SetRequestViewedByUser(req *http.Request) {
	SetContextValueInfoFromRequest(req, REQUEST_CONTEXT_KEY_RequestIsViewedByUser, true)
}

func GetRequestViewedByUser(req *http.Request) bool {
	return GetContextBoolInfoFromRequest(req, REQUEST_CONTEXT_KEY_RequestIsViewedByUser)
}

func SetResponseViewedByUser(req *http.Request) {
	SetContextValueInfoFromRequest(req, REQUEST_CONTEXT_KEY_ResponseIsViewedByUser, true)
}

func GetResponseViewedByUser(req *http.Request) bool {
	return GetContextBoolInfoFromRequest(req, REQUEST_CONTEXT_KEY_ResponseIsViewedByUser)
}

func GetRequestViaCONNECT(req *http.Request) bool {
	return GetContextBoolInfoFromRequest(req, REQUEST_CONTEXT_KEY_ViaConnect)
}

func SetRequestViaCONNECT(req *http.Request, b bool) {
	SetContextValueInfoFromRequest(req, REQUEST_CONTEXT_KEY_ViaConnect, b)
}

func GetFlowTags(r *http.Request) []string {
	v := GetContextAnyFromRequest(r, REQUEST_CONTEXT_KEY_Tags)
	switch ret := v.(type) {
	case []string:
		return ret
	}
	return []string{}
}

func SetFlowTags(r *http.Request, tags []string) {
	SetContextValueInfoFromRequest(r, REQUEST_CONTEXT_KEY_Tags, tags)
}

func SetRequestTimestamp(r *http.Request, ts time.Time) {
	SetContextValueInfoFromRequest(r, REQUEST_CONTEET_KEY_Timestamp, ts)
}

func GetRequestTimestamp(r *http.Request) time.Time {
	v := GetContextAnyFromRequest(r, REQUEST_CONTEET_KEY_Timestamp)
	switch ret := v.(type) {
	case time.Time:
		return ret
	}
	return time.Time{}
}

func SetResponseTimestamp(r *http.Response, ts time.Time) {
	if r.Request == nil {
		return
	}
	SetContextValueInfoFromRequest(r.Request, RESPONSE_CONTEXT_KEY_Timestamp, ts)
}

func GetResponseTimestamp(r *http.Response) time.Time {
	if r.Request == nil {
		return time.Time{}
	}
	v := GetContextAnyFromRequest(r.Request, RESPONSE_CONTEXT_KEY_Timestamp)
	switch ret := v.(type) {
	case time.Time:
		return ret
	}
	return time.Time{}
}

func SetRequestReaderOffset(r *http.Request, offset int) {
	SetContextValueInfoFromRequest(r, REQUEST_CONTEXT_KEY_ReaderOffset, offset)
}

func GetRequestReaderOffset(r *http.Request) int {
	return GetContextIntInfoFromRequest(r, REQUEST_CONTEXT_KEY_ReaderOffset)
}

func SetResponseTraceInfo(req *http.Request, info any) {
	SetContextValueInfoFromRequest(req, REQUEST_CONTEXT_KEY_ResponseTraceInfo, info)
}

func GetResponseTraceInfo(req *http.Request) any {
	// could not assert to *lowhttp.LowhttpTraceInfo, because of circular import
	return GetContextAnyFromRequest(req, REQUEST_CONTEXT_KEY_ResponseTraceInfo)
}

func GetWebsocketRequestHash(r *http.Request) string {
	return GetContextStringInfoFromRequest(r, REQUEST_CONTEXT_KEY_WebsocketRequestHash)
}

func SetWebsocketRequestHash(r *http.Request, hash string) {
	SetContextValueInfoFromRequest(r, REQUEST_CONTEXT_KEY_WebsocketRequestHash, hash)
}

func GetIsWebWebsocketRequest(r *http.Request) bool {
	val := GetContextAnyFromRequest(r, REQUEST_CONTEXT_KEY_IsWebsocketRequest)
	if val == "" {
		return false
	}
	if v, ok := val.(bool); ok {
		return v
	}
	return false
}

func SetIsWebWebsocketRequest(r *http.Request) {
	SetContextValueInfoFromRequest(r, REQUEST_CONTEXT_KEY_IsWebsocketRequest, true)
}

func SetPluginContext(r *http.Request, ctx context.Context) {
	SetContextValueInfoFromRequest(r, REQUEST_CONTEXT_KEY_PluginContext, ctx)
}

func GetPluginContext(r *http.Request) context.Context {
	ctx := GetContextAnyFromRequest(r, REQUEST_CONTEXT_KEY_PluginContext)
	if ctx == nil {
		return nil
	}
	if c, ok := ctx.(context.Context); ok {
		return c
	}
	return nil
}

func SetProcessName(r *http.Request, name string) {
	SetContextValueInfoFromRequest(r, REQUEST_CONTEXT_KEY_ProcessName, name)
}

func GetProcessName(r *http.Request) string {
	return GetContextStringInfoFromRequest(r, REQUEST_CONTEXT_KEY_ProcessName)
}

// SetIsStrongHostMode sets the strong host mode flag in httpctx
// This is critical for transparent hijacking of tun-generated data
func SetIsStrongHostMode(r *http.Request, b bool) {
	SetContextValueInfoFromRequest(r, REQUEST_CONTEXT_KEY_IsStrongHostMode, b)
}

// GetIsStrongHostMode gets the strong host mode flag from httpctx
func GetIsStrongHostMode(r *http.Request) bool {
	return GetContextBoolInfoFromRequest(r, REQUEST_CONTEXT_KEY_IsStrongHostMode)
}
