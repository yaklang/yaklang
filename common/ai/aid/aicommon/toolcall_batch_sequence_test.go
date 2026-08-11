package aicommon

import (
	"bytes"
	"context"
	"io"
	"sync"
	"testing"

	"github.com/segmentio/ksuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

func TestToolCaller_ParamTransactionReservedSeqIsConsumedOnce(t *testing.T) {
	var seqMu sync.Mutex
	var observedSeqs []int64
	cfg := NewTestConfig(
		context.Background(),
		WithID("param-seq-"+ksuid.New().String()),
		WithSequence(1200),
		WithAICallback(func(config AICallerConfigIf, request *AIRequest) (*AIResponse, error) {
			seqMu.Lock()
			observedSeqs = append(observedSeqs, request.GetSeqId())
			seqMu.Unlock()
			response := config.NewAIResponse()
			response.EmitOutputStream(bytes.NewBufferString(`{"@action":"call-tool","params":{"id":1}}`))
			response.Close()
			return response, nil
		}),
	)
	tool, err := aitool.New(
		"param_sequence_tool",
		aitool.WithIntegerParam("id", aitool.WithParam_Required(true)),
		aitool.WithSimpleCallback(func(aitool.InvokeParams, io.Writer, io.Writer) (any, error) {
			return "unused", nil
		}),
	)
	require.NoError(t, err)
	caller, err := NewToolCaller(
		context.Background(),
		WithToolCaller_AICallerConfig(cfg),
		WithToolCaller_AICaller(cfg),
		WithToolCaller_Emitter(cfg.GetEmitter()),
		WithToolCaller_Task(cfg.DefaultTask),
		WithToolCaller_CallToolID("param-seq-call"),
		WithToolCaller_ParamTransactionSeq(777),
		WithToolCaller_GenerateToolParamsBuilder(func(_ *aitool.Tool, _ string) (string, error) {
			return "generate params", nil
		}),
	)
	require.NoError(t, err)

	_, err = caller.generateParams(tool, func(any) {})
	require.NoError(t, err)
	_, err = caller.generateParams(tool, func(any) {})
	require.NoError(t, err)

	seqMu.Lock()
	require.Equal(t, []int64{777, 0}, observedSeqs,
		"the recursive request must not reuse the reserved outer sequence")
	seqMu.Unlock()
	require.Equal(t, int64(1200), cfg.SeqIdProvider.CurrentID(),
		"the zero request sequence must be replaced by a fresh config sequence")
}
