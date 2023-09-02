package crep

import (
	"github.com/yaklang/yaklang/common/utils"
	"regexp"
	"sync"
)

type TransparentHijackManager struct {
	requestHijacks *sync.Map
}

func NewTransparentHijackManager() *TransparentHijackManager {
	return &TransparentHijackManager{
		requestHijacks: new(sync.Map),
	}
}

func (t *TransparentHijackManager) SetHijackRequestForPath(
	re string,
	requestFunc MITMTransparentHijackFunc,
) error {
	filter := utils.NewHTTPPacketFilter()
	_, err := regexp.Compile(re)
	if err != nil {
		return err
	}
	filter.SetAllowForRequestPath(re)
	t.requestHijacks.Store(filter, requestFunc)
	return nil
}

func (t *TransparentHijackManager) SetHijackRequestForHost(
	re string,
	requsetFunc MITMTransparentHijackFunc,
) error {
	filter := utils.NewHTTPPacketFilter()
	_, err := regexp.Compile(re)
	if err != nil {
		return err
	}
	filter.SetAllowForRequestHeader("Host", re)
	filter.SetAllowForRequestHeader("host", re)
	t.requestHijacks.Store(filter, requsetFunc)
	return nil
}

func (t *TransparentHijackManager) Hijacked(isHttps bool, req []byte) []byte {
	var hijackedReq = req

	reqIns, err := utils.ReadHTTPRequestFromBytes(req)
	if err != nil {
		return hijackedReq
	}

	t.requestHijacks.Range(func(key, value interface{}) bool {
		if key.(*utils.HTTPPacketFilter).IsAllowed(reqIns, nil) {
			raw, ok := value.(MITMTransparentHijackFunc)
			if ok {
				hijackedReq = raw(isHttps, hijackedReq)
				return false
			}
		}
		return true
	})
	return hijackedReq
}

func (t *TransparentHijackManager) List() []string {
	var hashed []string
	t.requestHijacks.Range(func(key, value interface{}) bool {
		f := key.(*utils.HTTPPacketFilter)
		hashed = append(hashed, f.Hash())
		return true
	})
	return hashed
}

func (t *TransparentHijackManager) RemoveByHash(h string) {
	var filter *utils.HTTPPacketFilter
	t.requestHijacks.Range(func(key, value interface{}) bool {
		f := key.(*utils.HTTPPacketFilter)
		if f.Hash() == h {
			filter = f
			return false
		}
		return true
	})
	t.requestHijacks.Delete(filter)
}
