package reify

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"runtime"
	"sort"
	"sync"

	"github.com/yaklang/yaklang/common/yak/reify/core"
	"github.com/yaklang/yaklang/common/yak/reify/core/asm2lowir"
	"github.com/yaklang/yaklang/common/yak/reify/core/lowir2ssa"
	"github.com/yaklang/yaklang/common/yak/reify/core/lowirtext"
	"github.com/yaklang/yaklang/common/yak/reify/core/ssa2source"
	ssapass "github.com/yaklang/yaklang/common/yak/reify/core/ssa2source/pass"
	"github.com/yaklang/yaklang/common/yak/reify/loader"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssa/ssairtext"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

const (
	defaultStreamBatchSize     = 64
	defaultStreamBatchMaxBytes = 128 << 10
	defaultSSABatchSize        = 64
	defaultSSABatchMaxBytes    = 32 << 10
	defaultSaveBatchSize       = 256
	defaultSaveBatchMaxBytes   = 256 << 10
	heavyBatchBytesThreshold   = 64 << 10
	defaultMaxWorkers          = 8
	defaultSSAMaxWorkers       = 8
	maxOrderedWindow           = 32
	streamBufSize              = 256 * 1024
	maxPooledBatchBufCap       = 4 << 20
	largeProjectThreshold      = 16 << 20
)

type funcBatch struct {
	start     int
	end       int
	codeBytes uint64
}

func buildBatches(boundaries []core.FunctionBoundary, explicitBatchSize, defaultMaxFuncs int, defaultMaxBytes uint64) []funcBatch {
	if len(boundaries) == 0 {
		return nil
	}
	if explicitBatchSize > 0 {
		batches := make([]funcBatch, 0, (len(boundaries)+explicitBatchSize-1)/explicitBatchSize)
		for start := 0; start < len(boundaries); start += explicitBatchSize {
			end := start + explicitBatchSize
			if end > len(boundaries) {
				end = len(boundaries)
			}
			var sum uint64
			for i := start; i < end; i++ {
				sum += boundaries[i].Size
			}
			batches = append(batches, funcBatch{start: start, end: end, codeBytes: sum})
		}
		return batches
	}
	if defaultMaxFuncs < 1 {
		defaultMaxFuncs = 1
	}
	batches := make([]funcBatch, 0, (len(boundaries)+defaultMaxFuncs-1)/defaultMaxFuncs)
	for i := 0; i < len(boundaries); {
		start := i
		var sum uint64
		for i < len(boundaries) && (i-start) < defaultMaxFuncs {
			sz := boundaries[i].Size
			if i > start && defaultMaxBytes > 0 && sum+sz > defaultMaxBytes {
				break
			}
			sum += sz
			i++
		}
		batches = append(batches, funcBatch{start: start, end: i, codeBytes: sum})
	}
	return batches
}

var batchBufPool = sync.Pool{
	New: func() any {
		return new(bytes.Buffer)
	},
}

func acquireBatchBuf() *bytes.Buffer {
	buf := batchBufPool.Get().(*bytes.Buffer)
	buf.Reset()
	return buf
}

func releaseBatchBuf(buf *bytes.Buffer) {
	if buf == nil || buf.Cap() > maxPooledBatchBufCap {
		return
	}
	buf.Reset()
	batchBufPool.Put(buf)
}

// StreamHeader carries metadata available before LiftStream emits representation bytes.
type StreamHeader struct {
	Mode          Mode
	Target        core.Target
	Parser        *core.ParserInfo
	Region        core.Region
	BoundaryStats *core.BoundaryStats
	Language      *core.LanguageReport
}

// StreamResult summarizes a completed streaming lift.
type StreamResult struct {
	Mode          Mode
	Target        core.Target
	Parser        *core.ParserInfo
	Region        core.Region
	Instructions  int
	Diagnostics   []core.Diagnostic
	BoundaryStats *core.BoundaryStats
	Language      *core.LanguageReport
	Functions     int
	Blocks        int
	Values        int
}

// SaveResult summarizes a chunked lift-and-persist operation into ssadb.
type SaveResult struct {
	Parser        *core.ParserInfo
	Region        core.Region
	Functions     int
	Blocks        int
	Instructions  int
	Values        int
	Diagnostics   []core.Diagnostic
	BoundaryStats *core.BoundaryStats
	ProgramName   string
}

