package httpctx

import (
	"context"
	"fmt"
	"github.com/ReneKroon/ttlcache"
	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"net/http"
	"strconv"
	"sync"
	"time"
)

var refCache = ttlcache.NewCache()
var contextInfoMutex = new(sync.Mutex)

func init() {
	refCache.SetTTL(200 * time.Second)
}

func GetContextInfoMap(r *http.Request) *sync.Map {
	contextInfoMutex.Lock()
	defer func() {
		contextInfoMutex.Unlock()
	}()

	mHash := fmt.Sprintf("%p", r)
	val, ok := refCache.Get(mHash)
	if !ok {
		result := _getContextInfoMap(r)
		if result != nil {
			refCache.Set(mHash, result)
		}
		return result
	}
	return val.(*sync.Map)
}

func _getContextInfoMap(r *http.Request) *sync.Map {
	value := r.Context().Value(REQUEST_CONTEXT_INFOMAP)
	var infoMap *sync.Map
	if value == nil {
		var ret = new(sync.Map)
		ret.Store("uuid", uuid.New().String())
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
	return []byte(GetContextStringInfoFromRequest(r, REQUEST_CONTEXT_KEY_RequestBytes))
}

func GetRequestHTTPS(r *http.Request) bool {
	return GetContextBoolInfoFromRequest(r, REQUEST_CONTEXT_KEY_IsHttps)
}

func SetRequestHTTPS(r *http.Request, b bool) {
	SetContextValueInfoFromRequest(r, REQUEST_CONTEXT_KEY_IsHttps, b)
}

func SetRequestBytes(r *http.Request, bytes []byte) {
	SetContextValueInfoFromRequest(r, REQUEST_CONTEXT_KEY_RequestBytes, string(bytes))
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

const REQUEST_CONTEXT_INFOMAP = "InfoMap"

const (
	REQUEST_CONTEXT_KEY_IsHttps                      = "isHttps"
	REQUEST_CONTEXT_KEY_IsDropped                    = "isRequestDropped"
	RESPONSE_CONTEXT_KEY_IsDropped                   = "isResponseDropped"
	RESPONSE_CONTEXT_NOLOG                           = "isResponseNoLog"
	REQUEST_CONTEXT_KEY_AutoFoward                   = "isRequestAutoForward"
	RESPONSE_CONTEXT_KEY_AutoFoward                  = "isResponseAutoForward"
	REQUEST_CONTEXT_KEY_Url                          = "url"
	REQUEST_CONTEXT_KEY_IsModified                   = "requestIsModified"
	REQUEST_CONTEXT_KEY_ModifiedBy                   = "requestIsModifiedBy"
	REQUEST_CONTEXT_KEY_Modified                     = "requestModified"
	REQUEST_CONTEXT_KEY_RequestIsFiltered            = "requestIsFiltered"
	RESPONSE_CONTEXT_KEY_ResponseIsFiltered          = "responseIsFiltered"
	REQUEST_CONTEXT_KEY_RequestIsHijacked            = "requestIsHijacked"
	REQUEST_CONTEXT_KEY_RequestBytes                 = "requestBytes"
	REQUEST_CONTEXT_KEY_ResponseBytes                = "responseBytes"
	REQUEST_CONTEXT_KEY_RequestIsStrippedGzip        = "requestIsStrippedGzip"
	RESPONSE_CONTEXT_KEY_ShouldBeHijackedFromRequest = "shouldBeHijackedFromRequest"
	REQUEST_CONTEXT_KEY_ConnectedTo                  = "connectedTo"
	REQUEST_CONTEXT_KEY_ConnectedToPort              = "connectedToPort"
	REQUEST_CONTEXT_KEY_ConnectedToHost              = "connectedToHost"
)
