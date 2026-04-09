package ytoken

import (
	"bufio"
	"bytes"
	"compress/gzip"
	_ "embed"
	"encoding/base64"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
	"sync"

	"github.com/dlclark/regexp2"
)

//go:embed qwen.tiktoken.gz
var qwenBpeDataGz []byte

const (
	bpePattern     = `(?i:'s|'t|'re|'ve|'m|'ll|'d)|[^\r\n\p{L}\p{N}]?\p{L}+|\p{N}| ?[^\s\p{L}\p{N}]+[\r\n]*|\s*[\r\n]+|\s+(?!\S)|\s+`
	specialStartID = 151643
	specialPrefix  = "<|"
	specialSuffix  = "|>"
	endOfText      = "<|endoftext|>"
	imStart        = "<|im_start|>"
	imEnd          = "<|im_end|>"
)

var (
	initOnce        sync.Once
	mergeableRanks  map[string]int
	specialTokens   map[string]int
	specialKeys     []string
	decodeSlice     []string
	compiledPattern *regexp2.Regexp
)

func ensureInit() {
	initOnce.Do(doInit)
}

func doInit() {
	initSpecialTokens()
	initMergeableRanks()
	initDecodeSlice()
	compiledPattern = regexp2.MustCompile(bpePattern, 0)
}

func initSpecialTokens() {
	specialTokens = make(map[string]int, 208)
	specialKeys = make([]string, 0, 208)
	id := specialStartID
	for _, tok := range []string{endOfText, imStart, imEnd} {
		specialTokens[tok] = id
		specialKeys = append(specialKeys, tok)
		id++
	}
	for i := 0; i < 205; i++ {
		tok := fmt.Sprintf("<|extra_%d|>", i)
		specialTokens[tok] = id
		specialKeys = append(specialKeys, tok)
		id++
	}
}

func decompressBpeData() []byte {
	gr, err := gzip.NewReader(bytes.NewReader(qwenBpeDataGz))
	if err != nil {
		panic("ytoken: gzip open: " + err.Error())
	}
	defer gr.Close()
	data, err := io.ReadAll(gr)
	if err != nil {
		panic("ytoken: gzip decompress: " + err.Error())
	}
	return data
}

func initMergeableRanks() {
	raw := decompressBpeData()
	mergeableRanks = make(map[string]int, 160000)
	scanner := bufio.NewScanner(bytes.NewReader(raw))
	scanner.Buffer(make([]byte, 1<<20), 1<<20)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		sp := strings.LastIndex(line, " ")
		if sp < 0 {
			panic("ytoken: malformed BPE line: " + line)
		}
		tokBytes, err := base64.StdEncoding.DecodeString(line[:sp])
		if err != nil {
			panic("ytoken: base64 decode: " + err.Error())
		}
		rank, err := strconv.Atoi(line[sp+1:])
		if err != nil {
			panic("ytoken: rank parse: " + err.Error())
		}
		mergeableRanks[string(tokBytes)] = rank
	}
	if err := scanner.Err(); err != nil {
		panic("ytoken: scanner: " + err.Error())
	}
}

func initDecodeSlice() {
	n := len(mergeableRanks) + len(specialTokens)
	decodeSlice = make([]string, n)
	for tok, rank := range mergeableRanks {
		decodeSlice[rank] = tok
	}
	for tok, rank := range specialTokens {
		decodeSlice[rank] = tok
	}
}

// CalcTokenCount returns the Qwen BPE token count for text.
// Special tokens (<|im_start|>, <|im_end|>, etc.) are recognized.
func CalcTokenCount(text string) int {
	return len(Encode(text))
}

// CalcOrdinaryTokenCount returns token count without special token handling.
func CalcOrdinaryTokenCount(text string) int {
	return len(EncodeOrdinary(text))
}

// Encode returns Qwen BPE token IDs, recognizing special tokens.
func Encode(text string) []int {
	ensureInit()
	chunks := splitWithSpecial(text)
	var tokens []int
	for _, chunk := range chunks {
		if id, ok := specialTokens[chunk]; ok {
			tokens = append(tokens, id)
		} else {
			tokens = append(tokens, encodeOrdinary(chunk)...)
		}
	}
	return tokens
}