// LiftStream lifts path in bounded-memory batches and writes the requested
// representation directly into w without buffering the full program or text.
func LiftStream(ctx context.Context, path string, w io.Writer, options ...Option) (*StreamResult, error) {
	if w == nil {
		return nil, fmt.Errorf("lift stream: nil writer")
	}
	settings := config(options)
	if !settings.Mode.Valid() || settings.Mode == ModeAll {
		return nil, fmt.Errorf("lift stream: unsupported mode %q", settings.Mode)
	}
	if settings.Size != "" && settings.Address == "" {
		return nil, fmt.Errorf("lift: --size requires --address")
	}
	if settings.Mode == ModeSSAIR || settings.Mode == ModeSource {
		if err := validateSSAPasses(settings.SSAPasses); err != nil {
			return nil, err
		}
	}
	format, err := resolveFormat(settings.Format)
	if err != nil {
		return nil, err
	}
	artifact, err := loader.Default.OpenFileAs(ctx, path, format)
	if err != nil {
		return nil, err
	}
	defer func() { _ = artifact.Close() }()

	asmOptions, err := buildASMOptions(artifact, settings)
	if err != nil {
		return nil, err
	}
	boundaries, stats, err := asm2lowir.Discover(artifact, asmOptions...)
	if err != nil {
		return nil, err
	}
	if len(boundaries) == 0 {
		return nil, fmt.Errorf("asm2lowir: no function boundaries")
	}

	region := displayRegion(artifact, settings)
	if settings.Mode != ModeASM {
		if r := boundariesRegion(boundaries); r.Size > 0 {
			region = r
		}
	}

	parserInfo := ParserInfo(artifact)
	res := &StreamResult{
		Mode:          settings.Mode,
		Target:        artifact.Target,
		Parser:        parserInfo,
		Region:        region,
		BoundaryStats: &stats,
		Language:      artifact.Language,
	}
	if settings.Mode != ModeASM {
		res.Diagnostics = append(res.Diagnostics, languageLimitDiagnostics(artifact)...)
	}

	header := StreamHeader{
		Mode:          settings.Mode,
		Target:        artifact.Target,
		Parser:        parserInfo,
		Region:        region,
		BoundaryStats: &stats,
		Language:      artifact.Language,
	}
	bw := bufio.NewWriterSize(w, streamBufSize)
	headerWritten := false
	emitHeader := func() error {
		if headerWritten {
			return nil
		}
		headerWritten = true
		if settings.BeforeStream != nil {
			return settings.BeforeStream(bw, header)
		}
		return nil
	}

	workers := resolveWorkers(settings.Workers, defaultMaxWorkers)
	if settings.Mode == ModeSSAIR || settings.Mode == ModeSource {
		workers = resolveWorkers(settings.Workers, defaultSSAMaxWorkers)
	}

	switch settings.Mode {
	case ModeASM:
		if err := streamASM(ctx, bw, emitHeader, artifact, boundaries, asmOptions, workers, settings.BatchSize, res); err != nil {
			return nil, err
		}
	case ModeLowIR:
		if err := streamLowIR(ctx, bw, emitHeader, artifact, boundaries, asmOptions, workers, settings.BatchSize, res); err != nil {
			return nil, err
		}
	case ModeSSAIR, ModeSource:
		if err := streamSSAOrSource(ctx, bw, emitHeader, artifact, boundaries, asmOptions, settings, workers, res); err != nil {
			return nil, err
		}
	}

	if err := bw.Flush(); err != nil {
		return nil, err
	}
	if len(boundaries) > 1 {
		res.Diagnostics = FilterCrossFunctionDiagnostics(res.Diagnostics)
	}
	return res, nil
}

