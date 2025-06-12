package aid

import (
	"context"
	"github.com/yaklang/yaklang/common/log"
)

func (c *Config) startHotpatchLoop(ctx context.Context) {
	c.startHotpatchOnce.Do(func() {
		if c.hotpatchOptionChan == nil {
			return
		}
		go func() {
			for {
				select {
				case <-ctx.Done():
				case hotpatchOption := <-c.hotpatchOptionChan.OutputChannel():
					if hotpatchOption == nil {
						log.Errorf("hotpatch option is nil, will return")
						return
					}
					err := hotpatchOption(c)
					if err != nil {
						log.Errorf("hotpatch option err: %v", err)
					}
					c.EmitCurrentConfigInfo()
				}
			}
		}()
	})
}