// EncodeOrdinary encodes text without special token processing.
func EncodeOrdinary(text string) []int {
	ensureInit()
	return encodeOrdinary(text)
}

// Decode converts token IDs back to text.
func Decode(tokens []int) string {
	ensureInit()
	var buf strings.Builder
	for _, id := range tokens {
		if id >= 0 && id < len(decodeSlice) {
			buf.WriteString(decodeSlice[id])
		}
	}
	return buf.String()
}

// --- internal BPE ---

func encodeOrdinary(text string) []int {
	var tokens []int
	m, err := compiledPattern.FindStringMatch(text)
	for err == nil && m != nil {
		tokens = append(tokens, bpeEncodeChunk(m.String())...)
		m, err = compiledPattern.FindNextMatch(m)
	}
	return tokens
}

type bpeNode struct {
	data []byte
	rank int
}

func bpeEncodeChunk(chunk string) []int {
	raw := []byte(chunk)
	nodes := make([]*bpeNode, len(raw))
	for i, b := range raw {
		r := math.MaxInt32
		if v, ok := mergeableRanks[string([]byte{b})]; ok {
			r = v
		}
		nodes[i] = &bpeNode{data: []byte{b}, rank: r}
	}

	if len(nodes) < 2 {
		out := make([]int, len(nodes))
		for i, n := range nodes {
			out[i] = n.rank
		}
		return out
	}

	for len(nodes) >= 2 {
		best := lowestRankPair(nodes)
		if best == nil {
			break
		}
		nodes = applyMerge(nodes, best)
	}

	out := make([]int, len(nodes))
	for i, n := range nodes {
		out[i] = n.rank
	}
	return out
}

func lowestRankPair(nodes []*bpeNode) *bpeNode {
	seen := make(map[string]struct{}, len(nodes))
	var best *bpeNode
	lo := math.MaxInt32
	for i := 0; i < len(nodes)-1; i++ {
		merged := joinBytes(nodes[i].data, nodes[i+1].data)
		key := string(merged)
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
		if r, ok := mergeableRanks[key]; ok && r < lo {
			lo = r
			best = &bpeNode{data: merged, rank: r}
		}
	}
	return best
}

func applyMerge(nodes []*bpeNode, pair *bpeNode) []*bpeNode {
	pairKey := string(pair.data)
	out := make([]*bpeNode, 0, len(nodes))
	i := 0
	for i < len(nodes) {
		if i < len(nodes)-1 {
			merged := joinBytes(nodes[i].data, nodes[i+1].data)
			if string(merged) == pairKey {
				out = append(out, &bpeNode{data: merged, rank: pair.rank})
				i += 2
				continue
			}
		}
		out = append(out, nodes[i])
		i++
	}
	return out
}

func joinBytes(a, b []byte) []byte {
	c := make([]byte, len(a)+len(b))
	copy(c, a)
	copy(c[len(a):], b)
	return c
}

// --- special token splitting ---

func splitWithSpecial(text string) []string {
	if strings.Contains(text, specialPrefix) && strings.Contains(text, specialSuffix) {
		return splitByAll(text, specialKeys)
	}
	return []string{text}
}

func splitByAll(text string, seps []string) []string {
	chunks := []string{text}
	for _, sep := range seps {
		var next []string
		for _, ch := range chunks {
			next = append(next, splitBySep(ch, sep)...)
		}
		chunks = next
	}
	return chunks
}

func splitBySep(src, sep string) []string {
	if !strings.Contains(src, sep) {
		return []string{src}
	}
	var parts []string
	from := 0
	for {
		rel := strings.Index(src[from:], sep)
		if rel < 0 {
			break
		}
		pos := from + rel
		if pos > from {
			parts = append(parts, src[from:pos])
		}
		parts = append(parts, sep)
		from = pos + len(sep)
	}
	if from < len(src) {
		parts = append(parts, src[from:])
	}
	return parts
}
