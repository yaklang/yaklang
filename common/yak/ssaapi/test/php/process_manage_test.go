package php

import (
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
	"testing"
	"time"
)

func TestPHP_ProcessManage(t *testing.T) {
	var originAstCost time.Duration
	var cancelAstCost time.Duration

	fs := filesys.NewRelLocalFs(`./phpcode/badcase`)
	err := ssatest.CheckProfileWithFS(fs, t, func(p ssatest.ParseStage, prog ssaapi.Programs, start time.Time) error {
		if p == ssatest.OnlyMemory {
			originAstCost = time.Since(start)
		}
		return nil
	}, ssaapi.WithLanguage(ssaapi.PHP))
	require.NoError(t, err)
	m := ssaapi.NewSSAParseProcessManager()
	timer := time.NewTimer(originAstCost / 10)
	go func() {
		select {
		case <-timer.C:
			m.Stop()
		}
	}()
	err = ssatest.CheckProfileWithFS(fs, t, func(p ssatest.ParseStage, prog ssaapi.Programs, start time.Time) error {
		if p == ssatest.OnlyMemory {
			cancelAstCost = time.Since(start)
		}
		return nil
	}, ssaapi.WithLanguage(ssaapi.PHP), ssaapi.WithProcessManager(m))
	require.ErrorContains(t, err, `context canceled`)
	require.Greater(t, originAstCost, cancelAstCost*3)
	log.Info("origin ast cost: ", originAstCost)
	log.Info("Proactively stop ast cost: ", cancelAstCost)
	log.Info("error:", err)
}
