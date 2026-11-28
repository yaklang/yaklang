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
	defaultStreamBatchSize = 64
	defaultSSABatchSize    = 32
	defaultSaveBatchSize   = 256
	defaultMaxWorkers      = 8
	defaultSSAMaxWorkers   = 4
	streamBufSize          = 256 * 1024
	maxPooledBatchBufCap   = 4 << 20
	largeProjectThreshold  = 16 << 20
)

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
	batchSize := resolveBatchSize(settings.BatchSize, defaultStreamBatchSize)
	if settings.Mode == ModeSSAIR || settings.Mode == ModeSource {
		workers = resolveWorkers(settings.Workers, defaultSSAMaxWorkers)
		batchSize = resolveBatchSize(settings.BatchSize, defaultSSABatchSize)
	}

	switch settings.Mode {
	case ModeASM:
		if err := streamASM(ctx, bw, emitHeader, artifact, boundaries, asmOptions, workers, batchSize, res); err != nil {
			return nil, err
		}
	case ModeLowIR:
		if err := streamLowIR(ctx, bw, emitHeader, artifact, boundaries, asmOptions, workers, batchSize, res); err != nil {
			return nil, err
		}
	case ModeSSAIR, ModeSource:
		if err := streamSSAOrSource(ctx, bw, emitHeader, artifact, boundaries, asmOptions, settings, workers, batchSize, res); err != nil {
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

	batchSize := resolveBatchSize(settings.BatchSize, defaultSaveBatchSize)
	workers := resolveWorkers(settings.Workers, defaultSSAMaxWorkers)
	totalBatches := (len(boundaries) + batchSize - 1) / batchSize
	stringAt := stringAtFunc(artifact)
	baseConvertOpts := buildBaseConvertOptions(artifact, settings, ModeSSAIR)

	var summary *ssapass.ProgramSummary
	if totalBatches > 1 && ssapass.HasInterproceduralPass(settings.SSAPasses) {
		var err error
		summary, err = buildProgramSummary(ctx, artifact, boundaries, asmOptions, baseConvertOpts, workers, batchSize)
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

	nextID := core.FunctionID(1)
	for b := 0; b < totalBatches; b++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		start, end := batchBounds(len(boundaries), b, batchSize)
		unitKey := fmt.Sprintf("reify-batch-%d", b)
		program.BeginCompileUnit(unitKey)
		program.Cache.DisableInstructionSpill()

		parsed, err := asm2lowir.ParseBatch(ctx, artifact, boundaries, start, end, nextID, asmOptions...)
		if err != nil {
			program.Cache.EnableInstructionSpill()
			program.EndCompileUnit()
			return nil, err
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

func streamASM(ctx context.Context, w io.Writer, emitHeader func() error, artifact *core.Artifact, boundaries []core.FunctionBoundary, asmOptions []asm2lowir.Option, workers, batchSize int, res *StreamResult) error {
	totalBatches := (len(boundaries) + batchSize - 1) / batchSize
	type asmChunk struct {
		text  string
		insts int
		diags []core.Diagnostic
	}
	err := runOrderedBatches(ctx, totalBatches, workers,
		func(batchCtx context.Context, b int) (asmChunk, error) {
			start, end := batchBounds(len(boundaries), b, batchSize)
			dis, err := asm2lowir.DisassembleBatch(batchCtx, artifact, boundaries, start, end, asmOptions...)
			if err != nil {
				return asmChunk{}, err
			}
			return asmChunk{text: dis.Text, insts: dis.Instructions, diags: dis.Diagnostics}, nil
		},
		func(_ int, chunk asmChunk) error {
			res.Instructions += chunk.insts
			res.Diagnostics = append(res.Diagnostics, chunk.diags...)
			if chunk.insts == 0 && res.Instructions == 0 && totalBatches == 1 {
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

func streamLowIR(ctx context.Context, w io.Writer, emitHeader func() error, artifact *core.Artifact, boundaries []core.FunctionBoundary, asmOptions []asm2lowir.Option, workers, batchSize int, res *StreamResult) error {
	totalBatches := (len(boundaries) + batchSize - 1) / batchSize
	type lowirChunk struct {
		ir    *core.LowIR
		diags []core.Diagnostic
	}
	headerPrinted := false
	err := runOrderedBatches(ctx, totalBatches, workers,
		func(batchCtx context.Context, b int) (lowirChunk, error) {
			start, end := batchBounds(len(boundaries), b, batchSize)
			parsed, err := asm2lowir.ParseBatch(batchCtx, artifact, boundaries, start, end, core.FunctionID(start+1), asmOptions...)
			if err != nil {
				return lowirChunk{}, err
			}
			return lowirChunk{ir: parsed.IR, diags: parsed.Diagnostics}, nil
		},
		func(_ int, chunk lowirChunk) error {
			res.Diagnostics = append(res.Diagnostics, chunk.diags...)
			if chunk.ir == nil || len(chunk.ir.Functions) == 0 {
				return nil
			}
			res.Functions += len(chunk.ir.Functions)
			res.Blocks += countBlocks(chunk.ir)
			res.Instructions += countInstructions(chunk.ir)
			res.Values += countValues(chunk.ir)
			if !headerPrinted {
				if err := emitHeader(); err != nil {
					return err
				}
				if err := lowirtext.FormatHeaderTo(w, chunk.ir); err != nil {
					return err
				}
				headerPrinted = true
			}
			for _, fn := range chunk.ir.Functions {
				if err := lowirtext.FormatFunctionTo(w, fn); err != nil {
					return err
				}
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

func streamSSAOrSource(ctx context.Context, w io.Writer, emitHeader func() error, artifact *core.Artifact, boundaries []core.FunctionBoundary, asmOptions []asm2lowir.Option, settings Config, workers, batchSize int, res *StreamResult) error {
	totalBatches := (len(boundaries) + batchSize - 1) / batchSize
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
		summary, err = buildProgramSummary(ctx, artifact, boundaries, asmOptions, baseConvertOpts, workers, batchSize)
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

	err := runOrderedBatches(ctx, totalBatches, workers,
		func(batchCtx context.Context, b int) (*renderedBatch, error) {
			start, end := batchBounds(len(boundaries), b, batchSize)
			parsed, err := asm2lowir.ParseBatch(batchCtx, artifact, boundaries, start, end, core.FunctionID(start+1), asmOptions...)
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

func buildProgramSummary(ctx context.Context, artifact *core.Artifact, boundaries []core.FunctionBoundary, asmOptions []asm2lowir.Option, baseConvertOpts []lowir2ssa.Option, workers, batchSize int) (*ssapass.ProgramSummary, error) {
	totalBatches := (len(boundaries) + batchSize - 1) / batchSize
	summary := ssapass.NewProgramSummary()
	err := runOrderedBatches(ctx, totalBatches, workers,
		func(batchCtx context.Context, b int) (*ssapass.ProgramSummary, error) {
			start, end := batchBounds(len(boundaries), b, batchSize)
			parsed, err := asm2lowir.ParseBatch(batchCtx, artifact, boundaries, start, end, core.FunctionID(start+1), asmOptions...)
			if err != nil {
				return nil, err
			}
			if len(parsed.IR.Functions) == 0 {
				return nil, nil
			}
			conv, err := lowir2ssa.Convert(batchCtx, parsed.IR, baseConvertOpts...)
			parsed = nil
			if err != nil {
				return nil, err
			}
			local := ssapass.NewProgramSummary()
			local.CollectProgram(conv.Program)
			return local, nil
		},
		func(_ int, local *ssapass.ProgramSummary) error {
			if local != nil {
				summary.Merge(local)
			}
			return nil
		},
	)
	if err != nil {
		return nil, err
	}
	summary.Finalize()
	return summary, nil
}

// runOrderedBatches executes totalBatches workers bounded by a semaphore acquired
// before launching each batch and released only after consume finishes in order,
// capping concurrent live batch memory to at most workers batches.
func runOrderedBatches[T any](ctx context.Context, totalBatches, workers int, work func(context.Context, int) (T, error), consume func(int, T) error) error {
	if totalBatches == 0 {
		return nil
	}
	if workers < 1 {
		workers = 1
	}
	if workers > totalBatches {
		workers = totalBatches
	}

	type resultItem struct {
		val T
		err error
	}
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	sem := make(chan struct{}, workers)
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
			case sem <- struct{}{}:
			case <-runCtx.Done():
				return
			}
			if err := runCtx.Err(); err != nil {
				<-sem
				return
			}
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				val, err := work(runCtx, idx)
				slots[idx] <- resultItem{val: val, err: err}
			}(b)
		}
	}()

	var firstErr error
	for b := 0; b < totalBatches; b++ {
		var item resultItem
		select {
		case item = <-slots[b]:
		case <-runCtx.Done():
			<-sem
			if firstErr == nil {
				firstErr = runCtx.Err()
			}
			cancel()
			wg.Wait()
			return firstErr
		}
		if item.err != nil {
			<-sem
			if firstErr == nil {
				firstErr = item.err
			}
			cancel()
			wg.Wait()
			return firstErr
		}
		err := consume(b, item.val)
		<-sem
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

func resolveBatchSize(size, fallback int) int {
	if size > 0 {
		return size
	}
	return fallback
}

func batchBounds(total, batchIdx, batchSize int) (int, int) {
	start := batchIdx * batchSize
	end := start + batchSize
	if end > total {
		end = total
	}
	return start, end
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
