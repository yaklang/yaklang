package jsonextractor

import (
	"bytes"
	"io"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils/bufpipe"
	"github.com/yaklang/yaklang/common/yak/antlr4yak/yakvm/vmstack"
)

type ConditionalCallback struct {
	condition []string
	callback  func(data map[string]any)
}

func (c *ConditionalCallback) Feed(data map[string]any) {
	if c == nil || c.condition == nil || data == nil {
		return
	}

	for _, v := range c.condition {
		if _, existed := data[v]; !existed {
			return
		}
	}
	if c.callback == nil {
		return
	}
	c.callback(data)
}

// FieldMatchType 字段匹配类型
type FieldMatchType int

const (
	FieldMatchExact  FieldMatchType = iota // 精确匹配
	FieldMatchMulti                        // 多字段匹配（任意一个匹配即可）
	FieldMatchRegexp                       // 正则表达式匹配
	FieldMatchGlob                         // Glob模式匹配
)

// FieldStreamHandler 字段流处理器
type FieldStreamHandler struct {
	// 匹配相关
	matchType  FieldMatchType // 匹配类型
	pattern    string         // 匹配模式：可以是字段名、正则表达式或glob模式
	fieldNames []string       // 多字段匹配时使用

	syncHandler func(key string, reader io.Reader, parents []string)
	// 统一的回调函数
	handler func(key string, reader io.Reader, parents []string) // 回调函数，包含字段名和父路径
}

type fieldStreamContext struct {
	key    string
	writer io.WriteCloser
}

type fieldStreamFrame struct {
	contexts []*fieldStreamContext
}

type callbackManager struct {
	objectKeyValueCallback      func(string string, data any)
	arrayValueCallback          func(idx int, data any)
	onRootMapCallback           func(i map[string]any)
	onArrayCallback             func(data []any)
	onObjectCallback            func(data map[string]any)
	onConditionalObjectCallback []*ConditionalCallback

	fieldStreamHandlers []*FieldStreamHandler

	rawKVCallback    func(key, data any)
	formatKVCallback func(key, value any, parents []string)

	// 字段流处理相关
	activeWriters         []io.WriteCloser    // 当前活跃的写入器列表，支持多字段同时写入
	fieldStreamFrameStack []*fieldStreamFrame // 字段流写入栈，用于支持嵌套结构
}

type CallbackOption func(*callbackManager)

func WithObjectKeyValue(callback func(string string, data any)) CallbackOption {
	return func(c *callbackManager) {
		c.objectKeyValueCallback = callback
	}
}

func WithRawKeyValueCallback(callback func(key, data any)) CallbackOption {
	return func(c *callbackManager) {
		c.rawKVCallback = callback
	}
}

func WithFormatKeyValueCallback(callback func(key, data any, parents []string)) CallbackOption {
	return func(c *callbackManager) {
		c.formatKVCallback = callback
	}
}

func WithArrayCallback(callback func(data []any)) CallbackOption {
	return func(c *callbackManager) {
		c.onArrayCallback = callback
	}
}

func WithRegisterConditionalObjectCallback(key []string, callback func(data map[string]any)) CallbackOption {
	return func(c *callbackManager) {
		if c.onConditionalObjectCallback == nil {
			c.onConditionalObjectCallback = make([]*ConditionalCallback, 0)
		}
		c.onConditionalObjectCallback = append(c.onConditionalObjectCallback, &ConditionalCallback{
			condition: key,
			callback:  callback,
		})
	}
}

func WithObjectCallback(callback func(data map[string]any)) CallbackOption {
	return func(c *callbackManager) {
		c.onObjectCallback = callback
	}
}

func WithRootMapCallback(callback func(data map[string]any)) CallbackOption {
	return func(c *callbackManager) {
		c.onRootMapCallback = callback
	}
}

// WithRegisterFieldStreamHandler 注册字段流处理器
func WithRegisterFieldStreamHandler(fieldName string, handler func(key string, reader io.Reader, parents []string)) CallbackOption {
	return func(c *callbackManager) {
		if c.fieldStreamHandlers == nil {
			c.fieldStreamHandlers = make([]*FieldStreamHandler, 0)
		}
		c.fieldStreamHandlers = append(c.fieldStreamHandlers, &FieldStreamHandler{
			matchType: FieldMatchExact,
			pattern:   fieldName,
			handler:   handler,
		})
	}
}

