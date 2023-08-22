package fuzztag

import (
	"bytes"
	"fmt"
	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils/mixer"
	"regexp"
	"strconv"
	"strings"
	"sync"
)

type ExecutableNode interface {
	ToBytes() []byte
	Execute(map[string]func([]byte) [][]byte) [][]byte
}

// Nodes 一般是根结点
type Nodes struct {
	IsRoot   bool
	Nodes    []ExecutableNode
	AST      *FuzzTagAST
	InTag    bool
	callback func([]byte, [][]byte) bool
	payloads [][]byte
}

func (d *Nodes) SetRoot() {
	d.IsRoot = true
}

func (n *Nodes) SetPayloadCallback(cb func([]byte, [][]byte) bool) {
	n.callback = cb
}

func (d *Nodes) ToBytes() []byte {
	var buf bytes.Buffer
	for _, n := range d.Nodes {
		buf.Write(n.ToBytes())
	}
	return buf.Bytes()
}

func (d *Nodes) existedNewSymbol(s int) bool {
	if d == nil {
		return false
	}
	if d.AST == nil {
		return false
	}
	_, ok := d.AST.symbolTable[s]
	return ok
}

type DataPart struct {
	data [][]byte
	pos  int
}
type DataGenerator struct {
	data        []interface{}
	indexersMap map[string]int
	posIndex    int
	payloads    map[string][][]byte
}

func NewDataGenerator() *DataGenerator {
	return &DataGenerator{
		indexersMap: make(map[string]int),
		payloads:    make(map[string][][]byte),
	}
}

func (d *DataGenerator) pushTagData(name string, i [][]byte, payloads map[string][][]byte) {
	for k, v := range payloads {
		d.payloads[k] = v
	}
	if v, ok := d.indexersMap[name]; ok {
		d.data[v] = append(d.data[v].([]*DataPart), &DataPart{data: i, pos: d.posIndex})
		d.pushData(nil)
	} else {
		d.indexersMap[name] = d.posIndex
		d.pushData([]*DataPart{
			{data: i, pos: d.posIndex},
		})
	}
}
func (d *DataGenerator) pushData(i interface{}) {
	d.data = append(d.data, i)
	d.posIndex++
}
func (d *DataGenerator) generateBytes(callback func([]byte, [][]byte) bool) [][]byte {
	var result [][]byte

	var gen func(dataIndex int, allPartData [][]byte, payloads [][]byte)
	gen = func(dataIndex int, allPartData [][]byte, payloads [][]byte) {
		if dataIndex >= len(d.data) {
			res := bytes.Join(allPartData, []byte{})
			result = append(result, res)
			if callback != nil {
				callback(res, payloads)
			}
			payloads = [][]byte{}
			return
		}
		switch ret := d.data[dataIndex].(type) {
		case nil:
			gen(dataIndex+1, allPartData, payloads)
		case []byte:
			allPartData[dataIndex] = ret
			if payload, ok := d.payloads[string(ret)]; ok {
				payloads = append(payloads, payload...)
			}
			gen(dataIndex+1, allPartData, payloads)
		case []*DataPart:
			index := 0
			for {
				var ok bool
				var newPartData [][]byte = make([][]byte, d.posIndex)
				var newPayloads [][]byte = make([][]byte, len(payloads))
				copy(newPayloads, payloads)
				copy(newPartData, allPartData)
				for _, i := range ret {
					if index < len(i.data) {
						ok = true
						newPartData[i.pos] = i.data[index]
						newPayloads = append(newPayloads, i.data[index])
						if payload, ok := d.payloads[string(i.data[index])]; ok {
							newPayloads = append(newPayloads, payload...)
						}
					}
				}
				if !ok {
					break
				}
				gen(dataIndex+1, newPartData, newPayloads)
				index++
			}

		}
	}
	gen(0, make([][]byte, d.posIndex), [][]byte{})

	return result
}

