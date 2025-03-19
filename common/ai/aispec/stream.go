package aispec

import (
	"bufio"
	"encoding/json"
	"fmt"
	"github.com/samber/lo"
	"io"
	"net/http/httputil"
	"strings"
	"sync"

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
		var firstByte = make([]byte, 1)
		n, _ := io.ReadFull(reason, firstByte)
		if n > 0 {
			pw.WriteString("<think>")
			pw.Write(firstByte)
			io.Copy(pw, reason)
			pw.WriteString("</think>\n")
		}
		io.Copy(pw, out)
	}()
	return pr
}

func appendStreamHandlerPoCOption(opts []poc.PocConfigOption) (io.Reader, []poc.PocConfigOption) {
	out, reason, opts := appendStreamHandlerPoCOptionEx(opts)
	pr := mergeReasonIntoOutputStream(reason, out)
	return pr, opts
}

func appendStreamHandlerPoCOptionEx(opts []poc.PocConfigOption) (io.Reader, io.Reader, []poc.PocConfigOption) {
	outReader, outWriter := utils.NewBufPipe(nil)
	reasonReader, reasonWriter := utils.NewBufPipe(nil)
	opts = append(opts, poc.WithBodyStreamReaderHandler(func(r []byte, closer io.ReadCloser) {
		defer func() {
			outWriter.Write([]byte{'\n'})
			outWriter.Close()
			reasonWriter.Close()
		}()
		var chunked bool
		if te := lowhttp.GetHTTPPacketHeader(r, "transfer-encoding"); utils.IContains(te, "chunked") {
			chunked = true
		}

		if lowhttp.GetStatusCodeFromResponse(r) > 299 {
			log.Warnf("response status code: %v", lowhttp.GetStatusCodeFromResponse(r))
		}

		var reader io.Reader = closer
		// reader = io.TeeReader(reader, os.Stdout)
		var ioReader io.Reader = utils.NewTrimLeftReader(reader)
		if chunked {
			ioReader = httputil.NewChunkedReader(ioReader)
		}
		lineReader := bufio.NewReader(ioReader)
		haveReason := false
		reasonFinished := false
		onceStartReason := sync.Once{}
		onceEndReason := sync.Once{}

		for {
			line, err := utils.BufioReadLine(lineReader)
			if err != nil && string(line) == "" {
				if err != io.EOF {
					log.Warnf("failed to read line [%#v]: %v", line, err)
				}
				return
			}
			if string(line) == "" {
				continue
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
							reasonWriter.Close()
						})
					}
					outWriter.Write(data)
				}
				if !handled {
					if ret := codec.AnyToString(jsonpath.Find(j, `$..finish_reason`)); utils.IContains(ret, "stop") {
						log.Info("finished normal")
					} else if strings.TrimRight(codec.AnyToString(jsonpath.Find(j, `$..error`)), "[]\"'") != "" {
						log.Errorf("error for stream fetching: %v", j)
					} else {
						log.Infof("extra ai stream json: %v", j)
					}
				}
			}
		}
	}))
	return outReader, reasonReader, opts
}

func ChatWithStream(url string, model string, msg string, httpErrHandler func(err error), opt func() ([]poc.PocConfigOption, error)) (io.Reader, error) {
	opts, err := opt()
	if err != nil {
		return nil, utils.Wrap(err, "failed to get options")
	}

	msgIns := NewChatMessage(model, []ChatDetail{NewUserChatDetail(msg)})
	msgIns.Stream = true

	raw, err := json.Marshal(msgIns)
	if err != nil {
		return nil, utils.Wrap(err, "json.Marshal failed")
	}

	opts = append(opts, poc.WithReplaceHttpPacketBody(raw, false))

	pr, opts := appendStreamHandlerPoCOption(opts)
	go func() {
		rsp, _, err := poc.DoPOST(url, opts...)
		if err != nil {
			if httpErrHandler == nil {
				log.Errorf("failed to post stream request: %v", err)
			} else {
				httpErrHandler(err)
			}
			return
		}
		_ = rsp
	}()
	return pr, nil
}
