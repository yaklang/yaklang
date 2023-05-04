package httptpl

import (
	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"testing"
)

func TestYakMatcher_Execute(t *testing.T) {
	rsp := []byte(`HTTP/1.1 200 OK
Content-Type: text/html; charset=utf-8
Content-Length: 202

<html>
<head>
	<META HTTP-EQUIV="Refresh" CONTENT="0;URL=example/HelloWorld.action">
</head>

Hello World

</html>`)
	for _, i := range [][]any{
		{
			&YakMatcher{
				MatcherType: "status",
				Condition:   "or",
				Group:       []string{"200,201,203"},
			},
			true,
		},
		{
			&YakMatcher{
				MatcherType: "status",
				Condition:   "and",
				Group:       []string{"200,201,203"},
			},
			false,
		},
	} {
		if ret, err := i[0].(*YakMatcher).Execute(&lowhttp.LowhttpResponse{
			RawPacket: rsp,
		}, nil); err != nil {
			panic(err)
		} else {
			if ret != i[1].(bool) {
				panic("failed for " + spew.Sdump(i))
			}
		}
	}
}

func TestYakMatcher_CL(t *testing.T) {
	rsp := []byte(`HTTP/1.1 200 OK
Content-Type: text/html; charset=utf-8
Content-Length: 202

<html>
<head>
	<META HTTP-EQUIV="Refresh" CONTENT="0;URL=example/HelloWorld.action">
</head>

Hello World

</html>`)
	for _, i := range [][]any{
		{
			&YakMatcher{
				MatcherType: "content_length",
				Condition:   "or",
				Group:       []string{"200,201,203"},
			},
			false,
		},
		{
			&YakMatcher{
				MatcherType: "content_length",
				Condition:   "or",
				Group:       []string{"200,201,203,114"},
			},
			true,
		},
		{
			&YakMatcher{
				MatcherType: "content_length",
				Condition:   "and",
				Group:       []string{"200,201,203,114"},
			},
			false,
		},
		{
			&YakMatcher{
				MatcherType: "content_length",
				Condition:   "and",
				Group:       []string{"114"},
			},
			true,
		},
	} {
		if ret, err := i[0].(*YakMatcher).ExecuteRawResponse(rsp, nil); err != nil {
			panic(err)
		} else {
			if ret != i[1].(bool) {
				panic("failed for " + spew.Sdump(i))
			}
		}
	}
}

func TestYakMatcher_Binary(t *testing.T) {
	rsp := []byte(`HTTP/1.1 200 OK
Content-Type: text/html; charset=utf-8
Content-Length: 202

<html>
<head>
	<META HTTP-EQUIV="Refresh" CONTENT="0;URL=example/HelloWorld.action">
</head>

Hello World

</html>`)
	for _, i := range [][]any{
		{
			&YakMatcher{
				MatcherType: "binary",
				Condition:   "and",
				Scope:       "header",
				Group:       []string{"323032"},
			},
			true,
		},
		{
			&YakMatcher{
				MatcherType: "binary",
				Condition:   "and",
				Scope:       "header",
				Group:       []string{"323032", "aa", "bb", "cc"},
			},
			false,
		},
		{
			&YakMatcher{
				MatcherType: "binary",
				Condition:   "or",
				Scope:       "header",
				Group:       []string{"323032", "aa", "bb", "cc"},
			},
			true,
		},
		{
			&YakMatcher{
				MatcherType: "binary",
				Condition:   "and",
				Scope:       "raw",
				Group:       []string{"323032"},
			},
			true,
		},
		{
			&YakMatcher{
				MatcherType: "binary",
				Condition:   "and",
				Scope:       "raw",
				Group:       []string{"323032", "323032323032323032323032323032323032"},
			},
			false,
		},
		{
			&YakMatcher{
				MatcherType: "binary",
				Condition:   "or",
				Scope:       "raw",
				Group:       []string{"323032", "323032323032323032323032323032323032"},
			},
			true,
		},
		{
			&YakMatcher{
				MatcherType: "binary",
				Condition:   "and",
				Scope:       "body",
				Group:       []string{"323032"},
			},
			false,
		},
		{
			&YakMatcher{
				MatcherType: "binary",
				Condition:   "and",
				Scope:       "status",
				Group:       []string{"323032"},
			},
			false,
		},
	} {
		if ret, err := i[0].(*YakMatcher).ExecuteRawResponse(rsp, nil); err != nil {
			panic(err)
		} else {
			if ret != i[1].(bool) {
				panic("failed for " + spew.Sdump(i))
			}
		}
	}
}