func (d *Nodes) Execute(m map[string]func([]byte) [][]byte) [][]byte {
	g := NewDataGenerator()
	for _, node := range d.Nodes {
		if tNode, ok := node.(*TagNode); ok {
			name := tNode.Label
			if name == "" {
				name = fmt.Sprintf("__default__%s", uuid.New().String())
			}
			payloads1 := map[string][][]byte{} // 生成数据对应的payloads
			if nodes, ok := tNode.Params.(*Nodes); ok {
				payloads := map[string][][]byte{} // 生成数据对应的payloads
				nodes.callback = func(i []byte, i2 [][]byte) bool {
					payloads[string(i)] = i2
					return true
				}
				tNode.callback = func(i []byte, i2 [][]byte) bool {
					if v, ok := payloads[string(i)]; ok {
						for _, i3 := range i2 {
							payloads1[string(i3)] = v
						}
					}
					return true
				}
			}

			res := node.Execute(m)
			g.pushTagData(name, res, payloads1)
			//stableData = false
		} else {
			if dNode, ok := node.(*DataNode); ok {
				dNode.InParentTagNode = d.InTag
			}
			g.pushData(node.Execute(m)[0])
		}
	}
	return g.generateBytes(d.callback)
}

func (d *Nodes) Execute_(m map[string]func([]byte) [][]byte) [][]byte {
	if len(d.Nodes) <= 0 {
		if d.callback != nil {
			_ = d.callback(nil, nil)
		}
		return [][]byte{{}}
	}

	if len(d.Nodes) == 1 {
		node := d.Nodes[0]
		_, isTag := node.(*TagNode)

		if v, isData := node.(*DataNode); isData {
			v.InParentTagNode = d.InTag
		}
		results := node.Execute(m)
		if len(results) <= 0 {
			if d.callback != nil {
				_ = d.callback(nil, nil)
			}
			return [][]byte{{}}
		}
		if d.callback != nil {
			for _, r := range results {
				if isTag {
					_ = d.callback(r, [][]byte{r})
				} else {
					_ = d.callback(r, nil)
				}
			}
		}
		return results
	}

	var results [][][]byte
	var listIndex [][]string

	// 构建索引列表
	for nodeIndex, n := range d.Nodes {
		if v, isData := n.(*DataNode); isData {
			v.InParentTagNode = d.InTag
		}
		_ = nodeIndex
		executedResult := n.Execute(m)
		if len(executedResult) == 0 {
			executedResult = [][]byte{{}}
		}

		results = append(results, executedResult)
		length := len(executedResult)
		var l = make([]string, length)

		switch ret := n.(type) {
		case *DataNode:
			// 数据节点前缀有 $ 标志
			for i := range l {
				l[i] = "$" + fmt.Sprint(i)
			}
			listIndex = append(listIndex, l)
			continue
		case *TagNode:
			if ret.Symbol > 0 {
				if !d.existedNewSymbol(ret.Symbol) {
					// 新符号
					d.AST.symbolTable[ret.Symbol] = &SymbolContext{
						MaxLength: length,
					}
				} else {
					listIndex = append(listIndex, []string{"@" + fmt.Sprint(ret.Symbol)})
					continue
				}
				for i := range l {
					l[i] = fmt.Sprint(ret.Symbol) + "#" + fmt.Sprint(i)
				}
				listIndex = append(listIndex, l)
				continue
			}
		}
		// 默认笛卡尔乘积
		for i := range l {
			l[i] = fmt.Sprint(i)
		}
		listIndex = append(listIndex, l)
	}

	mx, err := mixer.NewMixer(listIndex...)
	if err != nil {
		log.Errorf("create mixer failed: %s", err)
		return nil
	}

	var r [][]byte
	for {
		values := mx.Value()
		var buf = bytes.NewBuffer(nil)
		buf.Reset()

		var payloads [][]byte
		for i, indexStr := range values {
			var isPayload = true
			var indexInt int
			var symbol int
			if strings.HasPrefix(indexStr, "$") {
				indexStr = indexStr[1:]
				isPayload = false
				indexInt, _ = strconv.Atoi(indexStr)
			} else if strings.HasPrefix(indexStr, "@") {
				indexStr = indexStr[1:]
				isPayload = false
				symbolId, _ := strconv.Atoi(indexStr)

				var payload []byte
				origin := results[i]
				offset := d.AST.symbolTable[symbolId].CurrentOffset - 1
				if len(origin) > offset {
					payload = origin[offset]
				}

				if isPayload {
					payloads = append(payloads, payload[:])
				}
				buf.Write(payload)
				continue
			} else if strings.Contains(indexStr, "#") {
				ret := strings.Split(indexStr, "#")
				if len(ret) != 2 {
					log.Errorf("Fuzztag BUG: `#` in fuzztag indexStr cannot be divided for symbol#index")
					return r
				}
				symbol, _ = strconv.Atoi(ret[0])
				indexInt, _ = strconv.Atoi(ret[1])

			} else {
				indexInt, _ = strconv.Atoi(indexStr)
			}

			if symbol > 0 {
				d.AST.symbolTable[symbol].CurrentOffset += 1
			}
			payload := results[i][indexInt]
			if isPayload {
				payloads = append(payloads, payload[:])
			}
			buf.Write(payload)
		}
		fResult := buf.Bytes()

		// 调用回调函数
		if d.callback != nil {
			if !d.callback(fResult, payloads) {
				return r
			}
		}
		r = append(r, fResult)
		if err := mx.Next(); err != nil {
			return r
		}
	}
}

