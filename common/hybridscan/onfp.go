package hybridscan

import (
	"fmt"
	"github.com/pkg/errors"
	"github.com/yaklang/yaklang/common/fp"
)

func mRtoStr(r *fp.MatchResult) string {
	if r == nil {
		return ""
	}
	return fmt.Sprintf("%v://%v:%v", r.GetProto(), r.Target, r.Port)
}

func (c *HyperScanCenter) onMatcherResult(matcherResult *fp.MatchResult, err error) {
	if matcherResult != nil {
		_, ok := c.config.FingerprintMatchResultTTLCache.Get(mRtoStr(matcherResult))
		if ok {
			return
		}
		c.config.FingerprintMatchResultTTLCache.Set(mRtoStr(matcherResult), matcherResult)
	}

	for _, h := range c.fpResultHandlers {
		h(matcherResult, err)
	}
}

func (c *HyperScanCenter) RegisterMatcherResultHandler(tag string, h fp.PoolCallback) error {
	c.fpResultHandlerMutex.Lock()
	defer c.fpResultHandlerMutex.Unlock()

	if _, ok := c.fpResultHandlers[tag]; ok {
		return errors.Errorf("existed handler: %s", tag)
	}
	c.fpResultHandlers[tag] = h
	return nil
}

func (c *HyperScanCenter) UnregisterMatcherResultHandler(tag string) {
	c.fpResultHandlerMutex.Lock()
	defer c.fpResultHandlerMutex.Unlock()

	delete(c.fpResultHandlers, tag)
}
