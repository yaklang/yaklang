package aireducer

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/ai/aid"
	"github.com/yaklang/yaklang/common/chunkmaker"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/chanx"
)

type Reducer struct {
	config *Config
	input  chunkmaker.ChunkMaker
}

func (r *Reducer) Run() error {
	if r.config.Memory == nil {
		r.config.Memory = aid.GetDefaultContextProvider()
	}
	ch := r.input.OutputChannel()
	for {
		select {
		case chunk, ok := <-ch:
			if !ok {
				if r.config.finishCallback != nil {
					return r.config.finishCallback(r.config, r.config.Memory)
				}
				return nil
			}
			if r.config.callback != nil {
				err := r.config.callback(r.config, r.config.Memory, chunk)
				if err != nil {
					return fmt.Errorf("reducer callback error: %w", err)
				}
				continue
			}
			// Default behavior: dump chunk data
			chunkData := string(chunk.Data())
			fmt.Println(spew.Sdump(chunkData))
		}
	}
}

// splitChunkBySize splits a chunk into smaller pieces respecting ChunkSize limit
func (r *Reducer) splitChunkBySize(data string, chunkSize int64) []string {
	var chunks []string
	dataBytes := []byte(data)

	for len(dataBytes) > 0 {
		end := int(chunkSize)
		if end > len(dataBytes) {
			end = len(dataBytes)
		}

		// Try to split at a convenient boundary (like newline) if possible
		if end < len(dataBytes) {
			// Look for the last newline within the chunk
			for i := end - 1; i >= 0; i-- {
				if dataBytes[i] == '\n' {
					end = i + 1 // Include the newline
					break
				}
			}
			// If no newline found in the latter half, split at chunkSize
			if end == int(chunkSize) && end < len(dataBytes) {
				// Check if we're in the middle of a UTF-8 character
				for end > 0 && (dataBytes[end]&0x80) != 0 && (dataBytes[end]&0xC0) != 0xC0 {
					end--
				}
			}
		}

		chunks = append(chunks, string(dataBytes[:end]))
		dataBytes = dataBytes[end:]
	}

	return chunks
}

func NewReducerEx(maker chunkmaker.ChunkMaker, opts ...Option) (*Reducer, error) {
	config := NewConfig(opts...)
	if maker == nil {
		return nil, errors.New("input chunk maker is nil, not right")
	}

	if config.callback == nil {
		return nil, errors.New("reducer callback is nil, not right")
	}
	return &Reducer{
		input:  maker,
		config: config,
	}, nil
}

func NewReducerFromInputChunk(chunk *chanx.UnlimitedChan[chunkmaker.Chunk], opts ...Option) (*Reducer, error) {
	config := NewConfig(opts...)
	if chunk == nil {
		return nil, errors.New("failed to create chunk channel from reader")
	}
	cm, err := chunkmaker.NewTextChunkMakerEx(chunk, chunkmaker.NewConfig(
		chunkmaker.WithTimeTrigger(config.TimeTriggerInterval),
		chunkmaker.WithChunkSize(config.ChunkSize),
		chunkmaker.WithSeparatorTrigger(config.SeparatorTrigger),
	))
	if err != nil {
		return nil, fmt.Errorf("failed to create chunk maker: %w", err)
	}
	return NewReducerEx(cm, opts...)
}

func NewReducerFromReader(r io.Reader, opts ...Option) (*Reducer, error) {
	config := NewConfig(opts...)

	// If line number is enabled, preprocess the data to add line numbers
	var finalReader io.Reader = r
	if config.EnableLineNumber {
		// Read all data first, then apply line numbers globally
		data, err := io.ReadAll(r)
		if err != nil {
			return nil, fmt.Errorf("failed to read data for line numbering: %w", err)
		}
		numberedData := utils.PrefixLinesWithLineNumbers(string(data))
		finalReader = strings.NewReader(numberedData)
	}

	// If line trigger is enabled, use line-based chunking
	if config.LineTrigger > 0 {
		cm, err := NewLineChunkMaker(finalReader, config)
		if err != nil {
			return nil, fmt.Errorf("failed to create line chunk maker: %w", err)
		}
		return NewReducerEx(cm, opts...)
	}

	// For standard chunking
	cm, err := chunkmaker.NewTextChunkMaker(finalReader, config.ChunkMakerOption()...)
	if err != nil {
		return nil, fmt.Errorf("failed to create chunk maker: %w", err)
	}
	return NewReducerEx(cm, opts...)
}

func NewReducerFromString(i string, opts ...Option) (*Reducer, error) {
	return NewReducerFromReader(bytes.NewReader([]byte(i)), opts...)
}

func NewReducerFromFile(filename string, opts ...Option) (*Reducer, error) {
	config := NewConfig(opts...)

	// Read the entire file content first for preprocessing if needed
	content, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	// If line number is enabled, apply line numbers to the entire content
	if config.EnableLineNumber {
		numberedContent := utils.PrefixLinesWithLineNumbers(string(content))
		content = []byte(numberedContent)
	}

	// If line trigger is enabled, use line-based chunking
	if config.LineTrigger > 0 {
		cm, err := NewLineChunkMaker(bytes.NewReader(content), config)
		if err != nil {
			return nil, fmt.Errorf("failed to create line chunk maker: %w", err)
		}
		return NewReducerEx(cm, opts...)
	}

	// Otherwise use standard chunking
	cm, err := chunkmaker.NewTextChunkMaker(bytes.NewReader(content), config.ChunkMakerOption()...)
	if err != nil {
		return nil, fmt.Errorf("failed to create chunk maker: %w", err)
	}
	return NewReducerEx(cm, opts...)
}

