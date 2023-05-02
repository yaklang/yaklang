package utils

import (
	"fmt"
	"net/http"
	"net/http/httputil"
	"regexp"
	"sort"
	"strings"
	"sync"
	"yaklang/common/log"
)

type httpPacketFilterCondType string
type httpFilterAction string

const (
	httpFilter_RequestPath    httpPacketFilterCondType = "path"
	httpFilter_RequestHeader  httpPacketFilterCondType = "req-header"
	httpFilter_ResponseHeader httpPacketFilterCondType = "rsp-header"
	httpFilter_RequestRaw     httpPacketFilterCondType = "req-raw"
	httpFilter_ResponseRaw    httpPacketFilterCondType = "rsp-raw"

	httpFilterAction_Reject httpFilterAction = "reject"
	httpFilterAction_Allow  httpFilterAction = "allow"
)

type httpPacketFilterCondition struct {
	Type   httpPacketFilterCondType
	Op1    string
	Op2    string
	Action httpFilterAction
}

func (h *httpPacketFilterCondition) Name() string {
	if h.Op2 == "" {
		return fmt.Sprintf("[%v]-[OP:%v]-[%v]", h.Type, h.Op1, h.Action)
	}
	return fmt.Sprintf("[%v]-[OP1:(%v)|OP2:(%v)]-[%v]", h.Type, h.Op1, h.Op2, h.Action)
}

func (h *httpPacketFilterCondition) IsAllowed(req *http.Request, rsp *http.Response) bool {
	var (
		matched bool
	)

	if req != nil {
		var (
			err      error
			op1, op2 string
		)
		switch h.Type {
		case httpFilter_RequestPath:
			matched, err = regexp.MatchString(h.Op1, req.RequestURI)
			op1 = h.Op1
			op2 = req.RequestURI
		case httpFilter_RequestRaw:
			raw, _ := httputil.DumpRequest(req, true)
			matched, err = regexp.Match(h.Op1, raw)
			op1 = h.Op1
			op2 = EscapeInvalidUTF8Byte(raw)
		case httpFilter_RequestHeader:
			matched, err = regexp.MatchString(h.Op2, req.Header.Get(h.Op1))
			op1 = h.Op1
			op2 = h.Op2
		}
		if err != nil {
			log.Errorf("regexp[%v] failed: op1: %v op2: %v", h.Type, op1, op2)
		}
	}

	if rsp != nil {
		switch h.Type {
		case httpFilter_ResponseHeader:
			matched, _ = regexp.MatchString(h.Op2, rsp.Header.Get(h.Op1))
		case httpFilter_ResponseRaw:
			raw, _ := httputil.DumpResponse(rsp, true)
			matched, _ = regexp.Match(h.Op1, raw)
		}
	}

	switch h.Action {
	case httpFilterAction_Allow:
		return matched
	case httpFilterAction_Reject:
		return !matched
	default:
		return matched
	}
}

type HTTPPacketFilter struct {
	conds *sync.Map // map[string]*httpPacketFilterCondition
}

func (h *HTTPPacketFilter) Hash() string {
	a := h.Conditions()
	sort.Strings(a)
	return strings.Join(a, "|")
}

func NewHTTPPacketFilter() *HTTPPacketFilter {
	return &HTTPPacketFilter{conds: new(sync.Map)}
}

func (h *HTTPPacketFilter) IsAllowed(req *http.Request, rsp *http.Response) bool {
	var result = true
	h.conds.Range(func(key, value interface{}) bool {
		if value.(*httpPacketFilterCondition).IsAllowed(req, rsp) {
			return true
		} else {
			result = false
			return false
		}
	})
	return result
}

func (h *HTTPPacketFilter) addFilter(t httpPacketFilterCondType, op1, op2 string, action httpFilterAction) {
	cond := &httpPacketFilterCondition{
		Type:   t,
		Op1:    op1,
		Op2:    op2,
		Action: action,
	}
	h.conds.Store(cond.Name(), cond)
}

func (j *HTTPPacketFilter) SetAllowForRequestPath(regexp string) {
	j.addFilter(httpFilter_RequestPath, regexp, "", httpFilterAction_Allow)
}

func (j *HTTPPacketFilter) SetAllowForRequestHeader(header, regexp string) {
	j.addFilter(httpFilter_RequestHeader, header, regexp, httpFilterAction_Allow)
}

func (j *HTTPPacketFilter) SetAllowForRequestRaw(regexp string) {
	j.addFilter(httpFilter_RequestRaw, regexp, "", httpFilterAction_Allow)
}

func (j *HTTPPacketFilter) SetAllowForResponseHeader(header, regexp string) {
	j.addFilter(httpFilter_ResponseHeader, header, regexp, httpFilterAction_Allow)
}

func (j *HTTPPacketFilter) SetAllowForResponseRaw(regexp string) {
	j.addFilter(httpFilter_ResponseRaw, regexp, "", httpFilterAction_Allow)
}

func (j *HTTPPacketFilter) SetRejectForRequestPath(regexp string) {
	j.addFilter(httpFilter_RequestPath, regexp, "", httpFilterAction_Reject)
}

func (j *HTTPPacketFilter) SetRejectForRequestHeader(header, regexp string) {
	j.addFilter(httpFilter_RequestHeader, header, regexp, httpFilterAction_Reject)
}

func (j *HTTPPacketFilter) SetRejectForRequestRaw(regexp string) {
	j.addFilter(httpFilter_RequestRaw, regexp, "", httpFilterAction_Reject)
}

func (j *HTTPPacketFilter) SetRejectForResponseHeader(header, regexp string) {
	j.addFilter(httpFilter_ResponseHeader, header, regexp, httpFilterAction_Reject)
}

func (j *HTTPPacketFilter) SetRejectForResponseRaw(regexp string) {
	j.addFilter(httpFilter_ResponseRaw, regexp, "", httpFilterAction_Reject)
}

func (j *HTTPPacketFilter) Conditions() []string {
	var conds []string
	j.conds.Range(func(key, value interface{}) bool {
		conds = append(conds, key.(string))
		return true
	})
	return conds
}

func (i *HTTPPacketFilter) Remove(name string) {
	i.conds.Delete(name)
}
