package memedit

import (
	"fmt"
	"github.com/yaklang/yaklang/common/utils"
	"strings"
	"testing"
)

func TestPromptContext(t *testing.T) {
	e := NewMemEditor(`package sarif
Values: 2
0:Undefined-DocumentBuilderFactory.newInstance(valid)()
Undefined-db.parse(valid)(Undefined-ByteArrayInputStream(Undefined-ByteArrayInputStream,Undefined-xmlData.getBytes()))

Values: 2
0: [Call  ] Undefined-DocumentBuilderFactory.newInstance(valid)()	27:60 - 27:73: newInstance()
1: [Call  ] Undefined-db.parse(valid)(Undefined-ByteArrayInputStream(Undefined-ByteArrayInputStream,Undefined-xmlData.getBytes()))	41:26 - 41:42: parse(xmlStream)

`)
	raw := e.GetTextContextWithPrompt(NewRange(NewPosition(3, 5), NewPosition(3, 33)), 3)
	//fmt.Println(raw)
	if !strings.Contains(raw, `       ^~~~~~~`) {
		t.Fatal("PromptContext failed")
	}

	raw = e.GetTextContextWithPrompt(NewRange(NewPosition(3, 5), NewPosition(4, 10)), 3, "你好～")
	if !strings.Contains(raw, `  ~~~~~~~~~~^ -- 你好～`) {
		t.Fatal("PromptContext failed")
	}

	raw = e.GetTextContextWithPrompt(NewRange(NewPosition(1, 5), NewPosition(3, 10)), 3, "你好～")
	fmt.Println(raw)
	if !utils.MatchAllOfSubString(
		raw,
		`  ~~~~~~~~~~^ -- 你好～`,
		"  ~~~~~~~~~\n", `       ^~~~~~~`,
	) {
		t.Fatal("PromptContext failed")
	}
}
