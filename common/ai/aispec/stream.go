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

		type firstReadResult struct {
			source string
			data   []byte
			err    error
		}

		reasonBuf := bufio.NewReader(reason)
		outBuf := bufio.NewReader(out)
		firstResultCh := make(chan firstReadResult, 2)

		go func() {
			r, size, err := reasonBuf.ReadRune()
			if err != nil {
				firstResultCh <- firstReadResult{source: "reason", err: err}
				return
			}
			if size > 0 {
				firstResultCh <- firstReadResult{source: "reason", data: []byte(string(r))}
				return
			}
			firstResultCh <- firstReadResult{source: "reason", err: io.EOF}
		}()

		go func() {
			b, err := outBuf.ReadByte()
			if err != nil {
				firstResultCh <- firstReadResult{source: "out", err: err}
				return
			}
			firstResultCh <- firstReadResult{source: "out", data: []byte{b}}
		}()

		first := <-firstResultCh

		writeReason := func(firstBytes []byte) {
			if len(firstBytes) == 0 {
				return
			}
			pw.WriteString("<think>")
			_, _ = pw.Write(firstBytes)
			_, _ = reasonBuf.WriteTo(pw)
			pw.WriteString("</think>\n")
		}

		writeOut := func(firstBytes []byte) {
			if len(firstBytes) > 0 {
				_, _ = pw.Write(firstBytes)
			}
			_, _ = io.Copy(pw, outBuf)
		}

		if first.source == "reason" && len(first.data) > 0 {
			writeReason(first.data)
			second := <-firstResultCh
			if second.source == "out" {
				writeOut(second.data)
			}
			return
		}

		if first.source == "out" && len(first.data) > 0 {
			writeOut(first.data)
			second := <-firstResultCh
			if second.source == "reason" {
				writeReason(second.data)
			}
			return
		}

		second := <-firstResultCh
		if second.source == "reason" {
			writeReason(second.data)
		}
		if second.source == "out" {
			writeOut(second.data)
		}
	}()
	return pr
}

