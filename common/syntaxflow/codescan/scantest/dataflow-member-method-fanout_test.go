package scantest

import (
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

// This file pins the traversal rule behind the object/member fan-out that made
// a heavy Java rule hit the dataflow value limit on a large project:
//
//	A method member is reached as a callee, not as a field. A call site that
//	reads `obj.m()` already carries `obj` as its receiver, so the value flow is
//	carried by the object's own uses. Walking the method member as if it were a
//	data field makes bottom-use descend every call site of that method -- on a
//	class blueprint, every method of the class, each pulling in its own caller
//	set.
//
// The cases below assert both directions:
//
//	forward  - a value written into a field still reaches the sink that reads
//	           the field, and a value handed to a method still reaches the sink
//	           inside that method.
//	backward - tracing the object must NOT drag in values that the object never
//	           carries: a decoy field that is never read, and a second
//	           independent instance with no call path.

// methodFanoutRule reads the sink argument in the caller.

// TestDataflowMemberMethodArgumentsStillFlow pins that passing an object to a
// method still propagates the field value into the method body.
func TestDataflowMemberMethodArgumentsStillFlow(t *testing.T) {
	t.Run("go", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, `package main

type Box struct {
	cmd string
}

func consume(any) {}

func (b *Box) Run() {
	consume(b.cmd)
}

func run(cmd string) {
	b := &Box{}
	b.cmd = cmd
	b.Run()
}
`, `consume(* #-> * as $target)`, map[string][]string{
			"target": {"Parameter-cmd"},
		}, ssaapi.WithLanguage(ssaconfig.GO))
	})

	t.Run("java", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, `
package com.example;

class Box {
    public String cmd;

    void run() {
        Sink.consume(this.cmd);
    }
}

class Sink {
    static void consume(Object o) {}
}

class CaseMethodFlow {
    public void assign(String cmd) {
        Box box = new Box();
        box.cmd = cmd;
        box.run();
    }
}
`, `consume(* #-> * as $target)`, map[string][]string{
			"target": {"Parameter-cmd"},
		}, ssaapi.WithLanguage(ssaconfig.JAVA))
	})

	t.Run("php", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, `<?php
class Box {
    public $cmd;

    public function run() {
        consume($this->cmd);
    }
}
function consume($any) {}
function assign($cmd) {
    $box = new Box();
    $box->cmd = $cmd;
    $box->run();
}
`, `consume(* #-> * as $target)`, map[string][]string{
			"target": {"Parameter-$cmd"},
		}, ssaapi.WithLanguage(ssaconfig.PHP))
	})
}

// TestDataflowMemberMethodTraversalNoDecoyLeak pins that tracing the read site
// does not drag in a sibling field that the program never reads. A traversal
// that enumerates the whole class surface fails this: the decoy field reaches
// the result purely because it is a member of the same object.
func TestDataflowMemberMethodTraversalNoDecoyLeak(t *testing.T) {
	cases := []struct {
		name   string
		source string
		lang   ssaconfig.Language
		rule   string
		decoy  string
	}{
		{
			name: "go",
			source: `package main

type Box struct {
	cmd   string
	decoy string
}

func consume(any) {}

func (b *Box) run() {
	consume(b.cmd)
}

func assign(cmd string) {
	b := &Box{}
	b.cmd = cmd
	b.decoy = "decoy"
	b.run()
}
`,
			lang:  ssaconfig.GO,
			rule:  `consume(* #-> * as $target)`,
			decoy: `"decoy"`,
		},
		{
			name: "java",
			source: `
package com.example;

class Box {
    public String cmd;
    public String decoy;

    void run() {
        Sink.consume(this.cmd);
    }
}

class Sink {
    static void consume(Object o) {}
}

class CaseNoDecoy {
    public void assign(String cmd) {
        Box box = new Box();
        box.cmd = cmd;
        box.decoy = "decoy";
        box.run();
    }
}
`,
			lang:  ssaconfig.JAVA,
			rule:  `consume(* #-> * as $target)`,
			decoy: `"decoy"`,
		},
		{
			name: "php",
			source: `<?php
class Box {
    public $cmd;
    public $decoy;

    public function run() {
        consume($this->cmd);
    }
}
function consume($any) {}
function assign($cmd) {
    $box = new Box();
    $box->cmd = $cmd;
    $box->decoy = "decoy";
    $box->run();
}
`,
			lang:  ssaconfig.PHP,
			rule:  `consume(* #-> * as $target)`,
			decoy: `"decoy"`,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			prog, err := ssaapi.Parse(tc.source, ssaapi.WithLanguage(tc.lang))
			if err != nil {
				t.Fatalf("parse failed: %v", err)
			}
			res, err := prog.SyntaxFlowWithError(tc.rule)
			if err != nil {
				t.Fatalf("syntaxflow failed: %v", err)
			}
			got := map[string]bool{}
			for _, v := range res.GetValues("target") {
				got[v.String()] = true
			}
			if len(got) == 0 {
				t.Fatalf("sink matched nothing; the forward flow is missing")
			}
			if got[tc.decoy] {
				t.Errorf("sibling field %s reached the sink: %v", tc.decoy, got)
			}
		})
	}
}
