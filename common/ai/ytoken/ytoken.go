package ytoken

import (
	"bufio"
	"bytes"
	"compress/gzip"
	_ "embed"
	"encoding/base64"
	"math"
	"strconv"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"

	regexp2 "github.com/VillanCh/go-pcre2-lite/regexp2"
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
	pcreInitOnce    sync.Once
	mergeableRanks  map[string]int
	specialTokens   map[string]int
	maxSpecialLen   int
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
}

func ensurePCRE() {
	pcreInitOnce.Do(func() {
		compiledPattern = regexp2.MustCompile(bpePattern, 0)
	})
}

func initSpecialTokens() {
	specialTokens = make(map[string]int, 208)
	id := specialStartID
	for _, tok := range []string{endOfText, imStart, imEnd} {
		specialTokens[tok] = id
		maxSpecialLen = max(maxSpecialLen, len(tok))
		id++
	}
	for i := 0; i < 205; i++ {
		tok := "<|extra_" + strconv.Itoa(i) + "|>"
		specialTokens[tok] = id
		maxSpecialLen = max(maxSpecialLen, len(tok))
		id++
	}
}

func openBpeData() *gzip.Reader {
	gr, err := gzip.NewReader(bytes.NewReader(qwenBpeDataGz))
	if err != nil {
		panic("ytoken: gzip open: " + err.Error())
	}
	return gr
}

func initMergeableRanks() {
	gr := openBpeData()
	defer gr.Close()
	mergeableRanks = make(map[string]int, 160000)
	scanner := bufio.NewScanner(gr)
	// The current vocabulary's longest line is well below the initial buffer.
	// Keep a generous growth limit for future vocabularies without paying a
	// 1 MiB allocation on every process start.
	scanner.Buffer(make([]byte, 4<<10), 1<<20)
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
	ensureInit()
	sink := tokenSink{}
	tokenize(text, &sink, true)
	return sink.count
}

// CalcOrdinaryTokenCount returns token count without special token handling.
func CalcOrdinaryTokenCount(text string) int {
	ensureInit()
	sink := tokenSink{}
	encodeOrdinary(text, &sink)
	return sink.count
}

// CalcTokenCountUpTo counts tokens until text is fully processed or the count
// exceeds limit. The returned count is exact when exceeded is false; otherwise
// it is an early-exit count greater than limit rather than the total count.
func CalcTokenCountUpTo(text string, limit int) (count int, exceeded bool) {
	if limit < 0 {
		return 0, true
	}
	ensureInit()
	sink := tokenSink{bounded: true, limit: limit}
	tokenize(text, &sink, true)
	return sink.count, sink.exceeded
}

// TokenCountExceeds reports whether text contains more than limit tokens.
// Since byte-level BPE cannot produce more tokens than input bytes, inputs no
// longer than a non-negative limit are reported as fitting without invoking
// the tokenizer.
func TokenCountExceeds(text string, limit int) bool {
	if limit < 0 {
		return true
	}
	if len(text) <= limit && utf8.ValidString(text) {
		return false
	}
	_, exceeded := CalcTokenCountUpTo(text, limit)
	return exceeded
}

// Encode returns Qwen BPE token IDs, recognizing special tokens.
func Encode(text string) []int {
	ensureInit()
	sink := tokenSink{collect: true}
	if len(text) >= 4 {
		sink.tokens = make([]int, 0, len(text)/4)
	}
	tokenize(text, &sink, true)
	return sink.tokens
}