func TestYakMatcher_Word(t *testing.T) {
	rsp := []byte(`HTTP/1.1 200 OK
Content-Type: text/html; charset=utf-8
Content-Length: 202

<html>
<head>
	<META HTTP-EQUIV="Refresh" CONTENT="0;URL=example/HelloWorld.action">
</head>

Hello World

</html>`)
	for _, i := range [][]any{
		{
			&YakMatcher{
				MatcherType: "word",
				Condition:   "or",
				Group:       []string{"IV=\"Refresh\" CONTENT=\"0;URL=examp"},
			},
			true,
		},
		{
			&YakMatcher{
				MatcherType: "word",
				Condition:   "or",
				Group: []string{
					"IV=\"Refresh\" CONTENT=\"0;URL=examp",
					"asdfjasdfjasdf",
				},
			},
			true,
		},
		{
			&YakMatcher{
				MatcherType: "word",
				Condition:   "or",
				Scope:       "header",
				Group: []string{
					"IV=\"Refresh\" CONTENT=\"0;URL=examp",
					"asdfjasdfjasdf",
				},
			},
			false,
		},
		// text/html; charset=utf-8
		{
			&YakMatcher{
				MatcherType: "word",
				Condition:   "or",
				Scope:       "header",
				Group: []string{
					"IV=\"Refresh\" CONTENT=\"0;URL=examp",
					"asdfjasdfjasdf",
					"text/html; charset=utf-8",
				},
			},
			true,
		},
		// text/html; charset=utf-8
		{
			&YakMatcher{
				MatcherType:   "word",
				Condition:     "or",
				Scope:         "header",
				GroupEncoding: "hex",
				Group: []string{
					"617364666a617364666a61736466",
					"746578742f68746d6c3b20636861727365743d7574662d38",
				},
			},
			true,
		},
		{
			&YakMatcher{
				MatcherType: "word",
				Condition:   "or",
				Scope:       "body",
				Group: []string{
					"asdfjasdfjasdf",
					"text/html; charset=utf-8",
				},
			},
			false,
		},
		{
			&YakMatcher{
				MatcherType: "word",
				Condition:   "or",
				Scope:       "body",
				Group: []string{
					"HTTP-EQUIV=",
					"asdfjasdfjasdf",
					"text/html; charset=utf-8",
				},
			},
			true,
		},
		{
			&YakMatcher{
				MatcherType: "word",
				Condition:   "and",
				Group: []string{
					"IV=\"Refresh\" CONTENT=\"0;URL=examp",
					"asdfjasdfjasdf",
				},
			},
			false,
		},
	} {
		if ret, err := i[0].(*YakMatcher).ExecuteRawResponse(rsp, nil); err != nil {
			panic(err)
		} else {
			if ret != i[1].(bool) {
				panic("failed for " + spew.Sdump(i))
			}
		}
	}
}

func TestYakMatcher_Regexp(t *testing.T) {
	rsp := []byte(`HTTP/1.1 200 OK
Content-Type: text/html; charset=utf-8
Content-Length: 202

<html>
<head>
	<META HTTP-EQUIV="Refresh" CONTENT="0;URL=example/HelloWorld.action">
</head>

Hello World

</html>`)
	for _, i := range [][]any{
		{&YakMatcher{MatcherType: "regexp", Condition: "or", Group: []string{}}, false},
		{&YakMatcher{MatcherType: "regexp", Condition: "or", Group: []string{"jhasdjkhasjkhkhasdfjsajkdf"}}, false},
		{&YakMatcher{MatcherType: "regexp", Condition: "and", Group: []string{
			`Content-Length:\s20\d`,
		}}, true},
		{&YakMatcher{MatcherType: "regexp", Condition: "or", Group: []string{
			`Content-Length:\s20\d`,
		}}, true},
		{&YakMatcher{MatcherType: "regexp", Condition: "or", Scope: "status", Group: []string{
			`Content-Length:\s20\d`,
		}}, false},
		{&YakMatcher{MatcherType: "regexp", Condition: "or", Scope: "body", Group: []string{
			`Content-Length:\s20\d`,
		}}, false},
		{&YakMatcher{MatcherType: "regexp", Condition: "or", Scope: "header", Group: []string{
			`Content-Length:\s20\d`,
		}}, true},
		{&YakMatcher{MatcherType: "regexp", Condition: "or", Scope: "body", Group: []string{
			`Content-Length:\s20\d`, `Hello\sWorld`,
		}}, true},
		{&YakMatcher{MatcherType: "regexp", Condition: "and", Scope: "body", Group: []string{
			`Content-Length:\s20\d`, `Hello\sWorld`,
		}}, false},
		{&YakMatcher{MatcherType: "regexp", Condition: "or", Scope: "raw", Group: []string{
			`Content-Length:\s20\d`,
		}}, true},
	} {
		if ret, err := i[0].(*YakMatcher).ExecuteRawResponse(rsp, nil); err != nil {
			panic(err)
		} else {
			if ret != i[1].(bool) {
				panic("failed for " + spew.Sdump(i))
			}
		}
	}
}

func TestYakMatcher_EXPR_NUCLEI(t *testing.T) {
	rsp := []byte(`HTTP/1.1 200 OK
Content-Type: text/html; charset=utf-8
Content-Length: 202

<html>
<head>
	<META HTTP-EQUIV="Refresh" CONTENT="0;URL=example/HelloWorld.action">
</head>

Hello World

</html>`)
	count := 0
	for _, i := range [][]any{
		{&YakMatcher{MatcherType: "expr", ExprType: "nuclei-dsl", Condition: "or", Group: []string{}}, false},
		{&YakMatcher{MatcherType: "expr", ExprType: "nuclei-dsl", Condition: "or", Group: []string{"contains(`abc`, `a`)"}}, true},
		{&YakMatcher{MatcherType: "expr", ExprType: "nuclei-dsl", Condition: "or", Group: []string{"contains(`abc`, `d`)"}}, false},
		{&YakMatcher{MatcherType: "expr", ExprType: "nuclei-dsl", Condition: "or", Group: []string{"dump(status_code); status_code == 200"}}, true},
		{&YakMatcher{MatcherType: "expr", ExprType: "nuclei-dsl", Condition: "or", Group: []string{"dump(tolower(all_headers)); contains(tolower(all_headers), `content-length`)"}}, true},
	} {
		count++
		if ret, err := i[0].(*YakMatcher).ExecuteRawResponse(rsp, nil); err != nil {
			panic(err)
		} else {
			if ret != i[1].(bool) {
				panic("failed for " + spew.Sdump(i))
			}
		}
	}
	log.Infof("executed %v testcases", count)
}