// Save lifts path in bounded-memory batches using YakSSA compile units, saving
// each finished batch into stagedName in ssadb and releasing its bodies before
// starting the next batch.
func Save(ctx context.Context, path, stagedName string, options ...Option) (*SaveResult, error) {
	if stagedName == "" {
		return nil, fmt.Errorf("save: empty staged program name")
	}
	settings := config(options)
	if settings.Size != "" && settings.Address == "" {
		return nil, fmt.Errorf("save: --size requires --address")
	}
	if err := validateSSAPasses(settings.SSAPasses); err != nil {
		return nil, err
	}
	format, err := resolveFormat(settings.Format)
	if err != nil {
		return nil, err
	}
	artifact, err := loader.Default.OpenFileAs(ctx, path, format)
	if err != nil {
		return nil, err
	}
	defer func() { _ = artifact.Close() }()

	asmOptions, err := buildASMOptions(artifact, settings)
	if err != nil {
		return nil, err
	}
	boundaries, stats, err := asm2lowir.Discover(artifact, asmOptions...)
	if err != nil {
		return nil, err
	}
	if len(boundaries) == 0 {
		return nil, fmt.Errorf("asm2lowir: no function boundaries")
	}

	region := displayRegion(artifact, settings)
	if r := boundariesRegion(boundaries); r.Size > 0 {
		region = r
	}

	batches := buildBatches(boundaries, settings.BatchSize, defaultSaveBatchSize, defaultSaveBatchMaxBytes)
	workers := resolveWorkers(settings.Workers, defaultSSAMaxWorkers)
	totalBatches := len(batches)
	stringAt := stringAtFunc(artifact)
	baseConvertOpts := buildBaseConvertOptions(artifact, settings, ModeSSAIR)

	var summary *ssapass.ProgramSummary
	if totalBatches > 1 && ssapass.HasInterproceduralPass(settings.SSAPasses) {
		var err error
		summary, err = buildProgramSummary(ctx, artifact, boundaries, batches, asmOptions, baseConvertOpts, workers)
		if err != nil {
			return nil, err
		}
	}

	cfg, err := ssaconfig.New(
		ssaconfig.ModeSSACompile,
		ssaconfig.WithSetProgramName(stagedName),
	)
	if err != nil {
		return nil, fmt.Errorf("save: invalid program config: %w", err)
	}
	cfg.SetCompileUnitSplit(true)
	projectBytes := artifact.Origin.Size
	if len(boundaries) > defaultSaveBatchSize && projectBytes < largeProjectThreshold {
		projectBytes = largeProjectThreshold
	}
	if projectBytes > 0 {
		cfg.SetCompileProjectBytes(projectBytes)
	}
	program := ssa.NewProgram(cfg, ssa.ProgramCacheDBWrite, ssa.Application, nil, "", 0)
	program.Language = ssaconfig.C

	saved := false
	defer func() {
		if !saved && program != nil && program.Cache != nil && !program.Cache.IsClosed() {
			program.Cache.CloseWithoutSave()
		}
	}()

	convertOptions := append([]lowir2ssa.Option(nil), baseConvertOpts...)
	convertOptions = append(convertOptions, lowir2ssa.WithProgramName(stagedName), lowir2ssa.WithProgramCache(ssa.ProgramCacheDBWrite))

	res := &SaveResult{
		Parser:        ParserInfo(artifact),
		Region:        region,
		BoundaryStats: &stats,
		ProgramName:   stagedName,
		Diagnostics:   languageLimitDiagnostics(artifact),
	}

	var prefetched <-chan savePrefetchItem
	var stopPrefetch func()
	if totalBatches > 1 && workers > 1 {
		prefetched, stopPrefetch = startSavePrefetch(ctx, artifact, boundaries, batches, asmOptions, workers)
		defer stopPrefetch()
	}

	nextID := core.FunctionID(1)
	for b := 0; b < totalBatches; b++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		batch := batches[b]
		unitKey := fmt.Sprintf("reify-batch-%d", b)
		program.BeginCompileUnit(unitKey)
		program.Cache.DisableInstructionSpill()

		var parsed *asm2lowir.Result
		var parseErr error
		if prefetched != nil {
			select {
			case item, ok := <-prefetched:
				if !ok {
					parseErr = ctx.Err()
					if parseErr == nil {
						parseErr = fmt.Errorf("save: prefetch closed unexpectedly at batch %d", b)
					}
				} else {
					parsed, parseErr = item.parsed, item.err
				}
			case <-ctx.Done():
				parseErr = ctx.Err()
			}
			if parseErr == nil && parsed != nil && parsed.IR != nil && nextID != core.FunctionID(batch.start+1) {
				// Keep sequential FunctionID assignment contiguous if an earlier batch produced 0 functions.
				for idx := range parsed.IR.Functions {
					parsed.IR.Functions[idx].ID = nextID + core.FunctionID(idx)
				}
			}
		} else {
			parsed, parseErr = asm2lowir.ParseBatch(ctx, artifact, boundaries, batch.start, batch.end, nextID, asmOptions...)
		}
		if parseErr != nil {
			program.Cache.EnableInstructionSpill()
			program.EndCompileUnit()
			return nil, parseErr
		}
		res.Diagnostics = append(res.Diagnostics, parsed.Diagnostics...)
		if len(parsed.IR.Functions) == 0 {
			program.Cache.EnableInstructionSpill()
			program.EndCompileUnit()
			continue
		}
		res.Functions += len(parsed.IR.Functions)
		res.Blocks += countBlocks(parsed.IR)
		res.Instructions += countInstructions(parsed.IR)
		res.Values += countValues(parsed.IR)
		nextID += core.FunctionID(len(parsed.IR.Functions))

		funcs, convDiags, err := lowir2ssa.ConvertInto(ctx, program, parsed.IR, convertOptions...)
		parsed = nil
		if err != nil {
			program.Cache.EnableInstructionSpill()
			program.EndCompileUnit()
			return nil, err
		}
		res.Diagnostics = append(res.Diagnostics, convDiags...)

		if totalBatches == 1 && summary == nil {
			if err := applySSAPasses(program, artifact, settings.SSAPasses); err != nil {
				program.Cache.EnableInstructionSpill()
				program.EndCompileUnit()
				return nil, err
			}
		} else {
			activeSummary := summary
			if activeSummary == nil {
				activeSummary = ssapass.NewProgramSummary()
				activeSummary.Finalize()
			}
			if err := activeSummary.ApplyFunctions(funcs, stringAt, settings.SSAPasses...); err != nil {
				program.Cache.EnableInstructionSpill()
				program.EndCompileUnit()
				return nil, err
			}
		}
		ssa2source.AnnotateFunctions(funcs, artifact)
		program.FlushFunctions(funcs)
		program.Cache.EnableInstructionSpill()
		program.EndCompileUnit()

		program.DetachFinishedFunctionBodyScopes()
		program.Cache.FlushCompileUnit(unitKey)
		program.Cache.FlushInstructionSaver()
		program.ReleasePersistedOrdinaryValues()
		program.Cache.FlushAuxSavers()
		program.Cache.FlushInstructionSaver()
		program.ReleasePersistedOrdinaryValues()
		program.ReleaseSettledIndexes()
		program.CheckMemoryPressure(b+1, totalBatches)
	}

	if res.Functions == 0 {
		return nil, fmt.Errorf("no instructions recovered from function boundaries")
	}
	if len(boundaries) > 1 {
		res.Diagnostics = FilterCrossFunctionDiagnostics(res.Diagnostics)
	}
	if err := program.SaveFlushedToDatabase(); err != nil {
		return nil, err
	}
	saved = true
	return res, nil
}

