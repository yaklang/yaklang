package yakgrpc

import (
	"bytes"
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/httptpl"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func (s *Server) ExtractData(server ypb.Yak_ExtractDataServer) error {
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

var _cacheForExtractingRequest = utils.NewTTLCache[*httptpl.YakExtractor](10 * time.Second)

func execExtractRequest(req *ypb.ExtractDataRequest) (string, error) {
	data := req.GetData()
	if data == nil {
		return "", utils.Error("extract data is empty...")
	}
	var reRule string
	var reGroupName []string
	switch strings.ToLower(req.GetMode()) {
	case "regexp":
		if req.GetMatchRegexp() == "" {
			return "", utils.Error("empty mach regexp")
		}
		reRule = req.GetMatchRegexp()
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
		reRule = fmt.Sprintf("(?sU)(%v)(?P<extracted>.+)(%v)", prefixRe, suffixRe)
		reGroupName = []string{"extracted"}
	default:
		return "", utils.Errorf("no mode: %v", req.GetMode())
	}
	reRuleSha1 := utils.CalcSha1(fmt.Sprintf("%v-%v", reRule, reGroupName))
	var extractor *httptpl.YakExtractor
	if extractor, _ = _cacheForExtractingRequest.Get(reRuleSha1); extractor == nil {
		extractor = &httptpl.YakExtractor{
			Id:                   0,
			Name:                 "",
			Type:                 "regex",
			Scope:                "all",
			Groups:               []string{reRule},
			RegexpMatchGroup:     nil,
			RegexpMatchGroupName: reGroupName,
			XPathAttribute:       "",
		}
		_cacheForExtractingRequest.Set(reRuleSha1, extractor)
	}
	resMap, err := extractor.Execute(data)
	if resMap == nil {
		return "", utils.Error("extracted result is nil")
	}
	if extractData, ok := resMap["data"]; err != nil || !ok {
		return "", utils.Errorf("extract error: %s", err)
	} else {
		if ret, ok := extractData.([]string); ok {
			return strings.Join(ret, ","), nil
		}
		return utils.InterfaceToString(extractData), nil
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
		PrefixRegexp:   utils.EscapeInvalidUTF8Byte([]byte(pre)),
		SuffixRegexp:   utils.EscapeInvalidUTF8Byte([]byte(suf)),
		SelectedRegexp: utils.EscapeInvalidUTF8Byte([]byte(matched)),
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