// WithRegisterMultiFieldStreamHandler 注册多字段流处理器
func WithRegisterMultiFieldStreamHandler(fieldNames []string, handler func(key string, reader io.Reader, parents []string)) CallbackOption {
	return func(c *callbackManager) {
		if c.fieldStreamHandlers == nil {
			c.fieldStreamHandlers = make([]*FieldStreamHandler, 0)
		}
		c.fieldStreamHandlers = append(c.fieldStreamHandlers, &FieldStreamHandler{
			matchType:  FieldMatchMulti,
			fieldNames: fieldNames,
			handler:    handler,
		})
	}
}

// WithRegisterRegexpFieldStreamHandler 注册正则表达式字段流处理器
func WithRegisterRegexpFieldStreamHandler(pattern string, handler func(key string, reader io.Reader, parents []string)) CallbackOption {
	return func(c *callbackManager) {
		if c.fieldStreamHandlers == nil {
			c.fieldStreamHandlers = make([]*FieldStreamHandler, 0)
		}
		c.fieldStreamHandlers = append(c.fieldStreamHandlers, &FieldStreamHandler{
			matchType: FieldMatchRegexp,
			pattern:   pattern,
			handler:   handler,
		})
	}
}

// WithRegisterGlobFieldStreamHandler 注册Glob模式字段流处理器
func WithRegisterGlobFieldStreamHandler(pattern string, handler func(key string, reader io.Reader, parents []string)) CallbackOption {
	return func(c *callbackManager) {
		if c.fieldStreamHandlers == nil {
			c.fieldStreamHandlers = make([]*FieldStreamHandler, 0)
		}
		c.fieldStreamHandlers = append(c.fieldStreamHandlers, &FieldStreamHandler{
			matchType: FieldMatchGlob,
			pattern:   pattern,
			handler:   handler,
		})
	}
}

func WithRegisterFieldStreamHandlerAndStartCallback(fieldName string, handler func(key string, reader io.Reader, parents []string), callback func(key string, reader io.Reader, parents []string)) CallbackOption {
	return func(c *callbackManager) {
		if c.fieldStreamHandlers == nil {
			c.fieldStreamHandlers = make([]*FieldStreamHandler, 0)
		}
		c.fieldStreamHandlers = append(c.fieldStreamHandlers, &FieldStreamHandler{
			matchType:   FieldMatchExact,
			pattern:     fieldName,
			handler:     handler,
			syncHandler: callback,
		})
	}
}

// handleFieldStreamStart 开始字段流处理
func (c *callbackManager) handleFieldStreamStart(fieldName string, bufManager *bufStackManager) []*fieldStreamContext {
	// 清理字段名中的引号和空格
	cleanFieldName := strings.Trim(strings.TrimSpace(fieldName), `"`)

	// 从stack获取父路径
	var prefix []string
	if bufManager != nil {
		prefix = bufManager.getPrefixKey()
	}

	var contexts []*fieldStreamContext

	// 检查所有字段处理器
	if c.fieldStreamHandlers != nil {
		for _, handler := range c.fieldStreamHandlers {
			if c.isFieldMatch(cleanFieldName, handler) {
				ctx := c.createFieldStream(cleanFieldName, handler, prefix)
				if ctx != nil {
					contexts = append(contexts, ctx)
				}
			}
		}
	}

	return contexts
}

// isFieldMatch 检查字段是否匹配处理器
func (c *callbackManager) isFieldMatch(fieldName string, handler *FieldStreamHandler) bool {
	switch handler.matchType {
	case FieldMatchExact:
		return handler.pattern == fieldName
	case FieldMatchMulti:
		return matchAnyOfSubString(fieldName, handler.fieldNames...)
	case FieldMatchRegexp:
		return matchRegexp(fieldName, handler.pattern)
	case FieldMatchGlob:
		return matchGlob(fieldName, handler.pattern)
	default:
		return false
	}
}