// LineChunkMaker implements line-based chunking with ChunkSize as hard limit
type LineChunkMaker struct {
	ctx    context.Context
	cancel context.CancelFunc
	dst    *chanx.UnlimitedChan[chunkmaker.Chunk]
}

// NewLineChunkMaker creates a new line-based chunk maker
func NewLineChunkMaker(r io.Reader, config *Config) (*LineChunkMaker, error) {
	if config.LineTrigger <= 0 {
		return nil, fmt.Errorf("LineTrigger must be positive, got %d", config.LineTrigger)
	}
	if config.ChunkSize <= 0 {
		return nil, fmt.Errorf("ChunkSize must be positive, got %d", config.ChunkSize)
	}

	ctx, cancel := config.ctx, config.cancel
	if ctx == nil {
		ctx, cancel = context.WithCancel(context.Background())
	}

	dst := chanx.NewUnlimitedChan[chunkmaker.Chunk](ctx, 1000)

	lm := &LineChunkMaker{
		ctx:    ctx,
		cancel: cancel,
		dst:    dst,
	}

	go lm.processLines(r, config)

	return lm, nil
}

func (lm *LineChunkMaker) processLines(r io.Reader, config *Config) {
	defer lm.dst.Close()

	// Read the entire input first to preserve exact formatting
	data, err := io.ReadAll(r)
	if err != nil {
		fmt.Printf("Read error: %v\n", err)
		return
	}

	inputString := string(data)
	endsWithNewline := strings.HasSuffix(inputString, "\n")

	lines := strings.Split(inputString, "\n")
	// If the input ends with newline, Split will create an empty last element
	if endsWithNewline && len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}

	var currentChunk strings.Builder
	var lineCounter int

	for i, line := range lines {
		select {
		case <-lm.ctx.Done():
			return
		default:
		}

		// Add the line to current chunk
		currentChunk.WriteString(line)
		// Add newline after each line except possibly the last one
		if i < len(lines)-1 || (i == len(lines)-1 && endsWithNewline) {
			currentChunk.WriteString("\n")
		}
		lineCounter++

		// Check if we've reached the line limit
		if lineCounter >= config.LineTrigger {
			// Check if current chunk exceeds chunkSize (hard limit)
			chunkData := currentChunk.String()
			if int64(len(chunkData)) <= config.ChunkSize {
				// Chunk is within size limit, emit as single chunk
				chunk := chunkmaker.NewBufferChunk([]byte(chunkData))
				lm.dst.SafeFeed(chunk)
			} else {
				// Chunk exceeds size limit, split by chunkSize
				chunks := lm.splitChunk(chunkData, config.ChunkSize)
				for _, chunkData := range chunks {
					chunk := chunkmaker.NewBufferChunk([]byte(chunkData))
					lm.dst.SafeFeed(chunk)
				}
			}

			// Reset for next chunk
			currentChunk.Reset()
			lineCounter = 0
		}
	}

	// Handle any remaining lines after the loop
	if lineCounter > 0 {
		chunkData := currentChunk.String()
		if int64(len(chunkData)) <= config.ChunkSize {
			chunk := chunkmaker.NewBufferChunk([]byte(chunkData))
			chunk.SetIsTheLastChunk(true)
			lm.dst.SafeFeed(chunk)
		} else {
			// Split the final chunk and mark the last one
			chunks := lm.splitChunk(chunkData, config.ChunkSize)
			for j, chunkData := range chunks {
				chunk := chunkmaker.NewBufferChunk([]byte(chunkData))
				if j == len(chunks)-1 {
					chunk.SetIsTheLastChunk(true)
				}
				lm.dst.SafeFeed(chunk)
			}
		}
	}
}

func (lm *LineChunkMaker) splitAndEmitChunk(data string, chunkSize int64) {
	chunks := lm.splitChunk(data, chunkSize)
	for _, chunkData := range chunks {
		chunk := chunkmaker.NewBufferChunk([]byte(chunkData))
		lm.dst.SafeFeed(chunk)
	}
}

func (lm *LineChunkMaker) splitChunk(data string, chunkSize int64) []string {
	var chunks []string
	dataBytes := []byte(data)

	for len(dataBytes) > 0 {
		end := int(chunkSize)
		if end > len(dataBytes) {
			end = len(dataBytes)
		}

		// Try to split at a convenient boundary (like newline) if possible
		if end < len(dataBytes) {
			// Look for the last newline within the chunk
			for i := end - 1; i >= 0; i-- {
				if dataBytes[i] == '\n' {
					end = i + 1 // Include the newline
					break
				}
			}
			// If no newline found in the latter half, split at chunkSize
			if end == int(chunkSize) && end < len(dataBytes) {
				// Check if we're in the middle of a UTF-8 character
				for end > 0 && (dataBytes[end]&0x80) != 0 && (dataBytes[end]&0xC0) != 0xC0 {
					end--
				}
			}
		}

		chunks = append(chunks, string(dataBytes[:end]))
		dataBytes = dataBytes[end:]
	}

	return chunks
}

func (lm *LineChunkMaker) Close() error {
	if lm.cancel != nil {
		lm.cancel()
	}
	return nil
}

func (lm *LineChunkMaker) OutputChannel() <-chan chunkmaker.Chunk {
	return lm.dst.OutputChannel()
}
