package antlr4JS

import (
	_ "embed"
	"github.com/antlr/antlr4/runtime/Go/antlr/v4"
	"github.com/yaklang/yaklang/common/log"
	JS "github.com/yaklang/yaklang/common/yak/antlr4JS/parser"
	"testing"
	"time"
)

//go:embed test.js
var testJS string

func TestSyntax(t *testing.T) {
	lexer := JS.NewJavaScriptLexer(antlr.NewInputStream(testJS))
	lexer.RemoveErrorListeners()
	tokenStream := antlr.NewCommonTokenStream(lexer, antlr.TokenDefaultChannel)
	start := time.Now()
	ts := tokenStream.GetTokenSource()
	count := 0
	for {
		t := ts.NextToken()
		count++
		_ = t
		if count%1000 == 0 {
		}
		if t.GetTokenType() == antlr.TokenEOF {
			break
		}
	}
	log.Infof("get all tokens(%v) cost: %v", count, time.Now().Sub(start))

	log.Infof("start to build ast via parser")
	lexer = JS.NewJavaScriptLexer(antlr.NewInputStream(testJS))
	tokenStream = antlr.NewCommonTokenStream(lexer, antlr.TokenDefaultChannel)
	parser := JS.NewJavaScriptParser(tokenStream)
	_ = parser.Program()
	log.Infof("finish to build ast via parser")
}

func TestBasicSyntax1(t *testing.T) {
	code := `1+1;{1+1

2}`

	lexer := JS.NewJavaScriptLexer(antlr.NewInputStream(code))
	tokenStream := antlr.NewCommonTokenStream(lexer, antlr.TokenDefaultChannel)
	start := time.Now()
	ts := tokenStream.GetTokenSource()
	count := 0
	for {
		t := ts.NextToken()
		count++
		_ = t
		if count%1000 == 0 {
		}
		if t.GetTokenType() == antlr.TokenEOF {
			break
		}
		// fmt.Printf("%v\n", t)
	}
	log.Infof("get all tokens(%v) cost: %v", count, time.Now().Sub(start))

	log.Infof("start to build ast via parser")
	lexer = JS.NewJavaScriptLexer(antlr.NewInputStream(code))
	lexer.RemoveErrorListeners()
	tokenStream = antlr.NewCommonTokenStream(lexer, antlr.TokenDefaultChannel)
	parser := JS.NewJavaScriptParser(tokenStream)
	var prog = parser.Program()
	_ = prog
	log.Infof("finish to build ast via parser")
}

func TestBasicSyntax(t *testing.T) {
	code := `
var h = n.visualViewport ? n.visualViewport.width: innerWidth, d = n.visualViewport ? n.visualViewport.height: innerHeight, p = window.scrollX || pageXOffset, g = window.scrollY || pageYOffset, m = e.getBoundingClientRect(), b = m.height, _ = m.width, x = m.top, w = m.right, S = m.bottom, E = m.left, O = "start" === i || "nearest" === i ? x: "end" === i ? S: x + b / 2, C = "center" === o ? E + _ / 2 : "end" === o ? w: E, k = [], M = 0

        return e.prototype.delete = function(e) {
            var t = this.has(e);
            return t && delete this.data[e],
            t
        }


var t = this.has(e);
1 /Math.PI;

  var ne = function() {
        function e() {
            this.data = {}
        }
        return e.prototype.delete = function(e) {
            var t = this.has(e);
            return t && delete this.data[e],
            t
        },
        e.prototype.has = function(e) {
            return this.data.hasOwnProperty(e)
        },
        e.prototype.get = function(e) {
            return this.data[e]
        },
        e.prototype.set = function(e, t) {
            return this.data[e] = t,
            this
        },
        e.prototype.keys = function() {
            return j(this.data)
        },
        e.prototype.forEach = function(e) {
            var t = this.data;
            for (var n in t) t.hasOwnProperty(n) && e(t[n], n)
        },
        e
    } (),
    re = "function" === typeof Map;



for (var h = n.visualViewport ? n.visualViewport.width: innerWidth, d = n.visualViewport ? n.visualViewport.height: innerHeight, p = window.scrollX || pageXOffset, g = window.scrollY || pageYOffset, m = e.getBoundingClientRect(), b = m.height, _ = m.width, x = m.top, w = m.right, S = m.bottom, E = m.left, O = "start" === i || "nearest" === i ? x: "end" === i ? S: x + b / 2, C = "center" === o ? E + _ / 2 : "end" === o ? w: E, k = [], M = 0; M < c.length; M++) {
            var T = c[M],
            j = T.getBoundingClientRect(),
            P = j.height,
            I = j.width,
            B = j.top,
            N = j.right,
            L = j.bottom,
            D = j.left;
            if ("if-needed" === r && x >= 0 && E >= 0 && S <= d && w <= h && x >= B && S <= L && E >= D && w <= N) return k;
            var R = getComputedStyle(T),
            F = parseInt(R.borderLeftWidth, 10),
            U = parseInt(R.borderTopWidth, 10),
            z = parseInt(R.borderRightWidth, 10),
            H = parseInt(R.borderBottomWidth, 10),
            V = 0,
            G = 0,
            W = "offsetWidth" in T ? T.offsetWidth - T.clientWidth - F - z: 0,
            q = "offsetHeight" in T ? T.offsetHeight - T.clientHeight - U - H: 0;
            if (l === T) V = "start" === i ? O: "end" === i ? O - d: "nearest" === i ? A(g, g + d, d, U, H, g + O, g + O + b, b) : O - d / 2,
            G = "start" === o ? C: "center" === o ? C - h / 2 : "end" === o ? C - h: A(p, p + h, h, F, z, p + C, p + C + _, _),
            V = Math.max(0, V + g),
            G = Math.max(0, G + p);
            else {
                V = "start" === i ? O - B - U: "end" === i ? O - L + H + q: "nearest" === i ? A(B, L, P, U, H + q, O, O + b, b) : O - (B + P / 2) + q / 2,
                G = "start" === o ? C - D - F: "center" === o ? C - (D + I / 2) + W / 2 : "end" === o ? C - N + z + W: A(D, N, I, F, z + W, C, C + _, _);
                var Q = T.scrollLeft,
                Y = T.scrollTop;
                O += Y - (V = Math.max(0, Math.min(Y + V, T.scrollHeight - P + q))),
                C += Q - (G = Math.max(0, Math.min(Q + G, T.scrollWidth - I + W)))
            }
            k.push({
                el: T,
                top: V,
                left: G
            })
        }


`

	lexer := JS.NewJavaScriptLexer(antlr.NewInputStream(code))
	tokenStream := antlr.NewCommonTokenStream(lexer, antlr.TokenDefaultChannel)
	start := time.Now()
	ts := tokenStream.GetTokenSource()
	count := 0
	for {
		t := ts.NextToken()
		count++
		_ = t
		if count%1000 == 0 {
		}
		if t.GetTokenType() == antlr.TokenEOF {
			break
		}
		// fmt.Printf("%v\n", t)
	}
	log.Infof("get all tokens(%v) cost: %v", count, time.Now().Sub(start))

	log.Infof("start to build ast via parser")
	lexer = JS.NewJavaScriptLexer(antlr.NewInputStream(code))
	lexer.RemoveErrorListeners()
	tokenStream = antlr.NewCommonTokenStream(lexer, antlr.TokenDefaultChannel)
	parser := JS.NewJavaScriptParser(tokenStream)
	var prog = parser.Program()
	_ = prog
	log.Infof("finish to build ast via parser")
}
