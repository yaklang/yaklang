package jsonextractor

import (
	"bytes"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yak/antlr4yak/yakvm/vmstack"
	"io"
)

type callbackManager struct {
	objectKeyValueCallback func(string string, data any)
	arrayValueCallback     func(idx int, data any)
	onRootMapCallback      func(i map[string]any)
	onArrayCallback        func(data []any)
	onObjectCallback       func(data map[string]any)

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
		if err != nil {
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
		case state_jsonArray:
			switch ch {
			case ']':
				popState()
			default:
				s := currentStateIns()
				bufManager.PushKey(s.arrayCurrentKeyIndex)
				s.arrayCurrentKeyIndex++
				pushState(state_objectValue)
				currentStateIns().objectValueInArray = true
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
			case '\'':
				pushState(state_SingleQuoteString)
				continue
			case '}':
				popState()
				currentStateName := currentState()
				switch currentStateName {
				case state_jsonObj:
					popStateWithIdx(index + 1)
					continue
				}
				continue
			case '\n':
				popState()
				pushStateWithIdx(state_objectKey, index+1)
			case ',':
				popState()
				if currentState() == state_jsonArray {
					// 数组中，继续处理
					s := currentStateIns()
					bufManager.PushKey(s.arrayCurrentKeyIndex)
					s.arrayCurrentKeyIndex++
					pushStateWithIdx(state_objectValue, index+1)
					currentStateIns().objectValueInArray = true
					continue
				}
				pushStateWithIdx(state_objectKey, index+1)
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
			case '\'':
				pushState(state_SingleQuoteString)
				continue
				//case '`':
				//	pushState(state_esExpr)
				//	continue
			}
		case state_jsonObj:
			switch ch {
			case '{':
				pushState(state_jsonObj)
				continue
			case '"':
				pushState(state_DoubleQuoteString)
				continue
			case '\'':
				pushState(state_SingleQuoteString)
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
		case state_SingleQuoteString:
			switch ch {
			case '\\':
				pushState(state_quote)
				continue
			case '\'':
				popState()
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