func streamASM(ctx context.Context, w io.Writer, emitHeader func() error, artifact *core.Artifact, boundaries []core.FunctionBoundary, asmOptions []asm2lowir.Option, workers, explicitBatchSize int, res *StreamResult) error {
	batches := buildBatches(boundaries, explicitBatchSize, defaultStreamBatchSize, defaultStreamBatchMaxBytes)
	type asmChunk struct {
		text  string
		insts int
		diags []core.Diagnostic
	}
	err := runOrderedBatches(ctx, batches, workers, maxOrderedWindow, 0,
		func(batchCtx context.Context, b int) (asmChunk, error) {
			batch := batches[b]
			dis, err := asm2lowir.DisassembleBatch(batchCtx, artifact, boundaries, batch.start, batch.end, asmOptions...)
			if err != nil {
				return asmChunk{}, err
			}
			return asmChunk{text: dis.Text, insts: dis.Instructions, diags: dis.Diagnostics}, nil
		},
		func(_ int, chunk asmChunk) error {
			res.Instructions += chunk.insts
			res.Diagnostics = append(res.Diagnostics, chunk.diags...)
			if chunk.insts == 0 && res.Instructions == 0 && len(batches) == 1 {
				return nil
			}
			if err := emitHeader(); err != nil {
				return err
			}
			_, err := io.WriteString(w, chunk.text)
			return err
		},
	)
	if err != nil {
		return err
	}
	if res.Instructions == 0 {
		return fmt.Errorf("no instructions recovered from function boundaries")
	}
	return nil
}