// matchAnyOfSubString 检查字符串是否包含任意一个子串
func matchAnyOfSubString(s string, subStrings ...string) bool {
	s = strings.ToLower(s)
	for _, sub := range subStrings {
		if strings.Contains(s, strings.ToLower(sub)) {
			return true
		}
	}
	return false
}

// matchRegexp 检查字符串是否匹配正则表达式
func matchRegexp(s string, pattern string) bool {
	matched, err := regexp.MatchString(pattern, s)
	if err != nil {
		return false
	}
	return matched
}

// matchGlob 检查字符串是否匹配Glob模式
func matchGlob(s string, pattern string) bool {
	matched, err := filepath.Match(pattern, s)
	if err != nil {
		return false
	}
	return matched
}

// createFieldStream 创建字段流
func (c *callbackManager) createFieldStream(fieldName string, handler *FieldStreamHandler, parents []string) *fieldStreamContext {
	// 创建管道
	reader, writer := bufpipe.NewPipe()

	if handler.syncHandler != nil {
		// 调用开始回调函数 用于强同步
		handler.syncHandler(fieldName, reader, parents)
	}

	// 在新的 goroutine 中调用处理函数
	go func(h *FieldStreamHandler, r io.Reader, key string, parentPath []string) {
		defer func() {
			if err := recover(); err != nil {
				log.Errorf("field stream handler panic: %v", err)
			}
		}()

		// 调用统一的回调函数
		if h.handler != nil {
			h.handler(key, r, parentPath)
		}
	}(handler, reader, fieldName, parents)

	log.Infof("started field stream for: %s", fieldName)
	return &fieldStreamContext{
		key:    fieldName,
		writer: writer,
	}
}

// handleFieldStreamData 写入字段流数据
func (c *callbackManager) handleFieldStreamData(fieldName string, data []byte) {
	cleanFieldName := strings.Trim(strings.TrimSpace(fieldName), `"`)
	for _, frame := range c.fieldStreamFrameStack {
		for _, ctx := range frame.contexts {
			if ctx == nil || ctx.writer == nil {
				continue
			}
			if ctx.key != cleanFieldName {
				continue
			}
			if _, err := ctx.writer.Write(data); err != nil {
				log.Errorf("failed to write field stream data for %s: %v", cleanFieldName, err)
			}
		}
	}
}

func (c *callbackManager) pushFieldStreamFrame(contexts []*fieldStreamContext) *fieldStreamFrame {
	if len(contexts) == 0 {
		return nil
	}
	frame := &fieldStreamFrame{
		contexts: contexts,
	}
	c.fieldStreamFrameStack = append(c.fieldStreamFrameStack, frame)
	for _, ctx := range contexts {
		if ctx != nil && ctx.writer != nil {
			c.activeWriters = append(c.activeWriters, ctx.writer)
		}
	}
	return frame
}

func (c *callbackManager) popFieldStreamFrame(frame *fieldStreamFrame) {
	if frame == nil {
		return
	}
	idx := -1
	for i := len(c.fieldStreamFrameStack) - 1; i >= 0; i-- {
		if c.fieldStreamFrameStack[i] == frame {
			idx = i
			break
		}
	}
	if idx == -1 {
		return
	}

	// 移除frame
	c.fieldStreamFrameStack = append(c.fieldStreamFrameStack[:idx], c.fieldStreamFrameStack[idx+1:]...)

	// 重建active writers列表
	c.activeWriters = nil
	for _, frm := range c.fieldStreamFrameStack {
		for _, ctx := range frm.contexts {
			if ctx != nil && ctx.writer != nil {
				c.activeWriters = append(c.activeWriters, ctx.writer)
			}
		}
	}

	// 关闭当前frame的writer
	for _, ctx := range frame.contexts {
		if ctx != nil && ctx.writer != nil {
			ctx.writer.Close()
			log.Infof("ended field stream for: %s", ctx.key)
		}
	}
}

func (c *callbackManager) resetFieldStreamFrames() {
	for len(c.fieldStreamFrameStack) > 0 {
		frame := c.fieldStreamFrameStack[len(c.fieldStreamFrameStack)-1]
		c.popFieldStreamFrame(frame)
	}
}

// setCurrentFieldWriter 设置当前字段写入器（已废弃，保留兼容性）
func (c *callbackManager) setCurrentFieldWriter(fieldName string) {
	// 在新的多写入器架构中，此方法不再需要
}

