package ssaapi

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

func TestCompileBatchPipelineWaitsForCleanup(t *testing.T) {
	var pipeline compileBatchPipeline
	entered := make(chan struct{})
	release := make(chan struct{})
	cleaned := false
	pipeline.Start(func() error {
		close(entered)
		<-release
		cleaned = true
		return nil
	})
	<-entered
	// The producer gets control back while scan/persist is still running.
	// A second handoff cannot accumulate another outstanding IR batch.
	require.Panics(t, func() { pipeline.Start(func() error { return nil }) })
	waiting := make(chan error, 1)
	go func() { waiting <- pipeline.Wait() }()
	select {
	case <-waiting:
		t.Fatal("batch completed before its cleanup")
	case <-time.After(10 * time.Millisecond):
	}
	close(release)
	require.NoError(t, <-waiting)
	require.True(t, cleaned)
	require.NoError(t, pipeline.Wait())
}

func TestCompileBatchPipelinePropagatesFailure(t *testing.T) {
	var pipeline compileBatchPipeline
	want := errors.New("persist failed")
	pipeline.Start(func() error { return want })
	require.ErrorIs(t, pipeline.Wait(), want)
	pipeline.Start(func() error { panic("bad batch") })
	require.ErrorContains(t, pipeline.Wait(), "bad batch")
}

type parseSignalBuilder struct {
	ssa.Builder
	parsed chan struct{}
}

func (b *parseSignalBuilder) ParseAST(source string, cache *ssa.AntlrCache) (ssa.FrontAST, error) {
	ast, err := b.Builder.ParseAST(source, cache)
	b.parsed <- struct{}{}
	return ast, err
}

func TestCompileBatchPipelineParsesWhilePreviousBatchScans(t *testing.T) {
	t.Setenv(compileUnitBatchMinFilesEnv, "1")
	t.Setenv(compileUnitBatchMinBytesEnv, "1")
	vf := filesys.NewVirtualFs()
	vf.AddFile("a/A.java", `package a; class A { void f(String s) throws Exception { Runtime.getRuntime().exec(s); } }`)
	vf.AddFile("b/B.java", `package b; class B { void f(String s) throws Exception { Runtime.getRuntime().exec(s); } }`)
	parsed := make(chan struct{}, 2)
	callbacks := 0
	overlapped := false
	cfg, err := DefaultConfig(WithFileSystem(vf), WithLanguage(ssaconfig.JAVA),
		WithProgramName(uuid.NewString()), WithMemory(),
		WithStructRuleRaw(`desc(mode: "struct", language: "java")
Runtime.getRuntime().exec(* as $cmd) as $call
alert $call`), WithStructRuleCallback(func(*schema.SSARisk) {
			callbacks++
			if callbacks != 1 {
				return
			}
			<-parsed // current batch was already parsed before its scan
			select {
			case <-parsed:
				overlapped = true
			case <-time.After(5 * time.Second):
				t.Error("next batch parsing waited for the previous batch's scan")
			}
		}))
	require.NoError(t, err)
	cfg.LanguageBuilder = &parseSignalBuilder{Builder: cfg.LanguageBuilder, parsed: parsed}
	_, err = cfg.parseProjectWithFSUnits(vf, func(float64, string, ...any) {})
	require.NoError(t, err)
	require.Equal(t, 2, callbacks)
	require.True(t, overlapped)
}
