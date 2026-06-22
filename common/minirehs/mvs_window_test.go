package minirehs

import (
	"math/rand"
	"regexp"
	"strings"
	"testing"
)

// 本文件是"存在性本地化 (Rose-lite 窗口收窄)"的安全护栏: 验证收窄后的窗口 existsIn 与 stdlib
// 整段匹配在存在性上逐例一致, 重点构造"字面量远离匹配边界 + 长填充"的对抗输入, 确认窗口绝不
// 把真匹配截掉 (绝不假阴) —— 这是安全引擎的红线。
//
// 关键词: existence localization, window soundness, 绝不假阴, 对抗输入, 差分

// adversarialWindowCases 是手工挑选的"易踩窗口边界"的 pattern, 覆盖:
// 前导无界 (.*foo)、尾随无界 (token=\w+)、字面量后仍有必需内容 (eyJ{10,}\.{10,}, foo\w+bar)、
// 锚点 (^/$)、有界重复、alternation。
var adversarialWindowCases = []string{
	`eyJ[A-Za-z0-9_-]{10,}\.[A-Za-z0-9._-]{10,}`, // JWT: 字面量 eyJ 后有必需 "." -> tail 必须无界
	`token=\w+`,             // 尾随无界 -> tail 无界, head 有界
	`foo\w+bar`,             // 两字面量间无界 -> 触发 foo 后 tail 无界
	`\.oss\.aliyuncs\.com`,  // 纯字面量尾, 前导无界靠 prefilter
	`[\w-.]+\.oss\.aliyuncs\.com`, // 前导无界 + 字面量
	`jdbc:[a-z:]+://[a-z0-9\.\-_:;=/@?,&]+`,
	`(jsonp_[a-z0-9]+)|((_?callback|_cb)=)`,
	`abc[0-9]{2,4}xyz`,      // 有界重复, 两端字面量
	`(?i)password['"]?\s*[:=]`,
	`prefix.*suffix`,        // 中缀 .* -> 两字面量都必需, 触发任一都需整段
	`GET\s+\S+\.action`,
	`a{3,}b`,                // 无界下界重复 + 必需 b -> 触发 a-literal? (无 a 字面量, b 才是)
}

func TestMVSWindowSoundnessAdversarial(t *testing.T) {
	rng := rand.New(rand.NewSource(0xC0FFEE))
	for _, expr := range adversarialWindowCases {
		ref, err := regexp.Compile(expr)
		if err != nil {
			t.Fatalf("stdlib compile %q: %v", expr, err)
		}
		db, err := Compile([]Pattern{{ID: 1, Expr: expr}},
			WithBackend(BackendMVS), WithReportLocation(false), WithLogger(silentLogger{}))
		if err != nil {
			t.Fatalf("mvs compile %q: %v", expr, err)
		}
		sc, _ := db.NewScratch()

		inputs := buildAdversarialInputs(expr, ref, rng)
		for _, in := range inputs {
			want := ref.Match([]byte(in))
			got := false
			_ = db.Scan([]byte(in), sc, func(m Match) bool { got = true; return false })
			if got != want {
				t.Fatalf("DIVERGE expr=%q\n got(mvs)=%v want(stdlib)=%v\n input(len=%d)=%q",
					expr, got, want, len(in), truncForLog([]byte(in)))
			}
		}
		sc.Close()
		db.Close()
	}
}

// buildAdversarialInputs 为给定 pattern 生成既含真匹配 (各种位置/填充) 也含近似不命中的输入。
func buildAdversarialInputs(expr string, ref *regexp.Regexp, rng *rand.Rand) []string {
	var out []string
	// 1) 用 stdlib 反向构造若干真匹配样例 (经验串), 再加随机填充把字面量推到不同偏移。
	seeds := matchSeedsFor(expr)
	fillers := []string{
		"",
		strings.Repeat("A", 50),
		strings.Repeat("x9_", 200), // 长填充, 多在 \w 类内, 易诱发"窗口截断"假阴
		strings.Repeat(" ", 300),
		randAlnum(rng, 1000),
		strings.Repeat("eyJ", 30), // 重复字面量前缀诱发多命中点 union
	}
	for _, seed := range seeds {
		for _, pre := range fillers {
			for _, post := range fillers {
				out = append(out, pre+seed+post)
			}
		}
	}
	// 2) 纯随机串 (大概率不命中, 校验无假阳)。
	for i := 0; i < 20; i++ {
		out = append(out, randAlnum(rng, 100+rng.Intn(2000)))
	}
	// 3) 仅含字面量但缺必需尾/头的"近似串" (校验窗口不引入假阳, 也不因截断改判)。
	for _, seed := range seeds {
		if len(seed) > 4 {
			out = append(out, seed[:len(seed)/2]+randAlnum(rng, 500))
		}
	}
	return out
}

// matchSeedsFor 返回一组"应当命中 expr"的经验串 (人工构造, 覆盖各 pattern 的真匹配形态)。
func matchSeedsFor(expr string) []string {
	switch expr {
	case `eyJ[A-Za-z0-9_-]{10,}\.[A-Za-z0-9._-]{10,}`:
		return []string{
			"eyJ" + strings.Repeat("a", 10) + "." + strings.Repeat("b", 10),
			"eyJ" + strings.Repeat("a", 400) + "." + strings.Repeat("b", 12), // 第一段超长 -> 窗口若按 min 截断必假阴
		}
	case `token=\w+`:
		return []string{"token=a", "token=" + strings.Repeat("z", 500)}
	case `foo\w+bar`:
		return []string{"fooXbar", "foo" + strings.Repeat("Q", 600) + "bar"}
	case `\.oss\.aliyuncs\.com`:
		return []string{".oss.aliyuncs.com"}
	case `[\w-.]+\.oss\.aliyuncs\.com`:
		return []string{"my-bucket.oss.aliyuncs.com", strings.Repeat("b", 300) + ".oss.aliyuncs.com"}
	case `jdbc:[a-z:]+://[a-z0-9\.\-_:;=/@?,&]+`:
		return []string{"jdbc:mysql://host", "jdbc:mysql://" + strings.Repeat("a", 400)}
	case `(jsonp_[a-z0-9]+)|((_?callback|_cb)=)`:
		return []string{"jsonp_abc123", "callback=", "_cb="}
	case `abc[0-9]{2,4}xyz`:
		return []string{"abc12xyz", "abc1234xyz"}
	case `(?i)password['"]?\s*[:=]`:
		return []string{"password:", "PASSWORD = ", "password\"="}
	case `prefix.*suffix`:
		return []string{"prefixsuffix", "prefix" + strings.Repeat("M", 700) + "suffix"}
	case `GET\s+\S+\.action`:
		return []string{"GET /a.action", "GET " + strings.Repeat("p", 300) + ".action"}
	case `a{3,}b`:
		return []string{"aaab", strings.Repeat("a", 400) + "b"}
	}
	return nil
}

func randAlnum(rng *rand.Rand, n int) string {
	const al = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789 _-./"
	b := make([]byte, n)
	for i := range b {
		b[i] = al[rng.Intn(len(al))]
	}
	return string(b)
}