func streamLowIR(ctx context.Context, w io.Writer, emitHeader func() error, artifact *core.Artifact, boundaries []core.FunctionBoundary, asmOptions []asm2lowir.Option, workers, explicitBatchSize int, res *StreamResult) error {
	batches := buildBatches(boundaries, explicitBatchSize, defaultStreamBatchSize, defaultStreamBatchMaxBytes)
	type lowirChunk struct {
		header string
		body   *bytes.Buffer
		funcs  int
		blocks int
		insts  int
		values int
		diags  []core.Diagnostic
	}
	headerPrinted := false
	err := runOrderedBatches(ctx, batches, workers, maxOrderedWindow, 0,
		func(batchCtx context.Context, b int) (*lowirChunk, error) {
			batch := batches[b]
			parsed, err := asm2lowir.ParseBatch(batchCtx, artifact, boundaries, batch.start, batch.end, core.FunctionID(batch.start+1), asmOptions...)
			if err != nil {
				return nil, err
			}
			chunk := &lowirChunk{diags: parsed.Diagnostics}
			if parsed.IR == nil || len(parsed.IR.Functions) == 0 {
				return chunk, nil
			}
			chunk.funcs = len(parsed.IR.Functions)
			chunk.blocks = countBlocks(parsed.IR)
			chunk.insts = countInstructions(parsed.IR)
			chunk.values = countValues(parsed.IR)

			var hdr bytes.Buffer
			if err := lowirtext.FormatHeaderTo(&hdr, parsed.IR); err != nil {
				return nil, err
			}
			chunk.header = hdr.String()

			buf := acquireBatchBuf()
			for _, fn := range parsed.IR.Functions {
				if err := lowirtext.FormatFunctionTo(buf, fn); err != nil {
					releaseBatchBuf(buf)
					return nil, err
				}
			}
			chunk.body = buf
			return chunk, nil
		},
		func(_ int, chunk *lowirChunk) error {
			if chunk.body != nil {
				defer releaseBatchBuf(chunk.body)
			}
			res.Diagnostics = append(res.Diagnostics, chunk.diags...)
			if chunk.funcs == 0 {
				return nil
			}
			res.Functions += chunk.funcs
			res.Blocks += chunk.blocks
			res.Instructions += chunk.insts
			res.Values += chunk.values
			if !headerPrinted {
				if err := emitHeader(); err != nil {
					return err
				}
				if _, err := io.WriteString(w, chunk.header); err != nil {
					return err
				}
				headerPrinted = true
			}
			if chunk.body != nil && chunk.body.Len() > 0 {
				_, err := chunk.body.WriteTo(w)
				return err
			}
			return nil
		},
	)
	if err != nil {
		return err
	}
	if res.Functions == 0 {
		return fmt.Errorf("no instructions recovered from function boundaries")
	}
	return nil
}

