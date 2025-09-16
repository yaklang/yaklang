package jsonextractor

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/tidwall/gjson"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yak/antlr4yak/yakvm/vmstack"
)

var (
	reQuoted = regexp.MustCompile(`(?P<quoted>(\\x[0-9a-fA-F]{2}))`)
)

func FixJson(b []byte) []byte {
	// invalid character 'x' in string escape code
	b = reQuoted.ReplaceAllFunc(b, func(i []byte) []byte {
		raw, err := strconv.Unquote(`"` + string(i) + `"`)
		if err != nil || len(raw) <= 0 {
			return i
		}
		return []byte(fmt.Sprintf(`\u%04x`, raw[0]))
	})
	return b
}

func JsonValidObject(b []byte) ([]byte, bool) {
	if gjson.ValidBytes(b) {
		return b, true
	}

	r := gjson.ParseBytes(b)
	var buf []string
	if r.IsObject() {
		for k, v := range r.Map() {
			kJsonBytes, _ := json.Marshal(k)
			var kJson = string(kJsonBytes)
			if strings.HasPrefix(kJson, `"`) && strings.HasSuffix(kJson, `"`) {
				buf = append(buf, fmt.Sprintf(`%v: %s`, kJson, v.String()))
			} else {
				buf = append(buf, fmt.Sprintf(`"%v": %s`, kJson, v.String()))
			}
		}
	}

	if len(buf) > 0 {
		return []byte("{" + strings.Join(buf, ", ") + "}"), true
	}

	return nil, false
}

const (
	state_SingleQuoteString = "s-quote"
	state_DoubleQuoteString = "d-quote"
	state_BacktickString    = "b-quote"
	state_jsonObj           = "json-object"
	state_data              = "data"
	//state_esExpr            = "es-expr"
	state_reset = "reset"
	state_quote = "quote"

	// ex state
	state_objectKey      = "object-key"
	state_objectValue    = "object-value"
	state_jsonArray      = "json-array"
	state_arrayItem      = `json-array-item`
	state_primitiveValue = "primitive-value"
)

func ExtractObjectIndexes(c string) [][2]int {
	scanner := bufio.NewScanner(bytes.NewBufferString(c))
	scanner.Split(bufio.ScanBytes)

	var index = -1
	var objectDepth = 0
	var objectDepthIndexTable = make(map[int]int)
	var arrayDepth = 0
	var arrayDepthIndexTable = make(map[int]int)

	var results [][2]int
	stack := vmstack.New()
	pushState := func(i string) {
		if i == state_jsonObj {
			objectDepth++
			if _, existed := objectDepthIndexTable[objectDepth]; !existed {
				objectDepthIndexTable[objectDepth] = index
			}
		} else if i == state_jsonArray {
			arrayDepth++
			if _, existed := arrayDepthIndexTable[arrayDepth]; !existed {
				arrayDepthIndexTable[arrayDepth] = index
			}
		}
		stack.Push(i)
	}
	popState := func() {
		r := stack.Pop()
		if r != nil {
			raw, ok := r.(string)
			if ok && raw == state_jsonObj {
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
			} else if ok && raw == state_jsonArray {
				// 记录数组结果
				ret, ok := arrayDepthIndexTable[arrayDepth]
				if ok && ret >= 0 {
					results = append(results, [2]int{arrayDepthIndexTable[arrayDepth], index + 1})
				}
				delete(arrayDepthIndexTable, arrayDepth)
				if arrayDepth == 0 {
					arrayDepthIndexTable = make(map[int]int)
				}
				arrayDepth--
			}
		}
	}
	currentState := func() string {
		basicState := stack.Peek()
		if basicState == nil {
			return state_reset
		}
		return basicState.(string)
	}

	// 启动栈状态机
	pushState(state_data)
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

		switch currentState() {
		case state_data:
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
			case '[':
				pushState(state_jsonArray)
				continue
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
			case '[':
				pushState(state_jsonArray)
				continue
			case '}':
				popState()
				continue
			}
		case state_jsonArray:
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
			case '[':
				pushState(state_jsonArray)
				continue
			case ']':
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
				popState()
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

func ExtractJSONWithRaw(raw string) (results []string, rawStr []string) {
	defer func() {
		if err := recover(); err != nil {
			log.Errorf("extract json failed: %s", err)
		}
	}()
	var extraValid []string
	for _, obj := range ExtractObjectIndexes(raw) {
		jsonStr := raw[obj[0]:obj[1]]
		if ret, ok := JsonValidObject([]byte(jsonStr)); ok {
			if !json.Valid([]byte(jsonStr)) {
				rawStr = append(rawStr, jsonStr)
				// 修复后的 JSON
				extraValid = append(extraValid, string(ret))
			} else {
				// 完美的 JSON
				results = append(results, jsonStr)
			}
		} else {
			rawStr = append(rawStr, jsonStr)
		}
	}
	if len(extraValid) > 0 {
		results = append(results, extraValid...)
	}
	return
}

// ExtractJSON 尝试提取字符串中的 JSON 并进行修复, 返回中的元素都是原始 Json
// Example:
// ```
// json.ExtractJson("hello yak") // []
// res = json.ExtractJson(`[{"hello": "yak"}]`) // [[{"key": "value"}]]
// assert(res[0]==`[{"key": "value"}]`)
// ```
func ExtractStandardJSON(raw string) []string {
	jsonStr, _ := ExtractJSONWithRaw(raw)
	return jsonStr
}

// ExtractObjectsOnly 从输入中提取所有对象, 为了保持兼容性
// 不管输入是对象、数组还是混合文本，最终只返回对象
func ExtractObjectsOnly(raw string) []string {
	defer func() {
		if err := recover(); err != nil {
			log.Errorf("extract objects only failed: %s", err)
		}
	}()

	var results []string

	// 首先提取所有的 JSON 结构
	for _, obj := range ExtractObjectIndexes(raw) {
		jsonStr := raw[obj[0]:obj[1]]

		// 尝试解析为 JSON
		var jsonBytes []byte
		var valid bool

		if json.Valid([]byte(jsonStr)) {
			jsonBytes = []byte(jsonStr)
			valid = true
		} else {
			// 尝试修复 JSON
			if ret, ok := JsonValidObject([]byte(jsonStr)); ok {
				jsonBytes = ret
				valid = true
			}
		}

		if !valid {
			continue
		}

		// 使用 gjson 解析
		result := gjson.ParseBytes(jsonBytes)

		// 如果是对象，直接添加
		if result.IsObject() {
			results = append(results, result.Raw)
		} else if result.IsArray() {
			// 如果是数组，提取其中的对象
			result.ForEach(func(key, value gjson.Result) bool {
				if value.IsObject() {
					// 将对象转换为字符串
					objStr := value.Raw
					results = append(results, objStr)
				}
				return true // 继续遍历
			})
		}
		// 忽略其他类型（字符串、数字、布尔值等）
	}

	return results
}
