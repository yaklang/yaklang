package aid

import (
	"bytes"
	"context"
	"io"
	"os"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/schema"

	"sync"

	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/chanx"
)

type AIRequest struct {
	detachCheckpoint       bool
	prompt                 string
	startTime              time.Time
	seqId                  int64
	saveCheckpointCallback func(CheckpointCommitHandler)
	onAcquireSeq           func(int64)
}

func (ai *AIRequest) SetDetachCheckpoint(b bool) {
	ai.detachCheckpoint = b
}

func (ai *AIRequest) IsDetachedCheckpoint() bool {
	return ai.detachCheckpoint
}

type CheckpointCommitHandler func() (*schema.AiCheckpoint, error)

func (r *AIRequest) GetPrompt() string {
	return r.prompt
}

func WithAIRequest_SaveCheckpointCallback(callback func(CheckpointCommitHandler)) AIRequestOption {
	return func(req *AIRequest) {
		req.saveCheckpointCallback = callback
	}
}

func WithAIRequest_OnAcquireSeq(callback func(int64)) AIRequestOption {
	return func(req *AIRequest) {
		req.onAcquireSeq = callback
	}
}

func WithAIRequest_SeqId(i int64) AIRequestOption {
	return func(req *AIRequest) {
		req.seqId = i
	}
}

type AIResponse struct {
	ch                  *chanx.UnlimitedChan[*OutputStream]
	enableDebug         bool
	consumptionCallback func(current int)

	respStartTime time.Time
	reqStartTime  time.Time
}

func (a *AIResponse) Debug(i ...bool) {
	if len(i) <= 0 {
		a.enableDebug = true
		return
	}

	a.enableDebug = i[0]
}

func (c *Config) teeAIResponse(src *AIResponse, onFirstByte func(teeResp *AIResponse), onClose func()) *AIResponse {
	// 创建第一个响应对象
	first := c.NewAIResponse()
	first.consumptionCallback = nil
	firstReasonReader, firstReasonWriter := utils.NewBufPipe(nil)
	firstOutputReader, firstOutputWriter := utils.NewBufPipe(nil)

	// 创建第二个响应对象
	second := c.NewAIResponse()
	second.consumptionCallback = nil
	secondReasonReader, secondReasonWriter := utils.NewBufPipe(nil)
	secondOutputReader, secondOutputWriter := utils.NewBufPipe(nil)

	// 获取原始响应的流读取器
	reasonReader, outputReader := src.GetUnboundStreamReaderEx(func() {
		if onFirstByte != nil {
			onFirstByte(second)
		}
	}, onClose, nil)

	// 使用等待组确保所有数据复制完成
	wg := new(sync.WaitGroup)
	wg.Add(2)

	// 复制原因流到两个目标
	go func() {
		defer wg.Done()
		io.Copy(
			io.MultiWriter(firstReasonWriter, secondReasonWriter),
			reasonReader,
		)
	}()

	// 复制输出流到两个目标
	go func() {
		defer wg.Done()
		io.Copy(
			secondOutputWriter,
			io.TeeReader(outputReader, firstOutputWriter),
		)
	}()

	// 等待所有复制完成后关闭所有写入器和响应
	go func() {
		wg.Wait()
		firstReasonWriter.Close()
		firstOutputWriter.Close()
		secondReasonWriter.Close()
		secondOutputWriter.Close()
	}()

	copyWg := new(sync.WaitGroup)
	copyWg.Add(1)
	go func() {
		defer copyWg.Done()
		first.EmitOutputStreamWithoutConsumption(firstReasonReader)
		first.EmitOutputStreamWithoutConsumption(firstOutputReader)
		first.Close()
	}()

	second.EmitOutputStreamWithoutConsumption(secondReasonReader)
	second.EmitOutputStreamWithoutConsumption(secondOutputReader)
	second.Close()
	// 等待所有复制完成
	copyWg.Wait()
	return first
}

func (a *AIResponse) GetUnboundStreamReaderEx(onFirstByte func(), onClose func(), onError func()) (io.Reader, io.Reader) {
	reasonReader, reasonWriter := utils.NewBufPipe(nil)
	outputReader, outputWriter := utils.NewBufPipe(nil)

	callFirstByte := new(sync.Once)
	callClose := new(sync.Once)
	callError := new(sync.Once)

	syncCh := make(chan struct{})
	haveFirstByte := utils.NewBool(false)
	go func() {
		defer func() {
			select {
			case syncCh <- struct{}{}:
			default:
			}

			if !haveFirstByte.IsSet() {
				callError.Do(func() {
					if onError != nil {
						onError()
					}
				})
			}

			callClose.Do(func() {
				if onClose != nil {
					onClose()
				}
			})
		}()
		defer reasonWriter.Close()
		defer outputWriter.Close()

		// callEmptyOnce := new(sync.Once)

		for i := range a.ch.OutputChannel() {
			if i == nil {
				continue
			}

			if !haveFirstByte.IsSet() {
				var buf = make([]byte, 1)
				n, _ := io.ReadFull(i.out, buf)
				if n > 0 {
					haveFirstByte.SetTo(true)
					select {
					case syncCh <- struct{}{}:
					default:
					}
					callFirstByte.Do(func() {
						if onFirstByte != nil {
							onFirstByte()
						}
					})
					if i.IsReason {
						io.Copy(reasonWriter, io.MultiReader(bytes.NewReader(buf[:n]), i.out))
					} else {
						io.Copy(outputWriter, io.MultiReader(bytes.NewReader(buf[:n]), i.out))
					}
				}
				continue
			}

			if i.IsReason {
				io.Copy(reasonWriter, i.out)
			} else {
				io.Copy(outputWriter, i.out)
			}
		}
	}()
	<-syncCh
	return reasonReader, outputReader
}

