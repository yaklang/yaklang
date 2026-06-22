package minirehs

import (
	"fmt"
	"math/rand"
	"regexp"
	"strings"
	"testing"
)

// 本文件构造一个随机 RE2 正则生成器 + 差分一致性"模糊测试"安全网: 随机生成大量风格各异
// 的 RE2 正则 (字面量/字符类/交替/分组/量词/锚点/各种 flag), 在随机语料上对比自研引擎与
// stdlib 逐条 oracle 的命中集合, 必须逐字节完全一致. 它是所有性能优化的正确性兜底:
// 任何预过滤/窗口/去重引入的偏差都会在这里以可复现的 seed 暴露.
//
// 关键词: 差分测试, fuzz, 随机正则生成, engine vs oracle, 一致性安全网

// 字面量字符集: 全部非正则元字符, 直接写入正则与语料都安全 (无需转义).
const safeLiteralBytes = "abcdefghijklmnopqrstuvwxyz0123456789 :=/_"

// genAtom 生成一个"原子" (可被量词修饰的最小单元). depth 控制递归分组深度.
func genAtom(r *rand.Rand, depth int) string {
	// 深度耗尽时只产出简单原子, 避免无界递归与超大程序.
	maxKind := 7
	if depth <= 0 {
		maxKind = 5
	}
	switch r.Intn(maxKind) {
	case 0:
		// 单字面量字符.
		return string(safeLiteralBytes[r.Intn(len(safeLiteralBytes))])
	case 1:
		// 2-4 字符的字面量串 (制造必需字面量, 走预过滤/窗口路径).
		n := 2 + r.Intn(3)
		var sb strings.Builder
		for i := 0; i < n; i++ {
			sb.WriteByte(safeLiteralBytes[r.Intn(26)]) // 仅字母, 便于植入语料
		}
		return sb.String()
	case 2:
		// 预定义字符类.
		return []string{`\d`, `\w`, `\s`, `\D`, `\W`, `.`}[r.Intn(6)]
	case 3:
		// 自定义字符类.
		return []string{`[a-z]`, `[0-9]`, `[A-Za-z]`, `[abc]`, `[^0-9]`, `[a-z0-9_]`, `[ :=/]`}[r.Intn(7)]
	case 4:
		// 锚点 (作为原子直接返回, 不再被量词修饰, 见 genPiece).
		return []string{`^`, `$`, `\b`, `\B`}[r.Intn(4)]
	case 5:
		// 非捕获分组.
		return "(?:" + genAlt(r, depth-1) + ")"
	default:
		// 捕获分组.
		return "(" + genAlt(r, depth-1) + ")"
	}
}

// genPiece 生成"原子 + 可选量词". 锚点原子不加量词 (^* 之类非法).
func genPiece(r *rand.Rand, depth int) string {
	a := genAtom(r, depth)
	if a == "^" || a == "$" || a == `\b` || a == `\B` {
		return a
	}
	q := ""
	switch r.Intn(7) {
	case 0:
		q = "?"
	case 1:
		q = "*"
	case 2:
		q = "+"
	case 3:
		q = fmt.Sprintf("{%d}", 1+r.Intn(3))
	case 4:
		q = fmt.Sprintf("{%d,%d}", 1+r.Intn(2), 3+r.Intn(3))
	case 5:
		q = fmt.Sprintf("{%d,}", 1+r.Intn(2))
	default:
		q = ""
	}
	// 偶尔变懒惰量词.
	if q != "" && q != "?" && r.Intn(2) == 0 {
		q += "?"
	}
	return a + q
}

// genConcat 生成若干 piece 的串联.
func genConcat(r *rand.Rand, depth int) string {
	n := 1 + r.Intn(4)
	var sb strings.Builder
	for i := 0; i < n; i++ {
		sb.WriteString(genPiece(r, depth))
	}
	s := sb.String()
	if s == "" {
		s = "x"
	}
	return s
}

// genAlt 生成若干 concat 的交替.
func genAlt(r *rand.Rand, depth int) string {
	n := 1 + r.Intn(3)
	parts := make([]string, n)
	for i := range parts {
		parts[i] = genConcat(r, depth)
	}
	return strings.Join(parts, "|")
}

// genPattern 生成一条可编译的随机 RE2 正则 (内部已 compile-check, 失败则重试).
func genPattern(r *rand.Rand) string {
	for attempt := 0; attempt < 20; attempt++ {
		expr := genAlt(r, 2+r.Intn(2))
		if _, err := regexp.Compile(expr); err != nil {
			continue
		}
		// 排除"匹配空串"的退化正则: 它会在每个位置产生空匹配, 虽两端一致但意义不大,
		// 且会拖慢差分. 用是否匹配空输入近似判断.
		re := regexp.MustCompile(expr)
		if re.MatchString("") {
			continue
		}
		return expr
	}
	return "fallback123" // 极少触发: 兜底一个普通字面量正则.
}