// processAIResponse 处理流式响应
// If toolCallCallback is not nil, tool_calls will be passed to the callback instead of being
// converted to <|TOOL_CALL...|> format in the output stream.
func processAIResponse(r []byte, closer io.ReadCloser, outWriter io.Writer, reasonWriter io.Writer, toolCallCallback func([]*ToolCall), rawResponseCallback func([]byte, []byte)) {
	defer func() {
		utils.CallGeneralClose(reasonWriter)
		utils.CallGeneralClose(outWriter)
	}()

	var chunked bool
	if te := lowhttp.GetHTTPPacketHeader(r, "transfer-encoding"); utils.IContains(te, "chunked") {
		chunked = true
	}

	var mirrorResponse bytes.Buffer
	statusCode := lowhttp.GetStatusCodeFromResponse(r)
	if statusCode > 299 {
		log.Warnf("response status code: %v", statusCode)
		defer func() {
			if mirrorResponse.Len() > 0 {
				log.Infof("response body: %v", utils.ShrinkString(mirrorResponse.String(), 400))
			}
		}()
	}

	defer func() {
		if rawResponseCallback != nil {
			bodyPreview := mirrorResponse.Bytes()
			const maxBodyPreview = 4096
			if len(bodyPreview) > maxBodyPreview {
				bodyPreview = bodyPreview[:maxBodyPreview]
			}
			rawResponseCallback(r, bodyPreview)
		}
	}()

	// 429 rate limit: read body for rawResponseCallback then return early
	if statusCode == 429 {
		io.Copy(&mirrorResponse, closer)
		return
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

func appendStreamHandlerPoCOptionEx(isStream bool, opts []poc.PocConfigOption, toolCallCallback func([]*ToolCall), rawResponseCallback func([]byte, []byte)) (io.Reader, io.Reader, []poc.PocConfigOption, func()) {
	outReader, outWriter := utils.NewBufPipe(nil)
	reasonReader, reasonWriter := utils.NewBufPipe(nil)

	cancelFunc := func() {
		outWriter.Close()
		reasonWriter.Close()
	}

	opts = append(opts, poc.WithBodyStreamReaderHandler(func(r []byte, closer io.ReadCloser) {
		processAIResponse(r, closer, outWriter, reasonWriter, toolCallCallback, rawResponseCallback)
	}))

	opts = append(opts, poc.WithReplaceHttpPacketHeader("Content-Type", "application/json"))
	if isStream {
		opts = append(opts,
			poc.WithConnPool(false),
			poc.WithNoBodyBuffer(true),
			poc.WithReplaceHttpPacketHeader("Accept", "text/event-stream"),
		)
	}

	return outReader, reasonReader, opts, cancelFunc
}

func appendResponsesStreamHandlerPoCOptionEx(isStream bool, opts []poc.PocConfigOption, toolCallCallback func([]*ToolCall), rawResponseCallback func([]byte, []byte)) (io.Reader, io.Reader, []poc.PocConfigOption, func()) {
	outReader, outWriter := utils.NewBufPipe(nil)
	reasonReader, reasonWriter := utils.NewBufPipe(nil)

	cancelFunc := func() {
		outWriter.Close()
		reasonWriter.Close()
	}

	opts = append(opts, poc.WithReplaceHttpPacketHeader("Content-Type", "application/json"))
	if isStream {
		opts = append(opts,
			poc.WithConnPool(false),
			poc.WithNoBodyBuffer(true),
			poc.WithReplaceHttpPacketHeader("Accept", "text/event-stream"),
		)
	}

	opts = append(opts, poc.WithBodyStreamReaderHandler(func(r []byte, closer io.ReadCloser) {
		processAIResponseForResponses(r, closer, outWriter, reasonWriter, toolCallCallback, rawResponseCallback)
	}))

	return outReader, reasonReader, opts, cancelFunc
}

func processAIResponseForResponses(r []byte, closer io.ReadCloser, outWriter io.Writer, reasonWriter io.Writer, toolCallCallback func([]*ToolCall), rawResponseCallback func([]byte, []byte)) {
	defer func() {
		utils.CallGeneralClose(closer)
		utils.CallGeneralClose(reasonWriter)
		utils.CallGeneralClose(outWriter)
	}()

	var chunked bool
	if te := lowhttp.GetHTTPPacketHeader(r, "transfer-encoding"); utils.IContains(te, "chunked") {
		chunked = true
	}

	var mirrorResponse bytes.Buffer
	statusCode := lowhttp.GetStatusCodeFromResponse(r)
	if statusCode > 299 {
		log.Warnf("response status code: %v", statusCode)
		defer func() {
			if mirrorResponse.Len() > 0 {
				log.Infof("response body: %v", utils.ShrinkString(mirrorResponse.String(), 400))
			}
		}()
	}

	defer func() {
		if rawResponseCallback != nil {
			bodyPreview := mirrorResponse.Bytes()
			const maxBodyPreview = 4096
			if len(bodyPreview) > maxBodyPreview {
				bodyPreview = bodyPreview[:maxBodyPreview]
			}
			rawResponseCallback(r, bodyPreview)
		}
	}()

	if statusCode == 429 {
		io.Copy(&mirrorResponse, closer)
		return
	}

	var reader io.Reader = io.TeeReader(closer, &mirrorResponse)
	if chunked {
		reader = httputil.NewChunkedReader(reader)
	}

	contentType := strings.ToLower(lowhttp.GetHTTPPacketHeader(r, "content-type"))
	bufferedReader := bufio.NewReader(reader)
	if strings.Contains(contentType, "text/event-stream") {
		handleResponsesSSEStream(bufferedReader, outWriter, reasonWriter, toolCallCallback)
		return
	}

	prefix, firstNonSpace, err := readResponsesPrefix(bufferedReader)
	if err != nil && len(prefix) == 0 {
		if err != io.EOF {
			log.Warnf("failed to read responses body prefix: %v", err)
		}
		return
	}

	fullReader := io.MultiReader(bytes.NewReader(prefix), bufferedReader)
	if firstNonSpace == '{' || firstNonSpace == '[' {
		body, readErr := io.ReadAll(fullReader)
		if readErr != nil {
			log.Warnf("failed to read responses body: %v", readErr)
			return
		}
		bodyTrim := bytes.TrimSpace(body)
		if len(bodyTrim) == 0 {
			return
		}
		if handleResponsesJSONPayload(bodyTrim, outWriter, reasonWriter, toolCallCallback) {
			return
		}
		handleResponsesSSEStream(bytes.NewReader(body), outWriter, reasonWriter, toolCallCallback)
		return
	}

	handleResponsesSSEStream(fullReader, outWriter, reasonWriter, toolCallCallback)
}

type responsesToolCallState struct {
	byItemID          map[string]*ToolCall
	streamedMessageID map[string]struct{}
}

func newResponsesToolCallState() *responsesToolCallState {
	return &responsesToolCallState{
		byItemID:          make(map[string]*ToolCall),
		streamedMessageID: make(map[string]struct{}),
	}
}

func (s *responsesToolCallState) buildKey(itemID string, index int) string {
	if itemID != "" {
		return itemID
	}
	return fmt.Sprintf("index:%d", index)
}

func (s *responsesToolCallState) getOrCreate(itemID string, index int) *ToolCall {
	key := s.buildKey(itemID, index)
	if tc, ok := s.byItemID[key]; ok {
		return tc
	}
	tc := &ToolCall{
		Index: index,
		Type:  "function",
	}
	s.byItemID[key] = tc
	return tc
}

func (s *responsesToolCallState) markMessageStreamed(itemID string, index int) {
	s.streamedMessageID[s.buildKey(itemID, index)] = struct{}{}
}

func (s *responsesToolCallState) hasMessageStreamed(itemID string, index int) bool {
	_, ok := s.streamedMessageID[s.buildKey(itemID, index)]
	return ok
}

func handleResponsesJSONPayload(body []byte, outWriter io.Writer, reasonWriter io.Writer, toolCallCallback func([]*ToolCall)) bool {
	var payload any
	if err := json.Unmarshal(body, &payload); err != nil {
		return false
	}
	obj := utils.InterfaceToGeneralMap(payload)
	if len(obj) == 0 {
		return false
	}
	extractResponsesOutputFromObject(obj, outWriter, reasonWriter, toolCallCallback)
	return true
}

func handleResponsesSSEPayload(body []byte, outWriter io.Writer, reasonWriter io.Writer, toolCallCallback func([]*ToolCall)) {
	handleResponsesSSEStream(bytes.NewReader(body), outWriter, reasonWriter, toolCallCallback)
}

func handleResponsesSSEStream(reader io.Reader, outWriter io.Writer, reasonWriter io.Writer, toolCallCallback func([]*ToolCall)) {
	lineReader := bufio.NewReader(reader)
	toolState := newResponsesToolCallState()

	for {
		line, err := utils.BufioReadLine(lineReader)
		if err != nil && len(line) == 0 {
			return
		}
		lineStr := strings.TrimSpace(string(line))
		if lineStr == "" {
			continue
		}
		if !strings.HasPrefix(lineStr, "data:") {
			continue
		}
		rawEvent := strings.TrimSpace(strings.TrimPrefix(lineStr, "data:"))
		if rawEvent == "" || rawEvent == "[DONE]" {
			continue
		}

		jsonIdentifiers := jsonextractor.ExtractStandardJSON(rawEvent)
		if len(jsonIdentifiers) == 0 {
			if strings.HasPrefix(rawEvent, "{") {
				jsonIdentifiers = append(jsonIdentifiers, rawEvent)
			} else {
				continue
			}
		}

		for _, j := range jsonIdentifiers {
			var eventMap map[string]any
			if err := json.Unmarshal([]byte(j), &eventMap); err != nil {
				continue
			}
			handleResponsesSSEEvent(eventMap, outWriter, reasonWriter, toolCallCallback, toolState)
		}
	}
}

func readResponsesPrefix(reader *bufio.Reader) ([]byte, byte, error) {
	var prefix bytes.Buffer
	for {
		b, err := reader.ReadByte()
		if err != nil {
			return prefix.Bytes(), 0, err
		}
		prefix.WriteByte(b)
		switch b {
		case ' ', '\n', '\r', '\t':
			continue
		default:
			return prefix.Bytes(), b, nil
		}
	}
}

func handleResponsesSSEEvent(event map[string]any, outWriter io.Writer, reasonWriter io.Writer, toolCallCallback func([]*ToolCall), toolState *responsesToolCallState) {
	eventType := utils.MapGetString(event, "type")
	switch eventType {
	case "response.output_text.delta":
		outputIndex := utils.InterfaceToInt(event["output_index"])
		itemID := utils.MapGetString(event, "item_id")
		delta := utils.MapGetString(event, "delta")
		if delta != "" {
			toolState.markMessageStreamed(itemID, outputIndex)
			outWriter.Write([]byte(delta))
		}
	case "response.reasoning_text.delta", "response.reasoning_summary_text.delta":
		delta := utils.MapGetString(event, "delta")
		if delta != "" {
			reasonWriter.Write([]byte(delta))
		}
	case "response.function_call_arguments.delta":
		outputIndex := utils.InterfaceToInt(event["output_index"])
		itemID := utils.MapGetString(event, "item_id")
		tc := toolState.getOrCreate(itemID, outputIndex)
		if callID := utils.MapGetString(event, "call_id"); callID != "" {
			tc.ID = callID
		}
		if name := utils.MapGetString(event, "name"); name != "" {
			tc.Function.Name = name
		}
		delta := utils.MapGetString(event, "delta")
		if delta != "" {
			tc.Function.Arguments += delta
		}
		if delta == "" {
			return
		}
		if toolCallCallback != nil {
			toolCallCallback([]*ToolCall{tc.Clone()})
		} else {
			outWriter.Write([]byte(delta))
		}
	case "response.output_item.added", "response.output_item.done":
		item := utils.MapGetMapRaw(event, "item")
		if len(item) <= 0 {
			return
		}
		outputIndex := utils.InterfaceToInt(event["output_index"])
		handleResponsesOutputItem(item, outputIndex, eventType, outWriter, reasonWriter, toolCallCallback, toolState)
	case "response.completed":
		// Ignore completed event in streaming mode to avoid duplicate output/tool-calls.
	default:
		if strings.Contains(eventType, "reasoning") {
			delta := utils.MapGetString(event, "delta")
			if delta == "" {
				delta = utils.MapGetString(event, "text")
			}
			if delta != "" {
				reasonWriter.Write([]byte(delta))
			}
		}
	}
}

func handleResponsesOutputItem(item map[string]any, outputIndex int, eventType string, outWriter io.Writer, reasonWriter io.Writer, toolCallCallback func([]*ToolCall), toolState *responsesToolCallState) {
	itemType := utils.MapGetString(item, "type")
	switch itemType {
	case "message":
		text, reason := extractResponsesTextAndReason(item)
		if reason != "" {
			reasonWriter.Write([]byte(reason))
		}
		itemID := utils.MapGetString(item, "id")
		if text != "" && !toolState.hasMessageStreamed(itemID, outputIndex) {
			toolState.markMessageStreamed(itemID, outputIndex)
			outWriter.Write([]byte(text))
		}
	case "reasoning":
		reason := extractResponsesReasoning(item)
		if reason != "" {
			reasonWriter.Write([]byte(reason))
		}
	case "function_call":
		tc := parseResponsesFunctionCall(item, utils.InterfaceToInt(item["output_index"]))
		if tc == nil {
			return
		}
		streamTC := toolState.getOrCreate(utils.MapGetString(item, "id"), tc.Index)
		if streamTC.ID == "" {
			streamTC.ID = tc.ID
		}
		if streamTC.Type == "" {
			streamTC.Type = tc.Type
		}
		if tc.Function.Name != "" {
			streamTC.Function.Name = tc.Function.Name
		}
		if tc.Function.Arguments != "" {
			streamTC.Function.Arguments = tc.Function.Arguments
		}
		if eventType != "response.output_item.done" {
			return
		}
		if toolCallCallback != nil {
			toolCallCallback([]*ToolCall{streamTC.Clone()})
		} else {
			writeLegacyToolCall(outWriter, streamTC)
		}
	}
}

func extractResponsesOutputFromObject(obj map[string]any, outWriter io.Writer, reasonWriter io.Writer, toolCallCallback func([]*ToolCall)) {
	if nested := utils.MapGetMapRaw(obj, "response"); len(nested) > 0 {
		obj = nested
	}

	outputText := utils.MapGetString(obj, "output_text")
	if outputText != "" {
		outWriter.Write([]byte(outputText))
	}

	var toolCalls []*ToolCall
	outputItems := utils.InterfaceToSliceInterface(obj["output"])
	for index, rawItem := range outputItems {
		item := utils.InterfaceToGeneralMap(rawItem)
		if len(item) == 0 {
			continue
		}
		switch utils.MapGetString(item, "type") {
		case "message":
			if outputText == "" {
				text, _ := extractResponsesTextAndReason(item)
				if text != "" {
					outWriter.Write([]byte(text))
				}
			}
			_, reason := extractResponsesTextAndReason(item)
			if reason != "" {
				reasonWriter.Write([]byte(reason))
			}
		case "reasoning":
			reason := extractResponsesReasoning(item)
			if reason != "" {
				reasonWriter.Write([]byte(reason))
			}
		case "function_call":
			tc := parseResponsesFunctionCall(item, index)
			if tc != nil {
				toolCalls = append(toolCalls, tc)
			}
		}
	}
	if len(toolCalls) > 0 {
		if toolCallCallback != nil {
			toolCallCallback(toolCalls)
		} else {
			for _, tc := range toolCalls {
				writeLegacyToolCall(outWriter, tc)
			}
		}
	}
}

func extractResponsesTextAndReason(item map[string]any) (string, string) {
	var textBuilder strings.Builder
	var reasonBuilder strings.Builder
	contentItems := utils.InterfaceToSliceInterface(item["content"])
	for _, rawContent := range contentItems {
		content := utils.InterfaceToGeneralMap(rawContent)
		if len(content) == 0 {
			continue
		}
		contentType := utils.MapGetString(content, "type")
		text := utils.MapGetString(content, "text")
		if text == "" {
			text = utils.MapGetString(content, "output_text")
		}
		if text == "" {
			continue
		}
		if strings.Contains(contentType, "reasoning") {
			reasonBuilder.WriteString(text)
			continue
		}
		if contentType == "" || strings.Contains(contentType, "text") {
			textBuilder.WriteString(text)
		}
	}
	return textBuilder.String(), reasonBuilder.String()
}

func extractResponsesReasoning(item map[string]any) string {
	if text := utils.MapGetString(item, "text"); text != "" {
		return text
	}
	var result strings.Builder
	summary := utils.InterfaceToSliceInterface(item["summary"])
	for _, raw := range summary {
		summaryItem := utils.InterfaceToGeneralMap(raw)
		if len(summaryItem) == 0 {
			continue
		}
		text := utils.MapGetString(summaryItem, "text")
		if text != "" {
			result.WriteString(text)
		}
	}
	return result.String()
}

func parseResponsesFunctionCall(item map[string]any, defaultIndex int) *ToolCall {
	name := utils.MapGetString(item, "name")
	args := utils.MapGetString(item, "arguments")
	if name == "" && args == "" {
		return nil
	}
	index := defaultIndex
	if raw, ok := item["output_index"]; ok {
		index = utils.InterfaceToInt(raw)
	} else if raw, ok := item["index"]; ok {
		index = utils.InterfaceToInt(raw)
	}
	id := utils.MapGetString(item, "call_id")
	if id == "" {
		id = utils.MapGetString(item, "id")
	}
	return &ToolCall{
		Index: index,
		ID:    id,
		Type:  "function",
		Function: FuncReturn{
			Name:      name,
			Arguments: args,
		},
	}
}

func writeLegacyToolCall(outWriter io.Writer, tc *ToolCall) {
	if tc == nil {
		return
	}
	if tc.Function.Name == "" {
		if tc.Function.Arguments != "" {
			outWriter.Write([]byte(tc.Function.Arguments))
		}
		return
	}
	funcMap := map[string]any{
		"name": tc.Function.Name,
	}
	if tc.Function.Arguments != "" {
		funcMap["arguments"] = tc.Function.Arguments
	}
	funcMapRaw, err := json.Marshal(funcMap)
	if err != nil {
		return
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
		return
	}
	outWriter.Write([]byte(callData))
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
