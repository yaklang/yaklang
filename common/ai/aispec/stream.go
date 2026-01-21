package aispec

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http/httputil"
	"strings"
	"sync"
	"time"

	"github.com/samber/lo"

	"github.com/yaklang/yaklang/common/jsonextractor"
	"github.com/yaklang/yaklang/common/jsonpath"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/utils/lowhttp/poc"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
)

func mergeReasonIntoOutputStream(reason io.Reader, out io.Reader) io.Reader {
	pr, pw := utils.NewBufPipe(nil)
	go func() {
		defer pw.Close()
		defer func() {
			if err := recover(); err != nil {
				log.Errorf("panic in mergeReasonIntoOutputStream: %v", err)
				utils.PrintCurrentGoroutineRuntimeStack()
			}
		}()
		reasonBuf := bufio.NewReader(reason)
		runeResult, n, _ := reasonBuf.ReadRune()
		if n > 0 {
			pw.WriteString("<think>")
			pw.WriteString(string([]rune{runeResult}))
			reasonBuf.WriteTo(pw)
			pw.WriteString("</think>\n")
		}
		io.Copy(pw, out)
	}()
	return pr
}

// processAIResponse 处理流式响应
// If toolCallCallback is not nil, tool_calls will be passed to the callback instead of being
// converted to <|TOOL_CALL...|> format in the output stream.
func processAIResponse(r []byte, closer io.ReadCloser, outWriter io.Writer, reasonWriter io.Writer, toolCallCallback func([]*ToolCall)) {
	defer func() {
		utils.CallGeneralClose(reasonWriter)
		utils.CallGeneralClose(outWriter)
	}()

	var chunked bool
	if te := lowhttp.GetHTTPPacketHeader(r, "transfer-encoding"); utils.IContains(te, "chunked") {
		chunked = true
	}

	var mirrorResponse bytes.Buffer
	if lowhttp.GetStatusCodeFromResponse(r) > 299 {
		log.Warnf("response status code: %v", lowhttp.GetStatusCodeFromResponse(r))
		defer func() {
			if mirrorResponse.Len() > 0 {
				log.Infof("response body: %v", utils.ShrinkString(mirrorResponse.String(), 400))
			}
		}()
	}

	var reader io.Reader = closer
	ioReader := reader
	ioReader = io.TeeReader(ioReader, &mirrorResponse)

	start := time.Now()
	var firstbuf = make([]byte, 1)
	n, err := io.ReadFull(ioReader, firstbuf)
	if n <= 0 && err != nil {
		log.Debugf("no body read")
		return
	}
	utils.Debug(func() {
		log.Infof("read first byte [%#v] delay: %v", string(firstbuf), time.Since(start))
	})

	var chunkedErrorMirror bytes.Buffer
	if chunked {
		ioReader = httputil.NewChunkedReader(io.MultiReader(bytes.NewBufferString(string(firstbuf)), ioReader))
	} else {
		ioReader = io.MultiReader(bytes.NewBufferString(string(firstbuf)), ioReader)
	}

	lineReader := bufio.NewReader(ioReader)
	haveReason := false
	reasonFinished := false
	onceStartReason := sync.Once{}
	onceEndReason := sync.Once{}

	checkOnceNonStream := utils.NewOnce()
	skipStream := utils.NewAtomicBool()
	for {
		line, err := utils.BufioReadLine(lineReader)
		if err != nil && string(line) == "" {
			if err != io.EOF {
				log.Warnf("failed to read chunk line: %v, mirror: %#v", err, utils.ShrinkString(chunkedErrorMirror.String(), 200))
				fmt.Println(chunkedErrorMirror.String())
			}
			return
		}
		if string(line) == "" {
			continue
		}

		checkOnceNonStream.Do(func() {
			if strings.HasPrefix(string(line), "{") || strings.HasPrefix(string(line), "[") {
				skipStream.SetTo(true)
			}
		})
		if skipStream.IsSet() {
			// withdraw stream handler, switch to non-stream handler
			multireader := io.MultiReader(bytes.NewBufferString(string(line)+"\n"), lineReader)
			handleResponseErr := jsonextractor.ExtractStructuredJSONFromStream(
				multireader, jsonextractor.WithObjectKeyValue(func(key string, data any) {
					if key != "message" {
						return
					}
					result := utils.InterfaceToGeneralMap(data)
					reasonContent := utils.InterfaceToString(utils.MapGetString(result, "reasoning_content"))
					content := utils.InterfaceToString(utils.MapGetString(result, "content"))
					reasonWriter.Write([]byte(reasonContent))
					outWriter.Write([]byte(content))
					if toolcallRaw, ok := result["tool_calls"]; ok {
						toolcallList := utils.InterfaceToSliceInterface(toolcallRaw)
						if toolCallCallback != nil {
							// Use callback mode: parse and pass ToolCall objects to callback
							var toolCalls []*ToolCall
							for i, toolcall := range toolcallList {
								toolcallMap := utils.InterfaceToGeneralMap(toolcall)
								funcMap := utils.MapGetMapRaw(toolcallMap, "function")
								name := utils.MapGetString(funcMap, "name")
								if name == "" {
									continue
								}
								// Extract index from response, default to array position if not present
								index := i
								if idxRaw, ok := toolcallMap["index"]; ok {
									if idxFloat, ok := idxRaw.(float64); ok {
										index = int(idxFloat)
									} else if idxInt, ok := idxRaw.(int); ok {
										index = idxInt
									}
								}
								tc := &ToolCall{
									Index: index,
									ID:    utils.MapGetString(toolcallMap, "id"),
									Type:  utils.MapGetString(toolcallMap, "type"),
									Function: FuncReturn{
										Name:      name,
										Arguments: utils.MapGetString(funcMap, "arguments"),
									},
								}
								toolCalls = append(toolCalls, tc)
							}
							if len(toolCalls) > 0 {
								toolCallCallback(toolCalls)
							}
						} else {
							// Original behavior: convert to <|TOOL_CALL...|> format
							for _, toolcall := range toolcallList {
								toolcallMap := utils.InterfaceToGeneralMap(toolcall)
								funcMap := utils.MapGetMapRaw(toolcallMap, "function")
								name := utils.MapGetString(funcMap, "name")
								if name == "" {
									continue
								}
								funcMapRaw, err := json.Marshal(funcMap)
								if err != nil {
									continue
								}
								callData, err := utils.RenderTemplate(`
<|TOOL_CALL_{{ .Nonce }}|>
{{ .Data }}
<|TOOL_CALL_END{{ .Nonce }}|>
`, map[string]any{
									"Nonce": utils.RandStringBytes(4),
									"Data":  string(funcMapRaw),
								})
								if err != nil {
									continue
								}
								outWriter.Write([]byte(callData))
							}
						}
					}
				}),
			)
			if handleResponseErr != nil {
				log.Errorf("error in non-stream json extraction: %v", handleResponseErr)
			}
			return
		}

		lineStr := string(line)
		jsonIdentifiers := jsonextractor.ExtractStandardJSON(lineStr)
		for _, j := range jsonIdentifiers {
			var reasonDelta string
			if !reasonFinished {
				reasonContent := jsonpath.Find(j, `$..choices[*].delta.reasoning_content`)
				reasonStrs := lo.Map(utils.InterfaceToSliceInterface(reasonContent), func(reason any, idx int) string {
					if utils.IsNil(reason) {
						return ""
					}
					return fmt.Sprint(reason)
				})
				reasonDelta = strings.Join(reasonStrs, "")
			}

			handled := false
			if reasonDelta != "" {
				handled = true
				onceStartReason.Do(func() {
					haveReason = true
				})
				reasonWriter.Write([]byte(reasonDelta))
			}

			// First, check for tool_calls in delta - this MUST be handled separately from content
			toolCallsRaw := jsonpath.Find(j, `$..choices[*].delta.tool_calls`)
			toolCallsList := utils.InterfaceToSliceInterface(toolCallsRaw)
			if len(toolCallsList) > 0 {
				handled = true
				// Process tool_calls - either via callback or legacy format
				for _, tcArrayRaw := range toolCallsList {
					tcArray := utils.InterfaceToSliceInterface(tcArrayRaw)
					if toolCallCallback != nil {
						// Use callback mode: parse and pass ToolCall objects to callback
						var toolCalls []*ToolCall
						for i, toolcall := range tcArray {
							toolcallMap := utils.InterfaceToGeneralMap(toolcall)
							funcMap := utils.MapGetMapRaw(toolcallMap, "function")
							name := utils.MapGetString(funcMap, "name")
							args := utils.MapGetString(funcMap, "arguments")
							// In streaming, tool_calls come incrementally - we may only have partial data
							if name == "" && args == "" {
								continue
							}
							// Extract index from response, default to array position if not present
							index := i
							if idxRaw, ok := toolcallMap["index"]; ok {
								if idxFloat, ok := idxRaw.(float64); ok {
									index = int(idxFloat)
								} else if idxInt, ok := idxRaw.(int); ok {
									index = idxInt
								}
							}
							tc := &ToolCall{
								Index: index,
								ID:    utils.MapGetString(toolcallMap, "id"),
								Type:  utils.MapGetString(toolcallMap, "type"),
								Function: FuncReturn{
									Name:      name,
									Arguments: args,
								},
							}
							toolCalls = append(toolCalls, tc)
						}
						if len(toolCalls) > 0 {
							toolCallCallback(toolCalls)
						}
					} else {
						// Legacy mode: convert to <|TOOL_CALL...|> format for complete tool calls,
						// or accumulate incremental arguments to output
						for _, toolcall := range tcArray {
							toolcallMap := utils.InterfaceToGeneralMap(toolcall)
							funcMap := utils.MapGetMapRaw(toolcallMap, "function")
							name := utils.MapGetString(funcMap, "name")
							args := utils.MapGetString(funcMap, "arguments")

							if name != "" {
								// Complete tool call with name - use template format
								funcMapRaw, err := json.Marshal(funcMap)
								if err != nil {
									continue
								}
								callData, err := utils.RenderTemplate(`
<|TOOL_CALL_{{ .Nonce }}|>
{{ .Data }}
<|TOOL_CALL_END{{ .Nonce }}|>
`, map[string]any{
									"Nonce": utils.RandStringBytes(4),
									"Data":  string(funcMapRaw),
								})
								if err != nil {
									continue
								}
								outWriter.Write([]byte(callData))
							} else if args != "" {
								// Incremental arguments only - append to output stream
								// This maintains backward compatibility for legacy mode
								outWriter.Write([]byte(args))
							}
						}
					}
				}
			}

			// Process regular content - only if it's NOT tool_calls arguments
			results := jsonpath.Find(j, `$..choices[*].delta.content`)
			wordList := utils.InterfaceToSliceInterface(results)
			for _, raw := range wordList {
				handled = true
				data := codec.AnyToBytes(raw)
				if len(data) > 0 {
					onceEndReason.Do(func() {
						if haveReason {
							reasonFinished = true
						}
						if w, ok := reasonWriter.(io.Closer); ok {
							w.Close()
						}
					})
				}
				outWriter.Write(data)
			}
			if !handled {
				if ret := codec.AnyToString(jsonpath.Find(j, `$..finish_reason`)); utils.IContains(ret, "stop") {
					//log.Info("finished normal")
				} else if strings.TrimRight(codec.AnyToString(jsonpath.Find(j, `$..error`)), "[]\"'") != "" {
					log.Errorf("error for stream fetching: %v", j)
				} else {
					//log.Infof("extra ai stream json: %v", j)
				}
			}
		}
	}
}