// genFuzzCorpus 生成随机语料, 并以一定概率植入若干 pattern 中出现过的字母串片段,
// 以提升真实命中密度 (覆盖窗口验证/去重/提前停止等路径).
func genFuzzCorpus(r *rand.Rand, plants []string, n int) []byte {
	const alphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789 \t\n:=/_.\"'\\{}[]"
	buf := make([]byte, 0, n)
	for len(buf) < n {
		if len(plants) > 0 && r.Intn(6) == 0 {
			buf = append(buf, plants[r.Intn(len(plants))]...)
			continue
		}
		buf = append(buf, alphabet[r.Intn(len(alphabet))])
	}
	return buf[:n]
}

// extractPlants 从一批正则里抽取连续字母片段, 作为语料植入素材.
func extractPlants(exprs []string) []string {
	var plants []string
	for _, e := range exprs {
		cur := make([]byte, 0, 8)
		flush := func() {
			if len(cur) >= 2 {
				plants = append(plants, string(cur))
			}
			cur = cur[:0]
		}
		for i := 0; i < len(e); i++ {
			c := e[i]
			if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') {
				cur = append(cur, c)
			} else {
				flush()
			}
		}
		flush()
	}
	return plants
}

// randFlags 随机返回一组 flag (含 0).
func randFlags(r *rand.Rand) Flag {
	var f Flag
	if r.Intn(2) == 0 {
		f |= FlagCaseless
	}
	if r.Intn(3) == 0 {
		f |= FlagDotAll
	}
	if r.Intn(3) == 0 {
		f |= FlagMultiline
	}
	return f
}

// TestFuzzDifferentialEngineVsOracle 是核心模糊安全网: 多轮随机生成 pattern 集 + 随机语料,
// 自研引擎命中集合必须与 stdlib oracle 完全一致.
func TestFuzzDifferentialEngineVsOracle(t *testing.T) {
	rounds := 60
	if testing.Short() {
		rounds = 12
	}
	masterSeed := int64(0x1E3779B97F4A7C15)

	for round := 0; round < rounds; round++ {
		seed := masterSeed + int64(round)
		r := rand.New(rand.NewSource(seed))

		npat := 8 + r.Intn(24)
		exprs := make([]string, 0, npat)
		patterns := make([]Pattern, 0, npat)
		for i := 0; i < npat; i++ {
			expr := genPattern(r)
			exprs = append(exprs, expr)
			patterns = append(patterns, Pattern{ID: PatternID(i + 1), Expr: expr, Flags: randFlags(r)})
		}

		engine, err := Compile(patterns, WithBackend(BackendEngine), WithLogger(silentLogger{}))
		if err != nil {
			t.Fatalf("round=%d seed=%d compile engine: %v", round, seed, err)
		}
		oracle, err := Compile(patterns, WithBackend(BackendStdlib), WithLogger(silentLogger{}))
		if err != nil {
			engine.Close()
			t.Fatalf("round=%d seed=%d compile oracle: %v", round, seed, err)
		}

		plants := extractPlants(exprs)
		corpora := 20
		if testing.Short() {
			corpora = 6
		}
		for c := 0; c < corpora; c++ {
			size := r.Intn(2048)
			data := genFuzzCorpus(r, plants, size)
			eng := scanAllSet(t, engine, data)
			ora := scanAllSet(t, oracle, data)
			assertSameMatchSet(t, eng, ora, fmt.Sprintf("round=%d seed=%d corpus=%d len=%d", round, seed, c, size))
			if t.Failed() {
				t.Logf("FAILED patterns:")
				for _, p := range patterns {
					t.Logf("  id=%d flags=%d expr=%q", p.ID, p.Flags, p.Expr)
				}
				t.Fatalf("round=%d seed=%d corpus=%d data=%q", round, seed, c, data)
			}
		}
		engine.Close()
		oracle.Close()
	}
}

// TestFuzzTinyInputs 专门覆盖极短/空输入与边界 (空数据、单字节、纯换行等).
func TestFuzzTinyInputs(t *testing.T) {
	r := rand.New(rand.NewSource(12345))
	var patterns []Pattern
	var exprs []string
	for i := 0; i < 30; i++ {
		e := genPattern(r)
		exprs = append(exprs, e)
		patterns = append(patterns, Pattern{ID: PatternID(i + 1), Expr: e, Flags: randFlags(r)})
	}
	engine, err := Compile(patterns, WithBackend(BackendEngine), WithLogger(silentLogger{}))
	if err != nil {
		t.Fatal(err)
	}
	defer engine.Close()
	oracle, err := Compile(patterns, WithBackend(BackendStdlib), WithLogger(silentLogger{}))
	if err != nil {
		t.Fatal(err)
	}
	defer oracle.Close()

	inputs := [][]byte{
		{}, []byte(""), []byte("a"), []byte("\n"), []byte("\n\n\n"),
		[]byte(" "), []byte("ab"), []byte("0"), []byte("=:/"), []byte("\x00\x01\x02"),
		[]byte(strings.Repeat("a", 1)), []byte(strings.Repeat("ab", 2)),
	}
	for i, in := range inputs {
		eng := scanAllSet(t, engine, in)
		ora := scanAllSet(t, oracle, in)
		assertSameMatchSet(t, eng, ora, fmt.Sprintf("tiny#%d len=%d", i, len(in)))
		if t.Failed() {
			t.Fatalf("tiny#%d in=%q", i, in)
		}
	}
}