func streamSSAOrSource(ctx context.Context, w io.Writer, emitHeader func() error, artifact *core.Artifact, boundaries []core.FunctionBoundary, asmOptions []asm2lowir.Option, settings Config, workers int, res *StreamResult) error {
	batches := buildBatches(boundaries, settings.BatchSize, defaultSSABatchSize, defaultSSABatchMaxBytes)
	totalBatches := len(batches)
	stringAt := stringAtFunc(artifact)
	baseConvertOpts := buildBaseConvertOptions(artifact, settings, settings.Mode)

	// Fast path when everything fits in one batch: skip Pass 1 completely.
	if totalBatches == 1 {
		parsed, err := asm2lowir.ParseBatch(ctx, artifact, boundaries, 0, len(boundaries), 1, asmOptions...)
		if err != nil {
			return err
		}
		res.Diagnostics = append(res.Diagnostics, parsed.Diagnostics...)
		if len(parsed.IR.Functions) == 0 {
			return fmt.Errorf("no instructions recovered from function boundaries")
		}
		sourceFuncs := countSourceFunctions(parsed.IR)
		res.Blocks = countBlocks(parsed.IR)
		res.Instructions = countInstructions(parsed.IR)
		res.Values = countValues(parsed.IR)
		if settings.Mode == ModeSource {
			res.Functions = sourceFuncs
		} else {
			res.Functions = len(parsed.IR.Functions)
		}
		conv, err := lowir2ssa.Convert(ctx, parsed.IR, baseConvertOpts...)
		parsed = nil
		if err != nil {
			return err
		}
		res.Diagnostics = append(res.Diagnostics, conv.Diagnostics...)
		if err := applySSAPasses(conv.Program, artifact, settings.SSAPasses); err != nil {
			return err
		}
		ssa2source.AnnotateArtifact(conv.Program, artifact)
		if err := emitHeader(); err != nil {
			return err
		}
		if settings.Mode == ModeSSAIR {
			return ssairtext.FormatProgramTo(w, conv.Program)
		}
		return ssa2source.RenderTo(ctx, w, conv.Program)
	}

	var summary *ssapass.ProgramSummary
	if ssapass.HasInterproceduralPass(settings.SSAPasses) {
		var err error
		summary, err = buildProgramSummary(ctx, artifact, boundaries, batches, asmOptions, baseConvertOpts, workers)
		if err != nil {
			return err
		}
	}

	if err := emitHeader(); err != nil {
		return err
	}
	seenExterns := make(map[string]bool)
	if settings.Mode == ModeSSAIR && summary != nil && len(summary.Externs) > 0 {
		externs := make([]string, 0, len(summary.Externs))
		for name := range summary.Externs {
			externs = append(externs, name)
			seenExterns[name] = true
		}
		sort.Strings(externs)
		for _, name := range externs {
			if _, err := fmt.Fprintf(w, "extern %s\n", name); err != nil {
				return err
			}
		}
	}

	type renderedBatch struct {
		body        *bytes.Buffer
		externs     []string
		funcs       int
		srcFuncs    int
		blocks      int
		insts       int
		values      int
		diagnostics []core.Diagnostic
	}

	err := runOrderedBatches(ctx, batches, workers, maxOrderedWindow, heavyBatchBytesThreshold,
		func(batchCtx context.Context, b int) (*renderedBatch, error) {
			batch := batches[b]
			parsed, err := asm2lowir.ParseBatch(batchCtx, artifact, boundaries, batch.start, batch.end, core.FunctionID(batch.start+1), asmOptions...)
			if err != nil {
				return nil, err
			}
			chunk := &renderedBatch{diagnostics: parsed.Diagnostics}
			if len(parsed.IR.Functions) == 0 {
				return chunk, nil
			}
			chunk.funcs = len(parsed.IR.Functions)
			chunk.srcFuncs = countSourceFunctions(parsed.IR)
			chunk.blocks = countBlocks(parsed.IR)
			chunk.insts = countInstructions(parsed.IR)
			chunk.values = countValues(parsed.IR)

			conv, err := lowir2ssa.Convert(batchCtx, parsed.IR, baseConvertOpts...)
			parsed = nil
			if err != nil {
				return nil, err
			}
			chunk.diagnostics = append(chunk.diagnostics, conv.Diagnostics...)

			var funcs []*ssa.Function
			var batchExterns []string
			conv.Program.Funcs.ForEach(func(name string, fn *ssa.Function) bool {
				if fn == nil {
					return true
				}
				if fn.IsExtern() {
					if summary == nil && settings.Mode == ModeSSAIR {
						batchExterns = append(batchExterns, name)
					}
					return true
				}
				funcs = append(funcs, fn)
				return true
			})
			chunk.externs = batchExterns

			activeSummary := summary
			if activeSummary == nil {
				activeSummary = ssapass.NewProgramSummary()
				activeSummary.Finalize()
			}
			if err := activeSummary.ApplyFunctions(funcs, stringAt, settings.SSAPasses...); err != nil {
				return nil, err
			}
			ssa2source.AnnotateFunctions(funcs, artifact)

			buf := acquireBatchBuf()
			for _, fn := range funcs {
				if settings.Mode == ModeSSAIR {
					if err := ssairtext.FormatTo(buf, fn); err != nil {
						releaseBatchBuf(buf)
						return nil, err
					}
				} else {
					if err := ssa2source.RenderFunctionTo(buf, fn); err != nil {
						releaseBatchBuf(buf)
						return nil, err
					}
				}
			}
			chunk.body = buf
			return chunk, nil
		},
		func(_ int, chunk *renderedBatch) error {
			if chunk.body != nil {
				defer releaseBatchBuf(chunk.body)
			}
			res.Diagnostics = append(res.Diagnostics, chunk.diagnostics...)
			if settings.Mode == ModeSource {
				res.Functions += chunk.srcFuncs
			} else {
				res.Functions += chunk.funcs
			}
			res.Blocks += chunk.blocks
			res.Instructions += chunk.insts
			res.Values += chunk.values
			if len(chunk.externs) > 0 {
				sort.Strings(chunk.externs)
				for _, name := range chunk.externs {
					if seenExterns[name] {
						continue
					}
					seenExterns[name] = true
					if _, err := fmt.Fprintf(w, "extern %s\n", name); err != nil {
						return err
					}
				}
			}
			if chunk.body != nil && chunk.body.Len() > 0 {
				_, err := chunk.body.WriteTo(w)
				return err
			}
			return nil
		},
	)
	if err != nil {
		return err
	}
	if res.Functions == 0 && res.Blocks == 0 {
		return fmt.Errorf("no instructions recovered from function boundaries")
	}
	return nil
}

