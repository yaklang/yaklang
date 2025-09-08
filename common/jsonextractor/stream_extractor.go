package jsonextractor

import (
	"bytes"
	"fmt"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yak/antlr4yak/yakvm/vmstack"
	"io"
	"unicode"
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

type callbackManager struct {
	objectKeyValueCallback      func(string string, data any)
	arrayValueCallback          func(idx int, data any)
	onRootMapCallback           func(i map[string]any)
	onArrayCallback             func(data []any)
	onObjectCallback            func(data map[string]any)
	onConditionalObjectCallback []*ConditionalCallback

	rawKVCallback func(key, data any)
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

func ExtractStructuredJSON(c string, options ...CallbackOption) error {
	return ExtractStructuredJSONFromStream(bytes.NewBufferString(c), options...)
}

func ExtractStructuredJSONFromStream(jsonReader io.Reader, options ...CallbackOption) error {
	callbackManager := &callbackManager{}
	for _, option := range options {
		option(callbackManager)
	}

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
	}

	peekUntil := func(checkFunc func(b byte) bool) (byte, error) { // peek until meet the conditions
		i := 0
		for {
			i++
			res, err := reader.PeekN(i)
			if err != nil {
				return 0, err
			}
			if len(res) < i {
				return 0, fmt.Errorf("invalid peek , want %d but got %d", i, len(res))
			}

			if checkFunc(res[i-1]) {
				return res[i-1], nil
			}
		}
	}

	peekUntilNoWhiteSpace := func() (byte, error) {
		return peekUntil(func(b byte) bool {
			return b != ' ' && b != '\t' && b != '\f' && b != '\v'
		})
	}

	bufManager := newBufStackManager(func(key any, val any) {
		callbackManager.kv(key, val)
	})

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
				//log.Infof("pop  state: %v, with data: %#v (start:%v end:%v), current-state: %v", raw.value, sliceValue, raw.start, raw.end, currentState())
				switch raw.value {
				case state_objectKey:
					bufManager.PushKey(sliceValue)
				case state_objectValue:
					if !raw.isObject && !raw.isArray {
						bufManager.PushValue(sliceValue)
					}
				case state_jsonArray:
					bufManager.PopContainer()
				case state_jsonObj:
					bufManager.PopContainer()
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
				popState()
			case ',': // if get ',' means has new array item, should push state
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
					continue
				}
				bufManager.PushKey(s.arrayCurrentKeyIndex)
				s.arrayCurrentKeyIndex++
				pushState(state_arrayItem)
				goto RETRY
			}
		case state_objectValue:
			switch ch {
			case '[':
				currentStateIns().isArray = true
				pushState(state_jsonArray)
			case '{':
				currentStateIns().isObject = true
				pushState(state_jsonObj)
				pushStateWithIdx(state_objectKey, index+1)
				continue
			case '"':
				if ret := currentStateIns(); ret != nil {
					if ret.objectValueHandledString {
						// 处理过了
						continue
					} else {
						ret.objectValueHandledString = true
					}
				}
				pushState(state_DoubleQuoteString)
				continue
			case '}':
				if !currentStateIns().objectValueInArray {
					popState()
					currentStateName := currentState()
					switch currentStateName {
					case state_jsonObj:
						popStateWithIdx(index + 1)
						continue
					}
					continue
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
				peekByte, err := peekUntilNoWhiteSpace()
				if err != nil {
					return err
				}
				if peekByte == '"' || peekByte == '\n' || peekByte == '\r' {
					popState()
					pushStateWithIdx(state_objectKey, index+1)
				}
				continue
			case ']':
				if currentStateIns().objectValueInArray {
					popStateWithIdx(index)
					currentStateName := currentState()
					switch currentStateName {
					case state_jsonArray:
						popStateWithIdx(index)
						continue
					}
					goto RETRY
				}
			}
		case state_objectKey:
			switch ch {
			case '"':
				pushState(state_DoubleQuoteString)
				continue
			case ':':
				popStateWithIdx(index)
				pushStateWithIdx(state_objectValue, index+1)
				continue
			case '}':
				popStateWithIdx(index - 1)
				if currentState() == state_jsonObj {
					popStateWithIdx(index)
					continue
				}
				continue
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
				pushState(state_jsonObj)
				continue
			case '"':
				pushState(state_DoubleQuoteString)
				continue
			//case '`':
			//	pushState(state_esExpr)
			//	continue
			case '}':
				popState()
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
				pushState(state_quote)
				continue
			case '"':
				popStateWithIdx(index + 1)
				continue
			}
		case state_quote:
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
