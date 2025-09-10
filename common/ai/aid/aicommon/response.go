package aicommon

import (
	"bytes"
	"context"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/chanx"
	"io"
	"os"
	"sync"
	"time"
)

type AIResponseOutputStream struct {
	NodeType string
	IsReason bool
	out      io.Reader
}

type AIResponse struct {
	taskIndex           string
	ch                  *chanx.UnlimitedChan[*AIResponseOutputStream]
	enableDebug         bool
	consumptionCallback func(current int)
	onOutputFinished    func(string)

	respStartTime time.Time
	reqStartTime  time.Time
}

func (a *AIResponse) SetResponseStartTime(t time.Time) {
	if a == nil {
		return
	}
	a.respStartTime = t
}

func (a *AIResponse) GetResponseStartTime() time.Time {
	if a == nil {
		return time.Time{}
	}
	return a.respStartTime
}

func (a *AIResponse) SetRequestStartTime(t time.Time) {
	if a == nil {
		return
	}
	a.reqStartTime = t
}

func (a *AIResponse) GetRequestStartTime() time.Time {
	if a == nil {
		return time.Time{}
	}
	return a.reqStartTime
}

func (a *AIResponse) GetTaskIndex() string {
	return a.taskIndex
}

func (a *AIResponse) SetTaskIndex(taskIndex string) {
	if a == nil {
		return
	}
	a.taskIndex = taskIndex
}

func (a *AIResponse) Debug(i ...bool) {
	if len(i) <= 0 {
		a.enableDebug = true
		return
	}

	a.enableDebug = i[0]
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

func (a *AIResponse) GetOutputStreamReader(nodeId string, system bool, emitter *Emitter) io.Reader {
	system = false
	pr, pw := utils.NewBufPipe(nil)
	go func() {
		cbBuffer := bytes.NewBuffer(make([]byte, 4096))
		defer func() {
			if a.onOutputFinished != nil {
				a.onOutputFinished(cbBuffer.String())
				// config.ProcessExtendedActionCallback(cbBuffer.String())
			}
		}()
		defer pw.Close()
		wg := new(sync.WaitGroup)
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
					wg.Add(1)
					emitter.EmitSystemReasonStreamEvent(nodeId, a.respStartTime, targetStream, a.GetTaskIndex(), func() {
						wg.Done()
					})
				} else {
					wg.Add(1)
					emitter.EmitReasonStreamEvent(nodeId, a.respStartTime, targetStream, a.GetTaskIndex(), func() {
						wg.Done()
					})
				}
				continue
			}

			targetStream = io.TeeReader(targetStream, pw)
			if system {
				wg.Add(1)
				emitter.EmitSystemStreamEvent(nodeId, a.respStartTime, targetStream, a.GetTaskIndex(), func() {
					wg.Done()
				})
			} else {
				wg.Add(1)
				emitter.EmitStreamEvent(nodeId, a.respStartTime, targetStream, a.GetTaskIndex(), func() {
					wg.Done()
				})
			}
		}
		wg.Wait()
	}()
	return pr
}

func (r *AIResponse) EmitOutputStream(reader io.Reader) {
	r.ch.SafeFeed(&AIResponseOutputStream{
		out: CreateConsumptionReader(reader, r.consumptionCallback),
	})
}

func (r *AIResponse) EmitReasonStream(reader io.Reader) {
	r.ch.SafeFeed(&AIResponseOutputStream{
		IsReason: true,
		out:      CreateConsumptionReader(reader, r.consumptionCallback),
	})
}

func (r *AIResponse) EmitReasonStreamWithoutConsumption(reader io.Reader) {
	r.ch.SafeFeed(&AIResponseOutputStream{
		IsReason: true,
		out:      reader,
	})
}

func (r *AIResponse) EmitOutputStreamWithoutConsumption(reader io.Reader) {
	r.ch.SafeFeed(&AIResponseOutputStream{
		out: reader,
	})
}

func (r *AIResponse) Close() {
	if r == nil {
		return
	}
	if r.ch == nil {
		return
	}

	defer func() {
		if err := recover(); err != nil {
			// Handle panic if necessary, e.g., log it
			log.Errorf("recover from panic when closing AIResponse: %v", err)
			utils.PrintCurrentGoroutineRuntimeStack()
		}
	}()

	r.ch.Close()
}

func NewAIResponse(caller AICallerConfigIf) *AIResponse {
	return &AIResponse{
		ch: chanx.NewUnlimitedChan[*AIResponseOutputStream](context.TODO(), 2),
		consumptionCallback: func(current int) {
			if utils.IsNil(caller) {
				return
			}
			caller.CallAIResponseConsumptionCallback(current)
		},
		onOutputFinished: func(s string) {
			if utils.IsNil(caller) {
				return
			}
			caller.CallAIResponseOutputFinishedCallback(s)
		},
	}
}

func TeeAIResponse(
	aiCaller AICallerConfigIf,
	src *AIResponse,
	onFirstByte func(teeResp *AIResponse),
	onClose func(),
) *AIResponse {
	first := NewAIResponse(aiCaller)
	first.SetTaskIndex(src.GetTaskIndex())
	first.consumptionCallback = nil
	firstReasonReader, firstReasonWriter := utils.NewPipe()
	firstOutputReader, firstOutputWriter := utils.NewPipe()

	second := NewAIResponse(aiCaller)
	second.SetTaskIndex(src.GetTaskIndex())
	secondReasonReader, secondReasonWriter := utils.NewPipe()
	secondOutputReader, secondOutputWriter := utils.NewPipe()

	reasonReader, outputReader := src.GetUnboundStreamReaderEx(func() {
		if onFirstByte != nil {
			onFirstByte(second)
		}
	}, onClose, nil)

	wg := new(sync.WaitGroup)
	wg.Add(2)

	go func() {
		defer wg.Done()
		io.Copy(
			io.MultiWriter(firstReasonWriter, secondReasonWriter),
			reasonReader,
		)
	}()

	go func() {
		defer wg.Done()

		io.Copy(
			secondOutputWriter,
			io.TeeReader(outputReader, firstOutputWriter),
		)
	}()

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
		first.EmitOutputStreamWithoutConsumption(firstOutputReader)
		first.EmitReasonStreamWithoutConsumption(firstReasonReader)
		first.Close()
	}()

	second.EmitOutputStreamWithoutConsumption(secondOutputReader)
	second.EmitReasonStreamWithoutConsumption(secondReasonReader)
	second.Close()

	copyWg.Wait()
	return first
}

func NewUnboundAIResponse() *AIResponse {
	return newUnboundAIResponse()
}

func newUnboundAIResponse() *AIResponse {
	return &AIResponse{
		ch:                  chanx.NewUnlimitedChan[*AIResponseOutputStream](context.TODO(), 2),
		consumptionCallback: func(current int) {},
		onOutputFinished:    func(s string) {},
	}
}
