package standard_parser

import (
	"golang.org/x/exp/maps"
	"strings"
)

// 对于不合法字符 string 和 []rune 不能无损转换，所以有了stringx
type stringx []rune

type trieNode struct {
	children   map[rune]*trieNode
	failure    *trieNode
	patternLen int
	id         int
	flag       int // 对节点的标记，可以用来标记结束节点
}

// IndexAllSubstrings 只遍历一次查找所有子串位置
// 返回值是一个二维数组，每个元素是一个[2]int类型匹配结果，其中第一个元素是规则index，第二个元素是索引位置
func IndexAllSubstringsEx(s stringx, patterns ...stringx) (result [][2]int) {
	// 构建trie树
	root := &trieNode{
		children:   make(map[rune]*trieNode),
		failure:    nil,
		flag:       0,
		patternLen: 0,
	}

	for patternIndex, pattern := range patterns {
		node := root
		for _, char := range pattern {
			if _, ok := node.children[char]; !ok {
				node.children[char] = &trieNode{
					children:   make(map[rune]*trieNode),
					failure:    nil,
					flag:       0,
					patternLen: 0,
					id:         patternIndex,
				}
			}
			node = node.children[char]
		}
		node.flag = 1
		node.patternLen = len(pattern)
	}
	// 构建Failure
	queue := make([]*trieNode, 0)
	root.failure = root

	for _, child := range root.children {
		child.failure = root
		queue = append(queue, child)
	}

	for len(queue) > 0 {
		node := queue[0]
		queue = queue[1:]

		for char, child := range node.children {
			queue = append(queue, child)
			failure := node.failure

			for failure != root && failure.children[char] == nil {
				failure = failure.failure
			}

			if next := failure.children[char]; next != nil {
				child.failure = next
				child.flag = child.flag | next.flag
			} else {
				child.failure = root
			}
		}
	}

	// 查找
	node := root
	for i, char := range s {
		for node != root && node.children[char] == nil {
			node = node.failure
		}

		if next := node.children[char]; next != nil {
			node = next
			if node.flag == 1 {
				result = append(result, [2]int{node.id, i - node.patternLen + 1})
			}
		}
	}
	return
}
func IndexAllSubstrings(s string, patterns ...string) [][2]int {
	ps := []stringx{}
	for _, pattern := range patterns {
		ps = append(ps, stringx(pattern))
	}
	return IndexAllSubstringsEx(stringx(s), ps...)
}

type Escaper struct {
	escapeSymbol string
	escapeChars  map[string]stringx
}

func (e *Escaper) Escape(s string) string {
	keys := maps.Keys(e.escapeChars)
	poses := IndexAllSubstrings(s, keys...)
	res := ""
	pre := 0
	for _, pos := range poses {
		key := keys[pos[0]]
		res += s[pre:pos[1]]
		res += (e.escapeSymbol + key)
		pre = pos[1] + len(key)
	}
	res += s[pre:]
	return res
}
func (e *Escaper) Unescape(s string) (string, error) {
	res, err := e.UnescapeEx(s)
	return string(res), err
}
func (e *Escaper) UnescapeEx(s string) (stringx, error) {
	// 构建trie树
	root := &trieNode{
		children:   make(map[rune]*trieNode),
		failure:    nil,
		flag:       0,
		patternLen: 0,
	}
	patterns := []string{}
	for pattern, _ := range e.escapeChars {
		patterns = append(patterns, pattern)
		node := root
		for _, char := range pattern {
			if _, ok := node.children[char]; !ok {
				node.children[char] = &trieNode{
					children:   make(map[rune]*trieNode),
					failure:    nil,
					flag:       0,
					patternLen: 0,
					id:         len(patterns) - 1,
				}
			}
			node = node.children[char]
		}
		node.flag = 1
		node.patternLen = len(pattern)
	}

	var result stringx
	escapeState := false
	node := root
	data := s
	for {
		if escapeState {
			escapeState = false
			runeData := []rune(data)
			for i := 0; i < len(runeData); i++ {
				ch := runeData[i]
				if node.children[ch] != nil {
					node = node.children[ch]
					if node.flag == 1 { // 匹配成功
						result = append(result, []rune(patterns[node.id])...)
						data = string(runeData[i+1:])
						node = root
						break
					}
				} else {
					result = append(result, runeData[:i]...)
					data = string(runeData[i:])
					node = root
					break
				}
			}
		} else {
			index := strings.Index(data, e.escapeSymbol) // 查找后面第一个转义符
			if index != -1 {
				result = append(result, []rune(data[:index])...)
				data = data[index+len(e.escapeSymbol):]
				escapeState = true
			} else {
				result = append(result, []rune(data)...)
				break
			}
		}
	}
	return result, nil
}
func NewEscaper(escapeSymbol string, charsMap map[string]stringx) *Escaper {
	if _, ok := charsMap[escapeSymbol]; !ok {
		charsMap[escapeSymbol] = stringx(escapeSymbol)
	}
	return &Escaper{
		escapeSymbol: escapeSymbol,
		escapeChars:  charsMap,
	}
}
func NewDefaultEscaper(chars ...string) *Escaper {
	m := map[string]stringx{}
	for _, char := range chars {
		m[char] = stringx(char)
	}
	return NewEscaper(`\`, m)
}