func (a *AIResponse) GetUnboundStreamReader(haveReason bool) io.Reader {
	pr, pw := utils.NewBufPipe(nil)
	go func() {
		defer pw.Close()
		for i := range a.ch.OutputChannel() {
			if i == nil {
				continue
			}

			if haveReason && !i.IsReason {
				continue
			}
			targetStream := i.out
			io.Copy(pw, targetStream)
		}
	}()
	return pr
}

func (a *AIResponse) GetOutputStreamReader(nodeId string, system bool, config *Config) io.Reader {
	pr, pw := utils.NewBufPipe(nil)
	go func() {
		cbBuffer := bytes.NewBuffer(make([]byte, 4096))
		defer func() {
			config.ProcessExtendedActionCallback(cbBuffer.String())
		}()
		defer pw.Close()
		for i := range a.ch.OutputChannel() {
			if i == nil {
				continue
			}
			targetStream := io.TeeReader(i.out, cbBuffer)
			if a.enableDebug {
				targetStream = io.TeeReader(targetStream, os.Stdout)
			}
			if i.IsReason {
				if system {
					config.EmitSystemReasonStreamEvent(nodeId, a.respStartTime, targetStream)
				} else {
					config.EmitReasonStreamEvent(nodeId, a.respStartTime, targetStream)
				}
				continue
			}

			targetStream = io.TeeReader(targetStream, pw)
			if system {
				config.EmitSystemStreamEvent(nodeId, a.respStartTime, targetStream)
			} else {
				config.EmitStreamEvent(nodeId, a.respStartTime, targetStream)
			}
		}
		config.WaitForStream()
	}()
	return pr
}

type OutputStream struct {
	NodeType string
	IsReason bool
	out      io.Reader
}

type AIRequestOption func(req *AIRequest)

func NewAIRequest(prompt string, opt ...AIRequestOption) *AIRequest {
	req := &AIRequest{
		prompt:    prompt,
		startTime: time.Now(),
	}
	for _, i := range opt {
		i(req)
	}
	return req
}

type AICallbackType func(config *Config, req *AIRequest) (*AIResponse, error)

func (c *Config) NewAIResponse() *AIResponse {
	return &AIResponse{
		ch:                  chanx.NewUnlimitedChan[*OutputStream](context.TODO(), 2),
		consumptionCallback: c.outputConsumptionCallback,
	}
}

func NewUnboundAIResponse() *AIResponse {
	return newUnboundAIResponse()
}

func newUnboundAIResponse() *AIResponse {
	return &AIResponse{
		ch:                  chanx.NewUnlimitedChan[*OutputStream](context.TODO(), 2),
		consumptionCallback: func(current int) {},
	}
}

func (r *AIResponse) EmitOutputStream(reader io.Reader) {
	r.ch.SafeFeed(&OutputStream{
		out: CreateConsumptionReader(reader, r.consumptionCallback),
	})
}

func (r *AIResponse) EmitOutputStreamWithoutConsumption(reader io.Reader) {
	r.ch.SafeFeed(&OutputStream{
		out: reader,
	})
}

func (r *AIResponse) EmitReasonStream(reader io.Reader) {
	r.ch.SafeFeed(&OutputStream{
		IsReason: true,
		out:      CreateConsumptionReader(reader, r.consumptionCallback),
	})
}

func (r *AIResponse) EmitReasonStreamWithoutConsumption(reader io.Reader) {
	r.ch.SafeFeed(&OutputStream{
		IsReason: true,
		out:      reader,
	})
}

func (r *AIResponse) Close() {
	if r.ch == nil {
		return
	}
	r.ch.Close()
}

func AIChatToAICallbackType(cb func(prompt string, opts ...aispec.AIConfigOption) (string, error)) AICallbackType {
	return func(config *Config, req *AIRequest) (*AIResponse, error) {
		resp := config.NewAIResponse()
		go func() {
			defer resp.Close()

			isStream := false
			output, err := cb(
				req.GetPrompt(),
				aispec.WithStreamHandler(func(reader io.Reader) {
					isStream = true
					resp.EmitOutputStream(reader)
				}),
				aispec.WithReasonStreamHandler(func(reader io.Reader) {
					isStream = true
					resp.EmitReasonStream(reader)
				}),
			)
			if err != nil {
				log.Errorf("chat error: %v", err)
			}
			if !isStream {
				resp.EmitOutputStream(strings.NewReader(output))
			}
		}()
		return resp, nil
	}
}