type savePrefetchItem struct {
	parsed *asm2lowir.Result
	err    error
}

func startSavePrefetch(ctx context.Context, artifact *core.Artifact, boundaries []core.FunctionBoundary, batches []funcBatch, asmOptions []asm2lowir.Option, workers int) (<-chan savePrefetchItem, func()) {
	pCtx, cancel := context.WithCancel(ctx)
	ready := make(chan savePrefetchItem)

	prefetchWorkers := workers
	if prefetchWorkers > 2 {
		prefetchWorkers = 2
	}
	windowSem := make(chan struct{}, prefetchWorkers)
	slots := make([]chan savePrefetchItem, len(batches))
	for i := range slots {
		slots[i] = make(chan savePrefetchItem, 1)
	}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for b, batch := range batches {
			select {
			case windowSem <- struct{}{}:
			case <-pCtx.Done():
				return
			}
			wg.Add(1)
			go func(idx int, bt funcBatch) {
				defer wg.Done()
				parsed, err := asm2lowir.ParseBatch(pCtx, artifact, boundaries, bt.start, bt.end, core.FunctionID(bt.start+1), asmOptions...)
				slots[idx] <- savePrefetchItem{parsed: parsed, err: err}
			}(b, batch)
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		defer close(ready)
		for b := range batches {
			var item savePrefetchItem
			select {
			case item = <-slots[b]:
			case <-pCtx.Done():
				return
			}
			select {
			case ready <- item:
				<-windowSem
			case <-pCtx.Done():
				<-windowSem
				return
			}
			if item.err != nil {
				return
			}
		}
	}()

	stop := func() {
		cancel()
		wg.Wait()
	}
	return ready, stop
}

func buildProgramSummary(ctx context.Context, artifact *core.Artifact, boundaries []core.FunctionBoundary, batches []funcBatch, asmOptions []asm2lowir.Option, baseConvertOpts []lowir2ssa.Option, workers int) (*ssapass.ProgramSummary, error) {
	if len(batches) == 0 {
		summary := ssapass.NewProgramSummary()
		summary.Finalize()
		return summary, nil
	}
	if workers < 1 {
		workers = 1
	}
	if workers > len(batches) {
		workers = len(batches)
	}

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	jobs := make(chan int)
	heavyLimit := workers
	if heavyLimit > 2 {
		heavyLimit = 2
	}
	var heavySem chan struct{}
	if heavyLimit < workers {
		heavySem = make(chan struct{}, heavyLimit)
	}

	summary := ssapass.NewProgramSummary()
	var mu sync.Mutex
	var errOnce sync.Once
	var firstErr error
	setErr := func(err error) {
		errOnce.Do(func() {
			firstErr = err
			cancel()
		})
	}

	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for b := range jobs {
				if runCtx.Err() != nil {
					continue
				}
				batch := batches[b]
				isHeavy := heavySem != nil && batch.codeBytes >= heavyBatchBytesThreshold
				if isHeavy {
					select {
					case heavySem <- struct{}{}:
					case <-runCtx.Done():
						continue
					}
				}
				parsed, err := asm2lowir.ParseBatch(runCtx, artifact, boundaries, batch.start, batch.end, core.FunctionID(batch.start+1), asmOptions...)
				if err != nil {
					if isHeavy {
						<-heavySem
					}
					setErr(err)
					continue
				}
				if len(parsed.IR.Functions) == 0 {
					if isHeavy {
						<-heavySem
					}
					continue
				}
				conv, err := lowir2ssa.Convert(runCtx, parsed.IR, baseConvertOpts...)
				parsed = nil
				if err != nil {
					if isHeavy {
						<-heavySem
					}
					setErr(err)
					continue
				}
				local := ssapass.NewProgramSummary()
				local.CollectProgram(conv.Program)
				conv = nil
				if isHeavy {
					<-heavySem
				}
				mu.Lock()
				summary.Merge(local)
				mu.Unlock()
			}
		}()
	}

dispatch:
	for b := range batches {
		select {
		case jobs <- b:
		case <-runCtx.Done():
			break dispatch
		}
	}
	close(jobs)
	wg.Wait()

	if firstErr != nil {
		return nil, firstErr
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	summary.Finalize()
	return summary, nil
}

