package aispec

import (
	"bytes"
	"io"
	"net/http/httputil"

	"github.com/yaklang/yaklang/common/utils"
)

func readAIHTTPError(status int, body io.Reader, chunked bool, captured *bytes.Buffer) error {
	if chunked {
		body = httputil.NewChunkedReader(body)
	}
	// Keep diagnostics bounded while preserving the provider's error message.
	_, readErr := io.Copy(captured, io.LimitReader(body, 4096))
	err := utils.Errorf("AI provider returned HTTP %d: %s", status, captured.String())
	if readErr != nil {
		err = utils.Wrapf(readErr, "%v", err)
	}
	if evidence := DetectPromptOverflow(status, captured.Bytes(), ""); evidence != nil {
		return &PromptOverflowError{PromptOverflowEvidence: *evidence, Cause: err}
	}
	return err
}
