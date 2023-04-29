package fuzztag

import (
	"fmt"
	"yaklang/common/utils"
	"strings"
)

type FuzzTagAST struct {
	Lexer *FuzzTagLexer
	root  *Nodes

	symbolTable map[int]*SymbolContext
}

func ParseToFuzzTagAST(l *FuzzTagLexer) (*FuzzTagAST, error) {
	if l == nil {
		return nil, utils.Errorf("empty lexer")
	}

	if len(l.Tokens()) <= 0 {
		return nil, utils.Errorf("tokens is empty")
	}

	ast := &FuzzTagAST{
		Lexer:       l,
		symbolTable: make(map[int]*SymbolContext),
	}
	ast.root = ast.parse()

	return ast, nil
}

func (f *FuzzTagAST) Execute(m map[string]func([]byte) [][]byte) (res [][]byte, err error) {
	return f.ExecuteWithCallBack(m, nil)
}
func (f *FuzzTagAST) ExecuteWithCallBack(m map[string]func([]byte) [][]byte, cb func([]byte, [][]byte) bool) (res [][]byte, err error) {
	defer func() {
		if e := recover(); e != nil {
			err = utils.Error(e)
		}
	}()
	f.root.SetRoot()
	f.root.SetPayloadCallback(cb)
	return f.root.Execute(m), nil
}
func (f *FuzzTagAST) parse() (root *Nodes) {
	defer func() {
		if e := recover(); e != nil {
			root.Nodes = []ExecutableNode{NewDataNode(f.Lexer.Tokens()...)}
		}
	}()
	tokens := f.Lexer.Tokens()[:]
	var index = 0

	root = &Nodes{AST: f}
	var nodes []ExecutableNode
	for {
		nodes, index = f.readTagNode(tokens, index)
		if nodes != nil {
			root.Nodes = append(root.Nodes, nodes...)
		} else {
			break
		}
		if index >= len(tokens) {
			break
		}
	}
	return root
}

func (f *FuzzTagAST) readTagNode(t []*token, index int) (fb []ExecutableNode, fbIndex int) {
	originIndex := index
	skipSpace := func() {
		for {
			dataNode := t[index]
			if dataNode.Type == TokenType_DATA && strings.TrimSpace(string(dataNode.Raw)) == "" {
				index++
			} else {
				break
			}
		}
	}
	defer func() {
		if recover() != nil {
			fbIndex = index
			fb = append(fb, NewDataNode(t[originIndex:fbIndex]...))
		}
	}()

	// 一般有两种情况
	// 第一种是不带括号的参数的，这种的话，基本就是替换，无所谓
	tagOpen := t[index]
	if tagOpen.Type != TokenType_TAG_OPEN {
		return []ExecutableNode{NewDataNode(tagOpen)}, originIndex + 1
	}
	index++
	methodNodes := []ExecutableNode{}
	var n ExecutableNode
	for {
		skipSpace()
		n, index = f.readMethodNode(t, index, false)
		if _, ok := n.(*DataNode); n == nil || ok {
			panic(fmt.Sprintf("parse method error, unexpect node: %s", string(t[index].Raw)))
		}
		methodNodes = append(methodNodes, n)
		skipSpace()
		if index < len(t) {
			tagClose := t[index]
			if tagClose.Type == TokenType_TAG_CLOSE {
				return methodNodes, index + 1
			}
		} else {
			return []ExecutableNode{NewDataNode(t[originIndex:]...)}, index + 1
		}
	}
	//skipSpace()
	//if index < len(t) {
	//	tagClose := t[index]
	//	if tagClose.Type != TokenType_TAG_CLOSE {
	//		return []ExecutableNode{NewDataNode(t[originIndex:index]...)}, index
	//	}
	//	return methodNodes, index + 1
	//} else {
	//	return []ExecutableNode{NewDataNode(t[originIndex:]...)}, index + 1
	//}

}

func (f *FuzzTagAST) readMethodNode(t []*token, index int, inParam bool) (fb ExecutableNode, fbIndex int) {
	originIndex := index
	now := func() *token {
		if index >= len(t) {
			return nil
		}
		return t[index]
	}
	tagName := t[index]
	methodName := string(tagName.Raw)
	if !inParam {
		skipChar := []rune{' ', '\n', '\r'}
		methodName = strings.TrimFunc(methodName, func(r rune) bool {
			for _, r2 := range skipChar {
				if r2 == r {
					return true
				}
			}
			return false
		})
	}
	if !isIdentifyString(methodName) {
		if methodName == "{{" {
			return nil, originIndex
		}
		return NewDataNode(tagName), index + 1
	}
	//if tagName.Type != TokenType_METHOND {
	//	return NewDataNode( tagName), index + 1
	//}

	index++
	n := t[index]
	switch n.Type {
	case TokenType_TAG_CLOSE:
		//return NewDataNode( t[originIndex:]...), index + 1
		return f.NewMethodNode(methodName, &Nodes{
			Nodes: []ExecutableNode{},
			AST:   f,
		}), index
	case TokenType_LEFT_PAREN:
		index++
		var nodes []ExecutableNode
		var node ExecutableNode

		for {
			if now().Type == TokenType_RIGHT_PAREN {
				// 读到节点了
				index++
				break
			}
			var methodNodes []ExecutableNode
			if now() != nil && now().Type == TokenType_TAG_OPEN {
				methodNodes, index = f.readTagNode(t, index)
				if methodNodes != nil {
					nodes = append(nodes, methodNodes...)
				}
			} else {
				node, index = f.readMethodNode(t, index, true)
				if node != nil {
					nodes = append(nodes, node)
				}
			}
			if now() == nil {
				return NewDataNode(t[originIndex:index]...), index
			}
		}

		return f.NewMethodNode(methodName, &Nodes{
			Nodes: nodes,
			AST:   f,
		}), index
	case TokenType_TAG_OPEN:
		return NewDataNode(tagName), index
	default:
		return NewDataNode(t[originIndex]), originIndex + 1
		//return NewDataNode(n), index + 1
		//panic(fmt.Sprintf("read tag failed... ERR For token: [%v]", n.Type))
	}
}
