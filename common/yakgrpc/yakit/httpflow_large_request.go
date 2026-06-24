package yakit

import (
	"fmt"
	"time"

	"github.com/segmentio/ksuid"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
)

// maxHTTPFlowRequestBodyInDBBytes aligns with History list preview (200KB).
const maxHTTPFlowRequestBodyInDBBytes = 200 * 1024

const storedHTTPFlowLargeRequestTruncateNotice = "[[request too large(%s), truncated]] use GetHTTPFlowBodyById(IsRequest=true) for full body"

type largeRequestSpillResult struct {
	StoredPacket    []byte
	IsTooLarge      bool
	HeaderFile      string
	BodyFile        string
	OriginalBodyLen int
}

func spillLargeHTTPFlowRequestIfNeeded(packet []byte) (largeRequestSpillResult, error) {
	res := largeRequestSpillResult{StoredPacket: packet}
	if len(packet) == 0 {
		return res, nil
	}

	header, body := lowhttp.SplitHTTPHeadersAndBodyFromPacket(packet)
	res.OriginalBodyLen = len(body)
	if len(body) <= maxHTTPFlowRequestBodyInDBBytes {
		return res, nil
	}

	uid := ksuid.New().String()
	suffix := fmt.Sprintf(`%v_%v`, time.Now().Format(utils.DatetimePretty()), uid)

	headerFP, err := utils.OpenTempFile(fmt.Sprintf("large-request-header-%v.txt", suffix))
	if err != nil {
		return res, err
	}
	if _, err := headerFP.Write([]byte(header)); err != nil {
		headerFP.Close()
		return res, err
	}
	headerPath := headerFP.Name()
	headerFP.Close()

	bodyFP, err := utils.OpenTempFile(fmt.Sprintf("large-request-body-%v.txt", suffix))
	if err != nil {
		return res, err
	}
	if _, err := bodyFP.Write(body); err != nil {
		bodyFP.Close()
		return res, err
	}
	bodyPath := bodyFP.Name()
	bodyFP.Close()

	notice := []byte(fmt.Sprintf(storedHTTPFlowLargeRequestTruncateNotice, utils.ByteSize(uint64(len(body)))))
	stored := lowhttp.ReplaceHTTPPacketBody([]byte(header), notice, false)

	res.StoredPacket = stored
	res.IsTooLarge = true
	res.HeaderFile = headerPath
	res.BodyFile = bodyPath
	return res, nil
}

func CreateHTTPFlowWithTooLargeRequestHeaderFile(fp string) CreateHTTPFlowOptions {
	return func(c *CreateHTTPFlowConfig) {
		c.tooLargeRequestHeaderFile = fp
	}
}

func CreateHTTPFlowWithTooLargeRequestBodyFile(fp string) CreateHTTPFlowOptions {
	return func(c *CreateHTTPFlowConfig) {
		c.tooLargeRequestBodyFile = fp
	}
}

func requestBodyLengthFromPacket(packet []byte) int {
	if len(packet) == 0 {
		return 0
	}
	_, body := lowhttp.SplitHTTPHeadersAndBodyFromPacket(packet)
	return len(body)
}
