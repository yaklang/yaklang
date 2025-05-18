package jsonextractor

import (
	"bufio"
	"bytes"
	"encoding/json"
	"sort"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yak/antlr4yak/yakvm/vmstack"
)

type callbackManager struct {
	kvCallback func(key, data any)
}

func (c *callbackManager) kv(key, data any) {
	if c.kvCallback != nil {
		c.kvCallback(key, data)
	} else {
		log.Infof("kv callback is not set, key: %v, data: %v", key, data)
	}
}

type CallbackOption func(*callbackManager)

func WithKeyValueCallback(callback func(key, data any)) CallbackOption {
	return func(c *callbackManager) {
		c.kvCallback = callback
	}
}

func ExtractJSONStream(c string, options ...CallbackOption) [][2]int {
	callbackManager := &callbackManager{}
	for _, option := range options {
		option(callbackManager)
	}

	scanner := bufio.NewScanner(bytes.NewBufferString(c))
	scanner.Split(bufio.ScanBytes)

	var index = -1
	var objectDepth = 0
	var objectDepthIndexTable = make(map[int]int)

	var results [][2]int
	stack := vmstack.New()

	type state struct {
		value string
		start int
		end   int

		objectValueHandledString bool
	}

	bufManager := newBufStackManager(func(key any, val any) {
		callbackManager.kv(key, val)
	})

	pushStateWithIdx := func(i string, idx int) {
		log.Infof("push state: %v, with index: %v", i, index)
		if i == state_jsonObj {
			bufManager.PushContainer()
			objectDepth++
			if _, existed := objectDepthIndexTable[objectDepth]; !existed {
				objectDepthIndexTable[objectDepth] = index
			}
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
	_ = currentStateIns
	popStateWithIdx := func(idx int) {
		r := stack.Pop()
		if r != nil {
			raw, ok := r.(*state)
			if ok {
				raw.end = idx
				log.Infof("pop  state: %v, with data: %v (start:%v end:%v), current-state: %v", raw.value, c[raw.start:raw.end], raw.start, raw.end, currentState())
				switch raw.value {
				case state_objectKey:
					bufManager.PushKey(c[raw.start:raw.end])
				case state_objectValue:
					bufManager.PushValue(c[raw.start:raw.end])
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
		if !scanner.Scan() {
			break
		}
		index++
		results := scanner.Bytes()
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
		switch currentState() {
		case state_objectValue:
			switch ch {
			case '{':
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
				pushStateWithIdx(state_objectKey, index+1)
				continue
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

	// 收缩结果
	var blocks [][2]int
	var currentBlock = [2]int{-1, -1}
	sort.SliceStable(results, func(i, j int) bool {
		return results[i][0] < results[j][0]
	})
	currentBlockIsJson := func() bool {
		if currentBlock[0] < 0 {
			return false
		}
		return json.Valid([]byte(c[currentBlock[0]:currentBlock[1]]))
	}
	for _, result := range results {
		retRaw := c[result[0]:result[1]]
		_, isJson := JsonValidObject([]byte(retRaw))
		// fmt.Printf("%v: idx: %v json: %v\n", retRaw, result, isJson)
		if currentBlock[0] < 0 {
			currentBlock[0], currentBlock[1] = result[0], result[1]
			continue
		}

		if result[0] >= currentBlock[0] && result[1] <= currentBlock[1] && currentBlockIsJson() {
			// 被包含的内容
			continue
		} else {
			blocks = append(blocks, [2]int{currentBlock[0], currentBlock[1]})
			if isJson {
				currentBlock[0], currentBlock[1] = result[0], result[1]
			} else {
				blocks = append(blocks, [2]int{result[0], result[1]})
				currentBlock[0] = -1
				currentBlock[1] = -1
			}
		}
	}
	if currentBlock[0] < 0 {
		return blocks
	}
	return append(blocks, [2]int{currentBlock[0], currentBlock[1]})
}
