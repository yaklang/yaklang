package yakgrpc

import (
	"context"
	"errors"
	"fmt"
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
		args := []*ypb.FuzztagArgumentType{}
		for _, argumentType := range item.ArgumentTypes {
			defaultVal := ""
			switch argumentType.Name {
			case "range":
				if argumentType.Default != nil {
					defaultVal = fmt.Sprintf("%d-%d", argumentType.Default.([2]int)[0], argumentType.Default.([2]int)[1])
				}
			default:
				defaultVal = fmt.Sprintf("%v", argumentType.Default)
			}
			args = append(args, &ypb.FuzztagArgumentType{
				Name:         argumentType.Name,
				DefaultValue: defaultVal,
				Description:  argumentType.Description,
				IsOptional:   argumentType.IsOptional,
				IsList:       argumentType.IsList,
				Separators:   argumentType.Separator,
			})
		}
		return &ypb.FuzztagInfo{
			Name:          item.TagName,
			Description:   item.Description,
			VerboseName:   item.TagNameVerbose,
			Examples:      item.Examples,
			ArgumentTypes: args,
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
	name := req.GetName()
	typ := req.GetType()
	selectedRange := req.GetRange()
	source := selectedRange.GetCode()
	editor := memedit.NewMemEditor(source)
	startOffset := editor.GetOffsetByPositionRaw(int(selectedRange.StartLine), int(selectedRange.StartColumn))
	endOffset := editor.GetOffsetByPositionRaw(int(selectedRange.EndLine), int(selectedRange.EndColumn))
	var result string
	switch typ {
	case "insert":
		if startOffset < 0 || startOffset > len(source) || endOffset < 0 || endOffset > len(source) {
			return nil, errors.New("invalid range")
		}
		prefix := source[:startOffset]
		suffix := source[endOffset:]
		newTag := fmt.Sprintf("{{%s()}}", name)
		newData := fmt.Sprintf("%s%s%s", prefix, newTag, suffix)
		result = newData
	case "wrap":
		if startOffset < 0 || startOffset > len(source) || endOffset < 0 || endOffset > len(source) {
			return nil, errors.New("invalid range")
		}
		prefix := source[:startOffset]
		suffix := source[endOffset:]
		innerData := source[startOffset:endOffset]
		newTag := fmt.Sprintf("{{%s(%s)}}", name, innerData)
		newData := fmt.Sprintf("%s%s%s", prefix, newTag, suffix)
		result = newData
	default:
		return nil, errors.New("invalid type : " + typ)
	}
	return &ypb.GenerateFuzztagResponse{
		Status: &ypb.GeneralResponse{
			Ok: true,
		},
		Result: result,
	}, nil
}
