package yakgrpc

import (
	"context"
	"errors"
	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/mutate"
	"github.com/yaklang/yaklang/common/utils/memedit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"strings"
)

func (s *Server) GetAllFuzztagInfo(ctx context.Context, req *ypb.GetAllFuzztagInfoRequest) (*ypb.GetAllFuzztagInfoResponse, error) {
	keyWord := req.GetKey()
	allTag := mutate.GetAllFuzztags()
	res := []*ypb.FuzztagInfo{}
	res = lo.Map(allTag, func(item *mutate.FuzzTagDescription, index int) *ypb.FuzztagInfo {
		return &ypb.FuzztagInfo{
			Name:        item.TagName,
			Description: item.Description,
			VerboseName: item.TagNameVerbose,
		}
	})
	res = lo.Filter(res, func(item *ypb.FuzztagInfo, index int) bool {
		for _, s := range []string{item.GetName(), item.GetVerboseName(), item.GetDescription()} {
			if strings.Contains(s, keyWord) {
				return true
			}
		}
		return false
	})
	return &ypb.GetAllFuzztagInfoResponse{Data: res}, nil
}
func (s *Server) GenerateFuzztag(ctx context.Context, req *ypb.GenerateFuzztagRequest) (*ypb.GenerateFuzztagResponse, error) {
	//name := req.GetName()
	typ := req.GetType()
	selectedRange := req.GetRange()
	source := selectedRange.GetCode()
	editor := memedit.NewMemEditor(source)
	startOffset := editor.GetOffsetByPositionRaw(int(selectedRange.StartLine), int(selectedRange.StartColumn))
	endOffset := editor.GetOffsetByPositionRaw(int(selectedRange.EndLine), int(selectedRange.EndColumn))
	isSafeRange := func(offset int) bool {
		return offset >= 0 && offset < len(source)
	}
	if !isSafeRange(startOffset) || !isSafeRange(endOffset) {
		return nil, errors.New("invalid range")
	}
	//prefix := source[:startOffset]
	//suffix := source[endOffset:]
	switch typ {
	case "insert":

	case "wrap":

	default:
		return nil, errors.New("invalid type : " + typ)
	}
	return nil, nil
}