// EncodeOrdinary encodes text without special token processing.
func EncodeOrdinary(text string) []int {
	ensureInit()
	sink := tokenSink{collect: true}
	if len(text) >= 4 {
		sink.tokens = make([]int, 0, len(text)/4)
	}
	encodeOrdinary(text, &sink)
	return sink.tokens
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

type tokenSink struct {
	tokens   []int
	count    int
	limit    int
	collect  bool
	bounded  bool
	exceeded bool
}

func (s *tokenSink) addToken(rank int) bool {
	if s.collect {
		s.tokens = append(s.tokens, rank)
	}
	return s.addCount(1)
}

func (s *tokenSink) addCount(count int) bool {
	s.count += count
	if s.bounded && s.count > s.limit {
		s.exceeded = true
		return false
	}
	return true
}

func encodeOrdinary(text string, sink *tokenSink) bool {
	if canUsePureGoPreTokenizer(text) {
		return encodeOrdinaryPureGo(text, sink)
	}
	return encodeOrdinaryPCRE(text, sink)
}

func encodeOrdinaryPCRE(text string, sink *tokenSink) bool {
	ensurePCRE()
	mapper := newRuneByteMapper(text)
	m, err := compiledPattern.FindStringMatch(text)
	for err == nil && m != nil {
		piece := mapper.slice(m.Index, m.Index+m.Length, m)
		if !bpeEncodePiece(piece, sink) {
			return false
		}
		m, err = compiledPattern.FindNextMatch(m)
	}
	return true
}

func canUsePureGoPreTokenizer(text string) bool {
	for _, r := range text {
		if r < utf8.RuneSelf {
			continue
		}
		// Go 1.22 uses Unicode 15.0 while the bundled PCRE2 can know newer
		// assignments. Fall back for code points unassigned in Go so those rare
		// inputs retain the historical PCRE category behavior.
		if !unicode.In(r, unicode.L, unicode.M, unicode.N, unicode.P, unicode.S, unicode.Z, unicode.Cc, unicode.Cf, unicode.Co) {
			return false
		}
	}
	return true
}

// runeByteMapper slices the original UTF-8 string using regexp2's rune-based
// match offsets. This avoids allocating m.String() for every regex piece.
// Invalid UTF-8 retains regexp2's historical RuneError normalization path.
type runeByteMapper struct {
	text      string
	validUTF8 bool
	ascii     bool
	runePos   int
	bytePos   int
}

func newRuneByteMapper(text string) runeByteMapper {
	ascii := true
	for i := 0; i < len(text); i++ {
		if text[i] >= utf8.RuneSelf {
			ascii = false
			break
		}
	}
	return runeByteMapper{text: text, ascii: ascii, validUTF8: ascii || utf8.ValidString(text)}
}

func (m *runeByteMapper) byteOffset(runeOffset int) int {
	for m.runePos < runeOffset {
		_, size := utf8.DecodeRuneInString(m.text[m.bytePos:])
		m.bytePos += size
		m.runePos++
	}
	return m.bytePos
}

func (m *runeByteMapper) slice(startRune, endRune int, match *regexp2.Match) string {
	if !m.validUTF8 {
		return match.String()
	}
	if m.ascii {
		return m.text[startRune:endRune]
	}
	start := m.byteOffset(startRune)
	end := m.byteOffset(endRune)
	return m.text[start:end]
}

func encodeOrdinaryPureGo(text string, sink *tokenSink) bool {
	if !utf8.ValidString(text) {
		// regexp2 historically normalizes each invalid byte through []rune.
		text = string([]rune(text))
	}
	for start := 0; start < len(text); {
		end := nextPieceEnd(text, start)
		if end <= start {
			_, size := utf8.DecodeRuneInString(text[start:])
			end = start + size
		}
		if !bpeEncodePiece(text[start:end], sink) {
			return false
		}
		start = end
	}
	return true
}

// nextPieceEnd implements bpePattern as an anchored, allocation-free scanner.
// The alternatives intentionally follow the regex order because contractions,
// leading spaces, and whitespace lookahead depend on that precedence.
func nextPieceEnd(text string, start int) int {
	if end := contractionEnd(text, start); end > start {
		return end
	}

	// [^\r\n\p{L}\p{N}]?\p{L}+
	pos := start
	r, size := utf8.DecodeRuneInString(text[pos:])
	if r != '\r' && r != '\n' && !unicode.IsLetter(r) && !unicode.IsNumber(r) {
		next := pos + size
		if next < len(text) {
			nextRune, _ := utf8.DecodeRuneInString(text[next:])
			if unicode.IsLetter(nextRune) {
				pos = next
				r = nextRune
			}
		}
	}
	if unicode.IsLetter(r) {
		for pos < len(text) {
			r, size = utf8.DecodeRuneInString(text[pos:])
			if !unicode.IsLetter(r) {
				break
			}
			pos += size
		}
		return pos
	}

	// \p{N}
	r, size = utf8.DecodeRuneInString(text[start:])
	if unicode.IsNumber(r) {
		return start + size
	}

	//  ?[^\s\p{L}\p{N}]+[\r\n]*
	pos = start
	if text[pos] == ' ' && pos+1 < len(text) {
		nextRune, _ := utf8.DecodeRuneInString(text[pos+1:])
		if isSymbolRune(nextRune) {
			pos++
		}
	}
	if pos < len(text) {
		r, size = utf8.DecodeRuneInString(text[pos:])
		if isSymbolRune(r) {
			for pos < len(text) {
				r, size = utf8.DecodeRuneInString(text[pos:])
				if !isSymbolRune(r) {
					break
				}
				pos += size
			}
			for pos < len(text) {
				r, size = utf8.DecodeRuneInString(text[pos:])
				if r != '\r' && r != '\n' {
					break
				}
				pos += size
			}
			return pos
		}
	}

	// \s*[\r\n]+ -- greediness makes this end after the last newline in the
	// current whitespace run, leaving any trailing non-newline space behind.
	if isWhitespaceAt(text, start) {
		pos = start
		lastNewlineEnd := -1
		lastRuneStart := start
		runeCount := 0
		for pos < len(text) {
			r, size = utf8.DecodeRuneInString(text[pos:])
			if !unicode.IsSpace(r) {
				break
			}
			lastRuneStart = pos
			pos += size
			runeCount++
			if r == '\r' || r == '\n' {
				lastNewlineEnd = pos
			}
		}
		if lastNewlineEnd >= 0 {
			return lastNewlineEnd
		}

		// \s+(?!\S) consumes the complete final whitespace run. Before a
		// non-space rune it backtracks one rune so the lookahead sees space.
		if pos == len(text) {
			return pos
		}
		if runeCount > 1 {
			return lastRuneStart
		}

		// Final \s+ alternative.
		return pos
	}

	return start
}

func contractionEnd(text string, start int) int {
	if start >= len(text) || text[start] != '\'' {
		return -1
	}
	for _, suffix := range [...]string{"s", "t", "re", "ve", "m", "ll", "d"} {
		end := start + 1
		for range suffix {
			if end >= len(text) {
				break
			}
			_, size := utf8.DecodeRuneInString(text[end:])
			end += size
		}
		if strings.EqualFold(text[start+1:end], suffix) {
			return end
		}
	}
	return -1
}

func isSymbolRune(r rune) bool {
	return !unicode.IsSpace(r) && !unicode.IsLetter(r) && !unicode.IsNumber(r)
}

func isWhitespaceAt(text string, start int) bool {
	r, _ := utf8.DecodeRuneInString(text[start:])
	return unicode.IsSpace(r)
}

const smallPieceThreshold = 100

type bpePart struct {
	start int
	rank  int
}

func bpeEncodePiece(piece string, sink *tokenSink) bool {
	if piece == "" {
		return true
	}
	// Every non-empty pre-tokenized piece emits at least one token. Avoid the
	// merge work when a bounded counter has already consumed its full budget.
	if sink.bounded && sink.count >= sink.limit {
		return sink.addCount(1)
	}
	if rank, ok := mergeableRanks[piece]; ok {
		return sink.addToken(rank)
	}
	if len(piece) < smallPieceThreshold {
		return bpeEncodeSmall(piece, sink)
	}
	return bpeEncodeLarge(piece, sink)
}

func bpeEncodeSmall(piece string, sink *tokenSink) bool {
	var storage [smallPieceThreshold + 1]bpePart
	parts := storage[:len(piece)+1]
	minRank, minIndex := math.MaxInt, 0
	for i := 0; i < len(piece)-1; i++ {
		rank := pairRank(piece, i, i+2)
		parts[i] = bpePart{start: i, rank: rank}
		if rank < minRank {
			minRank, minIndex = rank, i
		}
	}
	parts[len(piece)-1] = bpePart{start: len(piece) - 1, rank: math.MaxInt}
	parts[len(piece)] = bpePart{start: len(piece), rank: math.MaxInt}

	getRank := func(i int) int {
		if i+3 >= len(parts) {
			return math.MaxInt
		}
		return pairRank(piece, parts[i].start, parts[i+3].start)
	}

	for minRank != math.MaxInt {
		i := minIndex
		if i > 0 {
			parts[i-1].rank = getRank(i - 1)
		}
		parts[i].rank = getRank(i)
		copy(parts[i+1:], parts[i+2:])
		parts = parts[:len(parts)-1]

		minRank, minIndex = math.MaxInt, 0
		for i := 0; i < len(parts)-1; i++ {
			if parts[i].rank < minRank {
				minRank, minIndex = parts[i].rank, i
			}
		}
	}

	tokenCount := len(parts) - 1
	if sink.collect {
		for i := 0; i < tokenCount; i++ {
			sink.tokens = append(sink.tokens, mergeableRanks[piece[parts[i].start:parts[i+1].start]])
		}
	}
	return sink.addCount(tokenCount)
}

func pairRank(piece string, start, end int) int {
	if rank, ok := mergeableRanks[piece[start:end]]; ok {
		return rank
	}
	return math.MaxInt
}

type bpeMerge struct {
	rank    int
	start   int
	version uint32
}

type bpeMergeHeap []bpeMerge

func (h bpeMergeHeap) Less(i, j int) bool {
	if h[i].rank != h[j].rank {
		return h[i].rank < h[j].rank
	}
	return h[i].start < h[j].start
}
func (h bpeMergeHeap) Swap(i, j int) { h[i], h[j] = h[j], h[i] }

func (h *bpeMergeHeap) push(value bpeMerge) {
	*h = append(*h, value)
	for child := len(*h) - 1; child > 0; {
		parent := (child - 1) / 2
		if !h.Less(child, parent) {
			break
		}
		h.Swap(child, parent)
		child = parent
	}
}

func (h *bpeMergeHeap) pop() bpeMerge {
	items := *h
	result := items[0]
	last := len(items) - 1
	if last == 0 {
		*h = items[:0]
		return result
	}
	items[0] = items[last]
	items = items[:last]
	*h = items
	for parent := 0; ; {
		left := parent*2 + 1
		if left >= len(items) {
			break
		}
		child := left
		right := left + 1
		if right < len(items) && h.Less(right, left) {
			child = right
		}
		if !h.Less(child, parent) {
			break
		}
		h.Swap(parent, child)
		parent = child
	}
	return result
}

type bpeLink struct {
	prev    int
	next    int
	version uint32
	alive   bool
}

func bpeEncodeLarge(piece string, sink *tokenSink) bool {
	links := make([]bpeLink, len(piece))
	merges := make(bpeMergeHeap, 0, len(piece))
	for i := range links {
		links[i] = bpeLink{prev: i - 1, next: i + 1, alive: true}
	}
	links[len(links)-1].next = -1

	pushPair := func(start int) {
		links[start].version++
		right := links[start].next
		if right < 0 {
			return
		}
		end := links[right].next
		if end < 0 {
			end = len(piece)
		}
		if rank, ok := mergeableRanks[piece[start:end]]; ok {
			merges.push(bpeMerge{rank: rank, start: start, version: links[start].version})
		}
	}

	for i := 0; i < len(piece)-1; i++ {
		pushPair(i)
	}
	for len(merges) > 0 {
		candidate := merges.pop()
		left := candidate.start
		if !links[left].alive || links[left].version != candidate.version {
			continue
		}
		right := links[left].next
		if right < 0 || !links[right].alive {
			continue
		}

		next := links[right].next
		links[left].next = next
		links[right].alive = false
		links[right].version++
		if next >= 0 {
			links[next].prev = left
		}
		pushPair(left)
		if prev := links[left].prev; prev >= 0 {
			pushPair(prev)
		}
	}

	tokenCount := 0
	for start := 0; start >= 0; start = links[start].next {
		end := links[start].next
		if end < 0 {
			end = len(piece)
		}
		if sink.collect {
			sink.tokens = append(sink.tokens, mergeableRanks[piece[start:end]])
		}
		tokenCount++
		if links[start].next < 0 {
			break
		}
	}
	return sink.addCount(tokenCount)
}

// --- special token scanning ---

func tokenize(text string, sink *tokenSink, recognizeSpecial bool) bool {
	if !recognizeSpecial || !strings.Contains(text, specialPrefix) {
		return encodeOrdinary(text, sink)
	}

	ordinaryStart, searchStart := 0, 0
	for searchStart < len(text) {
		rel := strings.Index(text[searchStart:], specialPrefix)
		if rel < 0 {
			break
		}
		start := searchStart + rel
		id, end, ok := specialTokenAt(text, start)
		if !ok {
			searchStart = start + len(specialPrefix)
			continue
		}
		if start > ordinaryStart && !encodeOrdinary(text[ordinaryStart:start], sink) {
			return false
		}
		if !sink.addToken(id) {
			return false
		}
		ordinaryStart, searchStart = end, end
	}
	return ordinaryStart == len(text) || encodeOrdinary(text[ordinaryStart:], sink)
}

func specialTokenAt(text string, start int) (id int, end int, ok bool) {
	limit := min(len(text), start+maxSpecialLen)
	relEnd := strings.Index(text[start:limit], specialSuffix)
	if relEnd < 0 {
		return 0, 0, false
	}
	end = start + relEnd + len(specialSuffix)
	id, ok = specialTokens[text[start:end]]
	return id, end, ok
}
