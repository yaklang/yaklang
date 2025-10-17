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

func appendStreamHandlerPoCOption(opts []poc.PocConfigOption) (io.Reader, []poc.PocConfigOption) {
	out, reason, opts, _ := appendStreamHandlerPoCOptionEx(true, opts)
	pr := mergeReasonIntoOutputStream(reason, out)
	return pr, opts
}

// processAIResponse 处理流式响应
func processAIResponse(r []byte, closer io.ReadCloser, outWriter io.Writer, reasonWriter io.Writer) {
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

			results := jsonpath.Find(j, `$..choices[*].delta.content`)
			wordList := utils.InterfaceToSliceInterface(results)
			if len(wordList) <= 0 {
				log.Debugf("cannot identifier delta content, try to fetch arguments for: %v", j)
				wordList = utils.InterfaceToSliceInterface(jsonpath.Find(j, `$..choices[*].delta.tool_calls[*].function.arguments`))
			}
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

func appendStreamHandlerPoCOptionEx(isStream bool, opts []poc.PocConfigOption) (io.Reader, io.Reader, []poc.PocConfigOption, func()) {
	outReader, outWriter := utils.NewBufPipe(nil)
	reasonReader, reasonWriter := utils.NewBufPipe(nil)

	cancelFunc := func() {
		outWriter.Close()
		reasonWriter.Close()
	}

	opts = append(opts, poc.WithBodyStreamReaderHandler(func(r []byte, closer io.ReadCloser) {
		processAIResponse(r, closer, outWriter, reasonWriter)

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