type DataNode struct {
	tokens          []*token
	InParentTagNode bool
}

func (d *DataNode) ToBytes() []byte {
	var buf bytes.Buffer
	for _, t := range d.tokens {
		if t == nil {
			continue
		}
		buf.Write(t.Raw)
	}
	return buf.Bytes()
}

func (d *DataNode) IsExecutable() bool {
	return false
}

func (d *DataNode) Execute(m map[string]func([]byte) [][]byte) [][]byte {
	if d.InParentTagNode {
		byt := bytes.ReplaceAll(d.ToBytes(), []byte{'\\', ')'}, []byte{')'})
		return [][]byte{bytes.ReplaceAll(byt, []byte{'\\', '('}, []byte{'('})}
	}
	return [][]byte{d.ToBytes()}
}

type TagNode struct {
	Method   string
	Params   ExecutableNode
	Symbol   int
	Label    string
	AST      *FuzzTagAST
	callback func([]byte, [][]byte) bool
	RawBytes []byte
}

func (t *TagNode) ToBytes() []byte {
	if t == nil {
		return nil
	}
	return t.RawBytes
}

func (t *TagNode) IsExecutable() bool {
	return true
}

var methodAlias = new(sync.Map)

func SetMethodAlias(origin string, names ...string) {
	for _, i := range names {
		if existingOrigin, ok := methodAlias.Load(i); ok {
			// 提醒：新的 origin 覆盖了旧的 origin
			log.Warnf("Alias fuzztag {{%s}} for {{%s}} is being overwritten by {{%s}}.", i, existingOrigin, origin)
		}
		methodAlias.Store(i, origin)
	}
}

func GetMethodAlias(name string) string {
	i, ok := methodAlias.Load(name)
	if !ok {
		return ""
	}
	raw, _ := i.(string)
	return raw
}

func (t *TagNode) Execute(m map[string]func([]byte) [][]byte) [][]byte {
	if m == nil || len(m) <= 0 {
		return [][]byte{{}}
	}
	f, ok := m[t.Method]
	if !ok {
		f, ok = m[GetMethodAlias(t.Method)]
		if !ok {
			return [][]byte{{}}
		}
	}
	var results [][]byte
	if t.Params == nil {
		return f(nil)
	}

	if v, ok := t.Params.(*Nodes); ok {
		v.InTag = true
	}

	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(t.Method)), "expr:") {
		var params string
		if t.Params != nil {
			params = string(t.Params.ToBytes())
		}
		return f([]byte(params))
	}
	t.Params.ToBytes()
	for _, n := range t.Params.Execute(m) {
		r := f(n)
		if t.callback != nil {
			t.callback(n, r)
		}
		if r == nil {
			results = append(results, []byte{})
			continue
		}
		results = append(results, r...)
	}
	return results
}

func NewDataNode(t ...*token) *DataNode {
	return &DataNode{tokens: t}
}

var symbolTagRe = regexp.MustCompile(`::(\d+)$`)

func (a *FuzzTagAST) NewMethodNode(method string, paramNode ExecutableNode) *TagNode {
	//var symbol = 0
	//if ret := symbolTagRe.FindStringSubmatch(method); ret != nil {
	//	symbol, _ = strconv.Atoi(ret[1])
	//	if symbol > 0 {
	//		method = symbolTagRe.ReplaceAllString(method, "")
	//	}
	//}
	var label string
	methodParts := strings.Split(method, "::")
	if len(methodParts) > 1 {
		method = methodParts[0]
		label = methodParts[1]
	}
	return &TagNode{
		Method: method,
		Params: paramNode,
		Symbol: 0,
		AST:    a,
		Label:  label,
	}
}
