package base

import (
	"errors"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

type BaseActionHandler func(getParam func(key string) (string, error), body []byte, raw []*ypb.KVPair) (*ypb.RequestYakURLResponse, error)
type BaseAction struct {
	handlers map[string]BaseActionHandler
}

func GetQueryParam(params []*ypb.KVPair, key string) string {
	for _, param := range params {
		if param.Key == key {
			return param.Value
		}
	}
	return ""
}
func (b *BaseAction) Handle(method string, group string, handler BaseActionHandler) {
	if b.handlers == nil {
		b.handlers = map[string]BaseActionHandler{}
	}
	b.handlers[method+group] = handler
}

func (b *BaseAction) Get(params *ypb.RequestYakURLParams) (*ypb.RequestYakURLResponse, error) {
	return b.Do(params)
}

func (b *BaseAction) Post(params *ypb.RequestYakURLParams) (*ypb.RequestYakURLResponse, error) {
	return b.Do(params)
}

func (b *BaseAction) Put(params *ypb.RequestYakURLParams) (*ypb.RequestYakURLResponse, error) {
	return b.Do(params)
}

func (b *BaseAction) Delete(params *ypb.RequestYakURLParams) (*ypb.RequestYakURLResponse, error) {
	return b.Do(params)
}

func (b *BaseAction) Head(params *ypb.RequestYakURLParams) (*ypb.RequestYakURLResponse, error) {
	return b.Do(params)
}

func (b *BaseAction) Do(params *ypb.RequestYakURLParams) (*ypb.RequestYakURLResponse, error) {
	url := params.Url
	if v, ok := b.handlers[params.Method+url.Path]; ok {
		getParams := map[string]string{}
		for _, pair := range params.Url.GetQuery() {
			getParams[pair.Key] = pair.Value
		}
		return v(func(key string) (string, error) {
			if v, ok := getParams[key]; ok {
				return v, nil
			} else {
				return "", utils.Errorf("not found param: %s", key)
			}
		}, params.Body, params.Url.Query)
	} else {
		return nil, errors.New("not found")
	}
}
