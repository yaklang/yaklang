package httpctx

import (
	"context"
	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"net/http"
	"strconv"
	"strings"
	"sync"
)

func GetContextInfoMap(r *http.Request) *sync.Map {
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
		var ret = new(sync.Map)
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
		var uidRaw, ok = infoMap.Load("uuid")
		if ok {
			uid = uidRaw.(string)
		}
	}
	return infoMap
}

func GetContextStringInfoFromRequest(r *http.Request, key string) string {
	var infoMap = GetContextInfoMap(r)
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

func GetRequestHTTPS(r *http.Request) bool {
	return GetContextBoolInfoFromRequest(r, REQUEST_CONTEXT_KEY_IsHttps)
}

func SetRequestHTTPS(r *http.Request, b bool) {
	SetContextValueInfoFromRequest(r, REQUEST_CONTEXT_KEY_IsHttps, b)
}

func GetContextBoolInfoFromRequest(r *http.Request, key string) bool {
	var infoMap = GetContextInfoMap(r)
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
	var infoMap = GetContextInfoMap(r)
	v, ok := infoMap.Load(key)
	if !ok {
		return nil
	}
	return v
}

func GetContextIntInfoFromRequest(r *http.Request, key string) int {
	var infoMap = GetContextInfoMap(r)
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
	var infoMap = GetContextInfoMap(r)
	infoMap.Store(key, value)
}

func SetBareRequestBytes(r *http.Request, bytes []byte) {
	//if len(GetBareRequestBytes(r)) != 0 {
	//	log.Debug("SetBareRequestBytes: bare request bytes already set, ignore")
	//	return
	//}
	SetContextValueInfoFromRequest(r, REQUEST_CONTEXT_KEY_RequestBareBytes, string(bytes))
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

const REQUEST_CONTEXT_INFOMAP = "InfoMap"

const (
	REQUEST_CONTEXT_KEY_IsHttps                      = "isHttps"
	REQUEST_CONTEXT_KEY_IsDropped                    = "isRequestDropped"
	RESPONSE_CONTEXT_KEY_IsDropped                   = "isResponseDropped"
	RESPONSE_CONTEXT_NOLOG                           = "isResponseNoLog"
	REQUEST_CONTEXT_KEY_AutoFoward                   = "isRequestAutoForward"
	RESPONSE_CONTEXT_KEY_AutoFoward                  = "isResponseAutoForward"
	REQUEST_CONTEXT_KEY_Url                          = "url"
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
	REQUEST_CONTEXT_KEY_ConnectedTo                  = "connectedTo"
	REQUEST_CONTEXT_KEY_ConnectedToPort              = "connectedToPort"
	REQUEST_CONTEXT_KEY_ConnectedToHost              = "connectedToHost"
	REQUEST_CONTEXT_KEY_RemoteAddr                   = "remoteAddr"
	REQUEST_CONTEXT_KEY_ViaConnect                   = "viaConnect"

	// matched mitm rules
	REQUEST_CONTEXT_KEY_MatchedRules = "MatchedRules"
)

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
