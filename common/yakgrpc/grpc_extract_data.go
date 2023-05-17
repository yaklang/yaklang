package yakgrpc

import (
	"bytes"
	"context"
	"fmt"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"regexp"
	"strings"
	"sync"
)

func (s *Server) ExtractData(server ypb.FuzzerApi_ExtractDataServer) error {
	for {
		req, err := server.Recv()
		if err != nil {
			return err
		}
		if req.GetEnd() {
			return nil
		}
		result, err := execExtractRequest(req)
		if err != nil {
			return utils.Errorf("extract error: %s", err)
		}
		server.Send(&ypb.ExtractDataResponse{
			Token:     req.GetToken(),
			Extracted: []byte(result),
		})
	}
}

var _cacheForExtractingRequest = new(sync.Map)

func execExtractRequest(req *ypb.ExtractDataRequest) (string, error) {
	data := req.GetData()
	if data == nil {
		return "", utils.Error("extract data is empty...")
	}
	dataString := string(data)

	var err error
	switch strings.ToLower(req.GetMode()) {
	case "regexp":
		if req.GetMatchRegexp() == "" {
			return "", utils.Error("empty mach regexp")
		}
		reRule := req.GetMatchRegexp()
		reRuleSha1 := utils.CalcSha1(reRule)
		var ins *regexp.Regexp
		insRaw, ok := _cacheForExtractingRequest.Load(reRuleSha1)
		if !ok {
			ins, err = regexp.Compile(reRule)
			if err != nil {
				return "", utils.Errorf("compile regexp-between re failed: %s", err)
			}
			_cacheForExtractingRequest.Store(reRuleSha1, ins)
		} else {
			ins = insRaw.(*regexp.Regexp)
		}
		results := ins.FindSubmatch(data)
		if results != nil {
			if len(results) > 1 {
				return string(bytes.Join(results[1:], []byte(" "))), nil
			}
			//start, end := results[0][0], results[0][1]
			return string(results[0]), nil
		} else {
			return "", nil
		}
	case "regexp-between":
		if req.GetPrefixRegexp() == "" && req.GetSuffixRegexp() == "" {
			return "", utils.Error("regexp-between mode cannot use empty prefix/suffix at same time")
		}
		prefixRe := req.GetPrefixRegexp()
		if prefixRe == "" {
			prefixRe = "^"
		}
		suffixRe := req.GetSuffixRegexp()
		if suffixRe == "" {
			suffixRe = "$"
		}
		reRule := fmt.Sprintf("(?sU)(%v)(?P<extracted>.+)(%v)", prefixRe, suffixRe)
		reRuleSha1 := utils.CalcSha1(reRule)
		var ins *regexp.Regexp
		insRaw, ok := _cacheForExtractingRequest.Load(reRuleSha1)
		if !ok {
			ins, err = regexp.Compile(reRule)
			if err != nil {
				return "", utils.Errorf("compile regexp-between re failed: %s", err)
			}
			_cacheForExtractingRequest.Store(reRuleSha1, ins)
		} else {
			ins = insRaw.(*regexp.Regexp)
		}
		subs := ins.FindAllStringSubmatch(dataString, 1)
		if len(subs) == 0 {
			return "", nil
		}
		firstSubmatches := subs[0]
		if firstSubmatches != nil {
			return firstSubmatches[ins.SubexpIndex("extracted")], nil
		} else {
			return "", nil
		}
	default:
		return "", utils.Errorf("no mode: %v", req.GetMode())
	}
}

func (s *Server) GenerateExtractRule(
	ctx context.Context,
	req *ypb.GenerateExtractRuleRequest,
) (*ypb.GenerateExtractRuleResponse, error) {
	offsetSize := req.GetOffsetSize()
	if offsetSize <= 0 {
		offsetSize = 20
	}

	//log.Infof("extracted prefix/suffix by selected: %v", strconv.Quote(string(req.GetSelected())))
	pre, suf, err := extractPrefixAndSuffix(req.GetData(), req.GetSelected(), int(offsetSize))
	if err != nil {
		return nil, err
	}

	matched := handleRegexpMeta(string(req.GetSelected()))
	rsp := &ypb.GenerateExtractRuleResponse{
		PrefixRegexp:   string(pre),
		SuffixRegexp:   string(suf),
		SelectedRegexp: string(matched),
	}
	return rsp, nil
}

func extractPrefixAndSuffix(req []byte, selected []byte, offsetSize int) (string, string, error) {
	if req == nil || selected == nil {
		return "", "", utils.Error("empty data and selected data")
	}
	var prefixStart = 0
	var suffixEnd = len(req)
	selectedStartIndex := bytes.Index(req, selected)
	if selectedStartIndex < 0 {
		return "", "", utils.Error("cannot found selected as substr in req")
	}
	if selectedStartIndex-prefixStart >= offsetSize {
		prefixStart = selectedStartIndex - offsetSize
	}

	selectedEndIndex := selectedStartIndex + len(selected)
	if suffixEnd-selectedEndIndex >= offsetSize {
		suffixEnd = selectedEndIndex + offsetSize
	}

	prefixBytes, suffixBytes := req[prefixStart:selectedStartIndex], req[selectedEndIndex:suffixEnd]
	return handleRegexpMeta(string(prefixBytes)), handleRegexpMeta(string(suffixBytes)), nil
}

var matchMumber = regexp.MustCompile(`\d+`)

func handleRegexpMeta(i string) string {
	m := regexp.QuoteMeta(string(i))
	m = strings.ReplaceAll(m, "\n", `\r?[\n]`)
	m = strings.ReplaceAll(m, "\r", `[\r]+`)
	m = matchMumber.ReplaceAllStringFunc(m, func(s string) string {
		return fmt.Sprintf(`\d{%d}`, len(s))
	})
	return m
}