// runOrderedBatches decouples CPU execution (workerSem) from in-order output
// consumption (windowSem) and caps concurrent heavy batches via heavySem so
// giant single functions never cause head-of-line stalling or memory spikes.
func runOrderedBatches[T any](ctx context.Context, batches []funcBatch, workers, windowSize int, heavyThreshold uint64, work func(context.Context, int) (T, error), consume func(int, T) error) error {
	totalBatches := len(batches)
	if totalBatches == 0 {
		return nil
	}
	if workers < 1 {
		workers = 1
	}
	if workers > totalBatches {
		workers = totalBatches
	}
	if windowSize < workers {
		windowSize = workers
	}
	if windowSize > totalBatches {
		windowSize = totalBatches
	}

	type resultItem struct {
		val T
		err error
	}
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	windowSem := make(chan struct{}, windowSize)
	workerSem := make(chan struct{}, workers)
	heavyLimit := workers
	if heavyLimit > 2 {
		heavyLimit = 2
	}
	var heavySem chan struct{}
	if heavyThreshold > 0 && heavyLimit < workers {
		heavySem = make(chan struct{}, heavyLimit)
	}

	slots := make([]chan resultItem, totalBatches)
	for i := range slots {
		slots[i] = make(chan resultItem, 1)
	}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for b := 0; b < totalBatches; b++ {
			select {
			case windowSem <- struct{}{}:
			case <-runCtx.Done():
				return
			}
			if err := runCtx.Err(); err != nil {
				<-windowSem
				return
			}
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				select {
				case workerSem <- struct{}{}:
				case <-runCtx.Done():
					return
				}
				isHeavy := heavySem != nil && batches[idx].codeBytes >= heavyThreshold
				if isHeavy {
					select {
					case heavySem <- struct{}{}:
					case <-runCtx.Done():
						<-workerSem
						return
					}
				}
				val, err := work(runCtx, idx)
				if isHeavy {
					<-heavySem
				}
				<-workerSem
				slots[idx] <- resultItem{val: val, err: err}
			}(b)
		}
	}()

	for b := 0; b < totalBatches; b++ {
		var item resultItem
		select {
		case item = <-slots[b]:
		case <-runCtx.Done():
			cancel()
			wg.Wait()
			return runCtx.Err()
		}
		if item.err != nil {
			<-windowSem
			cancel()
			wg.Wait()
			return item.err
		}
		err := consume(b, item.val)
		<-windowSem
		if err != nil {
			cancel()
			wg.Wait()
			return err
		}
	}
	wg.Wait()
	return nil
}

func buildBaseConvertOptions(artifact *core.Artifact, settings Config, mode Mode) []lowir2ssa.Option {
	opts := make([]lowir2ssa.Option, 0, 4)
	passSettings := settings
	passSettings.Mode = mode
	if passes := effectiveLowIRPasses(passSettings); len(passes) > 0 {
		opts = append(opts, lowir2ssa.WithLowIRPasses(passes...))
	}
	opts = append(opts,
		lowir2ssa.WithConsumeIR(),
		lowir2ssa.WithPointerReader(pointerReader(artifact)),
		lowir2ssa.WithLanguage(ssaconfig.C),
	)
	return opts
}

func validateSSAPasses(names []string) error {
	if len(names) == 0 {
		return nil
	}
	known := make(map[string]bool)
	for _, p := range ssapass.All() {
		known[p.Name] = true
	}
	known["redefine-type-value"] = true
	for _, name := range names {
		if !known[name] {
			return fmt.Errorf("unknown SSA pass %q", name)
		}
	}
	return nil
}

func resolveWorkers(workers, maxDefault int) int {
	if workers > 0 {
		return workers
	}
	n := runtime.GOMAXPROCS(0)
	if n < 1 {
		return 1
	}
	if n > maxDefault {
		return maxDefault
	}
	return n
}

func languageLimitDiagnostics(artifact *core.Artifact) []core.Diagnostic {
	if artifact == nil || artifact.Language == nil || len(artifact.Language.Notes) == 0 {
		return nil
	}
	diags := make([]core.Diagnostic, 0, len(artifact.Language.Notes))
	for _, note := range artifact.Language.Notes {
		diags = append(diags, core.Diagnostic{Code: "language-limit", Message: note})
	}
	return diags
}