// clearCurrentFieldWriter 清除当前字段写入器（已废弃，保留兼容性）
func (c *callbackManager) clearCurrentFieldWriter() {
	// 在新的多写入器架构中，此方法不再需要
}

func ExtractStructuredJSON(c string, options ...CallbackOption) error {
	return ExtractStructuredJSONFromStream(bytes.NewBufferString(c), options...)
}

func ExtractStructuredJSONFromStream(jsonReader io.Reader, options ...CallbackOption) error {
	callbackManager := &callbackManager{}
	for _, option := range options {
		option(callbackManager)
	}
	defer callbackManager.resetFieldStreamFrames()

	var mirror = new(bytes.Buffer)
	reader := newAutoPeekReader(io.TeeReader(jsonReader, mirror))

	getMirrorBytes := func() string {
		return mirror.String()
	}

	var index = -1
	var objectDepth = 0
	var objectDepthIndexTable = make(map[int]int)

	var results [][2]int
	stack := vmstack.New()

	type state struct {
		value string
		start int
		end   int

		isObject                 bool
		isArray                  bool
		objectValueHandledString bool
		objectValueInArray       bool
		arrayCurrentKeyIndex     int
		legalArrayItem           bool
		fieldStreamFrame         *fieldStreamFrame
	}

	bufManager := newBufStackManager(func(key any, val any, parents []string) {
		callbackManager.kv(key, val, parents)
	})
	bufManager.setCallbackManager(callbackManager)

	pushStateWithIdx := func(i string, idx int) {
		//log.Infof("push state: %v, with index: %v", i, index)
		if i == state_jsonObj {
			bufManager.PushContainer()
			objectDepth++
			if _, existed := objectDepthIndexTable[objectDepth]; !existed {
				objectDepthIndexTable[objectDepth] = index
			}
		} else if i == state_jsonArray {
			bufManager.PushContainer()
		}
		stack.Push(&state{
			value: i,
			start: idx,
			end:   idx,
		})
	}
	currentState := func() string {
		basicState := stack.Peek()
		if basicState == nil {
			return state_reset
		}
		return basicState.(*state).value
	}
	currentStateIns := func() *state {
		basicState := stack.Peek()
		return basicState.(*state)
	}

	getStrSlice := func(s *state) string {
		if s.start > s.end {
			return ""
		}
		if s.start == s.end {
			return ""
		}
		c := getMirrorBytes()
		if s.end >= len(c) {
			s.end = len(c) - 1
		}
		return c[s.start:s.end]
	}
	_ = currentStateIns
	popStateWithIdx := func(idx int) {
		r := stack.Pop()
		if r != nil {
			raw, ok := r.(*state)
			if ok {
				raw.end = idx
				c := getMirrorBytes()
				if raw.end >= len(c) {
					raw.end = len(c) - 1
				}
				sliceValue := getStrSlice(raw)
				if raw.value == state_DoubleQuoteString {
					if parentRaw := stack.Peek(); parentRaw != nil {
						if parentState, ok := parentRaw.(*state); ok && parentState.value == state_objectKey {
							bufManager.prepareFieldStreamContexts(sliceValue)
						}
					}
				}
				//log.Infof("pop  state: %v, with data: %#v (start:%v end:%v), current-state: %v", raw.value, sliceValue, raw.start, raw.end, currentState())
				switch raw.value {
				case state_objectKey:
					bufManager.PushKey(sliceValue)
				case state_objectValue:
					if !raw.isObject && !raw.isArray {
						bufManager.PushValue(sliceValue)
					}
					// 字段值处理完成，清理当前活跃的写入器
					if raw.fieldStreamFrame != nil && bufManager.callbackManager != nil {
						bufManager.callbackManager.popFieldStreamFrame(raw.fieldStreamFrame)
						raw.fieldStreamFrame = nil
					}
				case state_jsonArray:
					bufManager.PopContainer()
					// 数组处理完成，清理当前活跃的写入器
					if raw.fieldStreamFrame != nil && bufManager.callbackManager != nil {
						bufManager.callbackManager.popFieldStreamFrame(raw.fieldStreamFrame)
						raw.fieldStreamFrame = nil
					}
				case state_jsonObj:
					bufManager.PopContainer()
					// 对象处理完成，清理当前活跃的写入器
					if raw.fieldStreamFrame != nil && bufManager.callbackManager != nil {
						bufManager.callbackManager.popFieldStreamFrame(raw.fieldStreamFrame)
						raw.fieldStreamFrame = nil
					}
					// 记录结果
					ret, ok := objectDepthIndexTable[objectDepth]
					if ok && ret >= 0 {
						results = append(results, [2]int{objectDepthIndexTable[objectDepth], index + 1})
					}
					delete(objectDepthIndexTable, objectDepth)
					if objectDepth == 0 {
						objectDepthIndexTable = make(map[int]int)
					}
					objectDepth--
				case state_arrayItem:
					if !raw.legalArrayItem {
						bufManager.PushValue(sliceValue)
					}
				}

			}
		}
	}
	lastState := func() string {
		basicState := stack.Peek()
		if basicState == nil {
			return state_reset
		}
		return basicState.(*state).value
	}
	_ = lastState

	// 启动栈状态机
	pushStateWithIdx(state_data, 0)
	var ch byte
	for {
		var results = make([]byte, 1)
		n, err := io.ReadFull(reader, results)
		if n <= 0 && err != nil {
			if err == io.EOF {
				return nil
			}
			log.Errorf("parse json stream failed: %v", err)
			return err
		}
		results = results[:n]
		index++
		if len(results) <= 0 {
			break
		}
		ch = results[0]

		pushState := func(i string) {
			pushStateWithIdx(i, index)
		}
		popState := func() {
			popStateWithIdx(index)
		}

		// 处理字符级流式写入
		writeToFieldStream := func() {
			if len(callbackManager.activeWriters) > 0 {
				data := []byte{ch}
				for _, writer := range callbackManager.activeWriters {
					if writer != nil {
						_, err := writer.Write(data)
						if err != nil {
							log.Errorf("failed to write character to field stream: %v", err)
						}
					}
				}
			}
		}
	RETRY:
		switch currentState() {
		case state_arrayItem:
			if unicode.IsSpace(rune(ch)) {
				continue
			}
			if ch == ',' || ch == ']' {
				popState()
				goto RETRY // array item not consume ',' and ']'
			}
			currentStateIns().legalArrayItem = true
			popState()
			pushState(state_objectValue)
			currentStateIns().objectValueInArray = true
			goto RETRY
		case state_jsonArray:
			s := currentStateIns()
			switch ch {
			case ']':
				writeToFieldStream() // 写入结束括号
				popState()
			case ',': // if get ',' means has new array item, should push state
				writeToFieldStream()             // 写入逗号
				if s.arrayCurrentKeyIndex == 0 { // if get ',' and index == 0 ,should consume it. push 0:""
					bufManager.PushKey(s.arrayCurrentKeyIndex)
					s.arrayCurrentKeyIndex++
					bufManager.PushValue("")
				}
				bufManager.PushKey(s.arrayCurrentKeyIndex)
				s.arrayCurrentKeyIndex++
				pushStateWithIdx(state_arrayItem, index+1) // item should not contains this comma
			default:
				if unicode.IsSpace(rune(ch)) {
					writeToFieldStream() // 写入空白字符
					continue
				}
				bufManager.PushKey(s.arrayCurrentKeyIndex)
				s.arrayCurrentKeyIndex++
				pushState(state_arrayItem)
				goto RETRY
			}
		case state_objectValue:
			switch ch {
			case ' ', '\t', '\r':
				writeToFieldStream() // 写入空白字符
				continue
			case '[':
				currentStateIns().isArray = true
				// 激活待处理的字段流写入器，用于处理数组类型的值
				frame := bufManager.activatePendingFieldWriter()
				writeToFieldStream() // 写入开始括号
				pushState(state_jsonArray)
				if frame != nil {
					currentStateIns().fieldStreamFrame = frame
				}
			case '{':
				currentStateIns().isObject = true
				// 激活待处理的字段流写入器，用于处理对象类型的值
				frame := bufManager.activatePendingFieldWriter()
				writeToFieldStream() // 写入开始大括号
				pushState(state_jsonObj)
				if frame != nil {
					currentStateIns().fieldStreamFrame = frame
				}
				pushStateWithIdx(state_objectKey, index+1)
				continue
			case '"':
				if ret := currentStateIns(); ret != nil {
					if ret.objectValueHandledString {
						// 处理过了
						writeToFieldStream() // 写入引号字符
						continue
					} else {
						ret.objectValueHandledString = true
						// 激活待处理的字段流写入器
						frame := bufManager.activatePendingFieldWriter()
						ret.fieldStreamFrame = frame
						writeToFieldStream() // 写入开始引号
						pushState(state_DoubleQuoteString)
						continue
					}
				}
				// 如果没有激活字段流写入器，正常处理
				pushState(state_DoubleQuoteString)
				continue
			case '}':
				if !currentStateIns().objectValueInArray {
					popState()
					goto RETRY
				}
			case '\n':
				popState()
				if currentState() == state_jsonArray {

				} else {
					pushStateWithIdx(state_objectKey, index+1)
				}
			case ',':
				if currentStateIns().objectValueInArray {
					popState()
					goto RETRY
				}
				// Check if we're still inside a composite value (object/array)
				// If the previous state was jsonObj or jsonArray, we should have handled the comma there
				// This comma is a separator between object values, don't write it to field stream
				// Always transition to objectKey after comma in object value
				// This handles both normal cases and error recovery (e.g., consecutive commas)
				// The peek was removed to fix infinite loop - we always transition now
				popState()
				pushStateWithIdx(state_objectKey, index+1)
				goto RETRY
			case ']':
				if currentStateIns().objectValueInArray {
					writeToFieldStream()
					popStateWithIdx(index)
					currentStateName := currentState()
					switch currentStateName {
					case state_jsonArray:
						popStateWithIdx(index)
						continue
					}
					goto RETRY
				}
			default:
				// 处理数字、布尔值、null等其他类型
				if unicode.IsDigit(rune(ch)) || ch == '-' || ch == 't' || ch == 'f' || ch == 'n' {
					// 激活待处理的字段流写入器，用于处理数字、布尔值、null等类型的值
					frame := bufManager.activatePendingFieldWriter()
					currentStateIns().fieldStreamFrame = frame
					writeToFieldStream() // 写入当前字符
					pushState(state_primitiveValue)
					continue
				}
				continue
			}
		case state_objectKey:
			switch ch {
			case '"':
				writeToFieldStream() // 写入键名起始引号
				pushState(state_DoubleQuoteString)
				continue
			case ':':
				writeToFieldStream() // 写入键值分隔符
				popStateWithIdx(index)
				pushStateWithIdx(state_objectValue, index+1)
				continue
			case ',':
				// Check if we're inside a composite value by looking at parent state (2nd element on stack)
				// If parent is jsonObj/jsonArray, comma is part of the value and should be written
				if parentRaw := stack.PeekN(2); parentRaw != nil {
					if parentState, ok := parentRaw.(*state); ok {
						if parentState.value == state_jsonObj || parentState.value == state_jsonArray {
							writeToFieldStream() // 写入逗号（作为复合值的一部分）
							continue
						}
					}
				}
				// Otherwise, this comma is a separator between key-value pairs in the outer object
				// Pop state_objectKey, then let state_jsonObj handle the comma and push state_objectKey
				popState()
				// The comma will be handled by state_jsonObj in the next iteration
				// After that, we need to push state_objectKey to handle the next key
				// But we can't do that here because we're in state_objectKey switch
				// So we just goto RETRY and let state_jsonObj handle it
				goto RETRY
			case '}':
				writeToFieldStream() // 写入对象结束符，处理空对象
				popStateWithIdx(index - 1)
				if currentState() == state_jsonObj {
					popStateWithIdx(index)
					continue
				}
				continue
			}
		case state_primitiveValue:
			// 处理数字、布尔值、null等基本类型的值
			switch ch {
			case ',', '}', ']', '\n', '\r', '\t', ' ':
				// 遇到结束符，退出基本值处理状态
				popState()
				goto RETRY
			default:
				writeToFieldStream() // 写入当前字符
			}
		case state_data:
			switch ch {
			case '{':
				pushState(state_jsonObj)
				pushStateWithIdx(state_objectKey, index+1)
				continue
			case '"':
				pushState(state_DoubleQuoteString)
				continue
			case '[':
				currentStateIns().isArray = true
				pushState(state_jsonArray)
			}
		case state_jsonObj:
			switch ch {
			case '{':
				writeToFieldStream() // 写入嵌套对象开始大括号
				pushState(state_jsonObj)
				continue
			case '"':
				writeToFieldStream() // 写入引号
				pushState(state_DoubleQuoteString)
				continue
			//case '`':
			//	pushState(state_esExpr)
			//	continue
			case '}':
				writeToFieldStream() // 写入结束大括号
				popState()
				continue
			case ',':
				writeToFieldStream() // 写入逗号（作为复合值的一部分）
				// After comma in jsonObj, we need to push objectKey to handle the next key
				// But only if we're not already in objectKey (which would mean we're at top level)
				if currentState() != state_objectKey {
					pushStateWithIdx(state_objectKey, index+1)
				}
				continue
			case ':', ' ', '\t', '\n', '\r':
				writeToFieldStream() // 写入分隔符和空白字符
				continue
			default:
				writeToFieldStream() // 写入其他字符
				continue
			}
		//case state_esExpr:
		//	switch ch {
		//	case '}':
		//		popState()
		//		continue
		//	}
		case state_DoubleQuoteString:
			switch ch {
			case '\\':
				writeToFieldStream() // 写入转义字符
				pushState(state_quote)
				continue
			case '"':
				// 检查下一个字符是否是有效的JSON结构字符（表示字符串结束）
				// 如果是逗号、冒号、右大括号、右方括号、空格、制表符、换行符等，则字符串结束
				// 如果下一个字符是点号，再检查点号后面是否是有效结束字符
				// 否则，这个引号可能是字符串内容的一部分（非标准JSON，但需要容错处理）
				nextCh, peekErr := reader.Peek()
				isValidEndChar := false
				if peekErr == nil {
					// 有效的结束字符：逗号、冒号、右大括号、右方括号、空格、制表符、换行符、回车符
					if nextCh == ',' || nextCh == ':' || nextCh == '}' || nextCh == ']' ||
						nextCh == ' ' || nextCh == '\t' || nextCh == '\n' || nextCh == '\r' {
						isValidEndChar = true
					} else if nextCh == '.' {
						// 如果下一个字符是点号，检查点号后面是否是有效结束字符
						peek2, peekErr2 := reader.PeekN(2)
						if peekErr2 == nil && len(peek2) >= 2 {
							afterDot := peek2[1]
							if afterDot == ',' || afterDot == ':' || afterDot == '}' || afterDot == ']' ||
								afterDot == ' ' || afterDot == '\t' || afterDot == '\n' || afterDot == '\r' {
								isValidEndChar = true
							}
						}
					}
				} else if peekErr == io.EOF {
					// 到达文件末尾，字符串结束
					isValidEndChar = true
				}

				if isValidEndChar {
					writeToFieldStream() // 写入结束引号
					// 清除当前字段写入器
					callbackManager.clearCurrentFieldWriter()
					popStateWithIdx(index + 1)
					continue
				} else {
					// 这不是字符串结束，继续作为字符串内容处理
					writeToFieldStream() // 写入引号字符（作为字符串内容）
					continue
				}
			default:
				writeToFieldStream() // 写入普通字符
			}
		case state_quote:
			writeToFieldStream() // 写入被转义的字符
			popState()
			continue
		//case state_BacktickString:
		//	/*
		//		这个很特殊，有几种情况需要处理
		//		`abc`
		//		`abc${"123" + `abc`}`
		//	*/
		//	switch ch {
		//	case '{':
		//		if last == '$' {
		//			// ${ 开头的，认为这是 expr
		//			pushState(state_esExpr)
		//			continue
		//		}
		//	case '`':
		//		if last != '\\' {
		//			popState()
		//			continue
		//		}
		//	}
		case state_reset:
			// 空状态回溯，多半是有问题的
			//currentPair[0] = -1
			//currentPair[1] = -1
			//currentPair[2] = -1
			pushState(state_data)
		}
	}

	return nil
}
