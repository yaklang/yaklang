package minirehs

// ahoCorasick 是一个自实现的纯 Go 多字面量匹配自动机 (零外部依赖), 用作字面量预过滤.
// 它在 ASCII 小写域工作: 构建时字面量已小写, 扫描时数据也按 ASCII 小写处理, 从而实现
// 大小写无关的"超集"过滤 (允许假阳, 绝不假阴; 真伪由后续正则验证判定).
//
// 关键词: Aho-Corasick, 多字面量匹配, prefilter, 自动机
type ahoCorasick struct {
	// goto/转移表: 每个状态 256 路 (字节). 用一维切片 base = state*256.
	next []int32
	// fail 指针.
	fail []int32
	// output: 每个状态匹配到的字面量 id 列表 (含 fail 链汇总). 用 outFlat + outOff 压平存储.
	outOff  []int32
	outFlat []int32
	numLit  int
}

const acNoState int32 = -1

// acBuilder 用于增量构建 trie, 再编译为 ahoCorasick.
type acBuilder struct {
	next   [][256]int32
	output [][]int32
}

func newACBuilder() *acBuilder {
	b := &acBuilder{}
	// 根状态 0.
	b.next = append(b.next, blankRow())
	b.output = append(b.output, nil)
	return b
}

func blankRow() [256]int32 {
	var r [256]int32
	for i := range r {
		r[i] = acNoState
	}
	return r
}

// add 把一个字面量 (已 ASCII 小写) 以给定 litID 加入 trie.
func (b *acBuilder) add(literal string, litID int32) {
	if literal == "" {
		return
	}
	state := int32(0)
	for i := 0; i < len(literal); i++ {
		c := literal[i]
		nx := b.next[state][c]
		if nx == acNoState {
			nx = int32(len(b.next))
			b.next = append(b.next, blankRow())
			b.output = append(b.output, nil)
			b.next[state][c] = nx
		}
		state = nx
	}
	b.output[state] = append(b.output[state], litID)
}

// build 用 BFS 计算 fail 指针, 把 goto 表补全为 DFA 转移, 并压平输出, 返回不可变自动机.
func (b *acBuilder) build(numLit int) *ahoCorasick {
	numStates := len(b.next)
	fail := make([]int32, numStates)
	for i := range fail {
		fail[i] = 0
	}

	// BFS 队列从根的直接子节点开始.
	queue := make([]int32, 0, numStates)
	for c := 0; c < 256; c++ {
		s := b.next[0][c]
		if s == acNoState {
			b.next[0][c] = 0 // 根的缺失转移指回根.
		} else {
			fail[s] = 0
			queue = append(queue, s)
		}
	}

	for qi := 0; qi < len(queue); qi++ {
		state := queue[qi]
		for c := 0; c < 256; c++ {
			s := b.next[state][c]
			if s == acNoState {
				// DFA 化: 缺失转移走 fail 链.
				b.next[state][c] = b.next[fail[state]][c]
				continue
			}
			fail[s] = b.next[fail[state]][c]
			// 合并 fail 状态的输出 (这样扫描时只看当前状态输出即可).
			b.output[s] = append(b.output[s], b.output[fail[s]]...)
			queue = append(queue, s)
		}
	}

	// 压平转移表与输出表.
	flatNext := make([]int32, numStates*256)
	for st := 0; st < numStates; st++ {
		copy(flatNext[st*256:st*256+256], b.next[st][:])
	}
	outOff := make([]int32, numStates+1)
	total := 0
	for st := 0; st < numStates; st++ {
		total += len(b.output[st])
	}
	outFlat := make([]int32, 0, total)
	for st := 0; st < numStates; st++ {
		outOff[st] = int32(len(outFlat))
		outFlat = append(outFlat, b.output[st]...)
	}
	outOff[numStates] = int32(len(outFlat))

	return &ahoCorasick{
		next:    flatNext,
		fail:    fail,
		outOff:  outOff,
		outFlat: outFlat,
		numLit:  numLit,
	}
}

// scan 对 (已 ASCII 小写的) data 扫描一遍, 对命中的每个 (litID, end) 调用 onHit,
// 其中 end 是该字面量在 data 中的结束位置 (exclusive). 位置用于后续邻域窗口验证.
func (ac *ahoCorasick) scan(data []byte, onHit func(litID int32, end int)) {
	state := int32(0)
	for i := 0; i < len(data); i++ {
		state = ac.next[state<<8|int32(data[i])]
		off := ac.outOff[state]
		end := ac.outOff[state+1]
		for ; off < end; off++ {
			onHit(ac.outFlat[off], i+1)
		}
	}
}

// scanFoldASCII 是 scan 的大小写无关版本。它在状态转移前就地折叠 ASCII 大写字母，
// 避免 prefilter 为整段报文另建 lower 副本；非 ASCII 字节保持原样，因此与
// asciiLowerInto + scan 的匹配集合及偏移完全一致。
func (ac *ahoCorasick) scanFoldASCII(data []byte, onHit func(litID int32, end int)) {
	state := int32(0)
	for i, c := range data {
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		state = ac.next[state<<8|int32(c)]
		off := ac.outOff[state]
		end := ac.outOff[state+1]
		for ; off < end; off++ {
			onHit(ac.outFlat[off], i+1)
		}
	}
}