func appendStreamHandlerPoCOptionEx(isStream bool, opts []poc.PocConfigOption, toolCallCallback func([]*ToolCall)) (io.Reader, io.Reader, []poc.PocConfigOption, func()) {
	outReader, outWriter := utils.NewBufPipe(nil)
	reasonReader, reasonWriter := utils.NewBufPipe(nil)

	cancelFunc := func() {
		outWriter.Close()
		reasonWriter.Close()
	}

	opts = append(opts, poc.WithBodyStreamReaderHandler(func(r []byte, closer io.ReadCloser) {
		processAIResponse(r, closer, outWriter, reasonWriter, toolCallCallback)

		//if isStream {
		//} else {
		//	processNonStreamResponse(r, closer, outWriter, reasonWriter)
		//}
	}))

	return outReader, reasonReader, opts, cancelFunc
}

func ChatWithStream(
	url string, model string, msg string,
	httpErrHandler func(err error),
	reasonStream func(io.Reader),
	opt func() ([]poc.PocConfigOption, error),
	opts ...ChatBaseOption,
) (io.Reader, error) {
	pr, pw := utils.NewBufPipe(nil)
	reasonPr, reasonPw := utils.NewBufPipe(nil)
	baseOpt := []ChatBaseOption{
		WithChatBase_PoCOptions(opt), WithChatBase_StreamHandler(func(reader io.Reader) {
			defer func() {
				reasonPw.Close()
				pw.Close()
			}()
			if reasonStream != nil {
				reasonStream(reader)
			} else {
				io.Copy(reasonPw, reader)
			}
		}), WithChatBase_ReasonStreamHandler(func(reader io.Reader) {
			io.Copy(pw, reader)
		}), WithChatBase_ErrHandler(httpErrHandler),
	}

	baseOpt = append(baseOpt, opts...)
	go func() {
		_, _ = ChatBase(url, model, msg, baseOpt...)
	}()
	return mergeReasonIntoOutputStream(reasonPr, pr), nil
}
