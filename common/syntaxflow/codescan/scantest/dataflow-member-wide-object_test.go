package scantest

import (
	"fmt"
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

// This file pins the "wide object, one field read" shape across frontends:
//
//	type J struct { a, b, c, ... h string }
//	j := &J{}; j.a = cmd; j.b = "b"; ... ; Print(j.a)
//
// Two properties matter and they pull in opposite directions:
//
//	forward  - the value written into `j.a` must reach the `Print(j.a)` sink,
//	           through the real assignment path.
//	backward - the sibling fields (b..h) must NOT reach that sink, because
//	           nothing reads them.
//
// A traversal that enumerates every member of J satisfies the first property
// by accident and violates the second: it drags all eight fields into the
// result. These cases assert both, so a change that fixes fan-out by dropping
// the real flow is caught just as quickly as one that reintroduces the fan-out.

// wideObjectCases holds the same program shape in every supported frontend.
// Each entry writes a tainted value into field `a`, decoys into b..h, and reads
// only `a` at the sink.
type wideObjectCase struct {
	language ssaconfig.Language
	source   string
	// sinkRule must bind the argument read at the sink to `$target`.
	sinkRule string
	// forwardValues are the values that legitimately reach the sink.
	forwardValues []string
}

func wideObjectCases() []wideObjectCase {
	return []wideObjectCase{
		{
			language: ssaconfig.GO,
			source: `package main

type J struct {
	a string
	b string
	c string
	d string
	e string
	f string
	g string
	h string
}

func Print(any) {}

func run(cmd string) {
	j := &J{}
	j.a = cmd
	j.b = "b"
	j.c = "c"
	j.d = "d"
	j.e = "e"
	j.f = "f"
	j.g = "g"
	j.h = "h"
	Print(j.a)
}
`,
			sinkRule:      `Print(* #-> * as $target)`,
			forwardValues: []string{"Parameter-cmd"},
		},
		{
			language: ssaconfig.JAVA,
			source: `
package com.example;

class J {
    public String a;
    public String b;
    public String c;
    public String d;
    public String e;
    public String f;
    public String g;
    public String h;
}

public class WideObject {
    static void Print(Object o) {}

    public void run(String cmd) {
        J j = new J();
        j.a = cmd;
        j.b = "b";
        j.c = "c";
        j.d = "d";
        j.e = "e";
        j.f = "f";
        j.g = "g";
        j.h = "h";
        Print(j.a);
    }
}
`,
			sinkRule:      `Print(* #-> * as $target)`,
			forwardValues: []string{"Parameter-cmd"},
		},
		{
			language: ssaconfig.PHP,
			source: `<?php
class J {
    public $a;
    public $b;
    public $c;
    public $d;
    public $e;
    public $f;
    public $g;
    public $h;
}
function Print($any) {}
function run($cmd) {
    $j = new J();
    $j->a = $cmd;
    $j->b = "b";
    $j->c = "c";
    $j->d = "d";
    $j->e = "e";
    $j->f = "f";
    $j->g = "g";
    $j->h = "h";
    Print($j->a);
}
`,
			sinkRule:      `Print(* #-> * as $target)`,
			forwardValues: []string{"Parameter-$cmd"},
		},
	}
}

// TestDataflowMemberWideObject_Forward asserts the legitimate flow: the value
// assigned into field `a` reaches the sink that reads `a`.
func TestDataflowMemberWideObject_Forward(t *testing.T) {
	for _, tc := range wideObjectCases() {
		tc := tc
		t.Run(string(tc.language), func(t *testing.T) {
			ssatest.CheckSyntaxFlow(t, tc.source, tc.sinkRule, map[string][]string{
				"target": tc.forwardValues,
			}, ssaapi.WithLanguage(tc.language))
		})
	}
}

// TestDataflowMemberWideObject_NoSiblingLeak asserts the reverse property: the
// decoy fields b..h are written but never read, so none of them may appear at
// the sink. If this fails, member traversal is enumerating the whole object.
func TestDataflowMemberWideObject_NoSiblingLeak(t *testing.T) {
	decoyLiterals := []string{`"b"`, `"c"`, `"d"`, `"e"`, `"f"`, `"g"`, `"h"`}

	for _, tc := range wideObjectCases() {
		tc := tc
		t.Run(string(tc.language), func(t *testing.T) {
			prog, err := ssaapi.Parse(tc.source, ssaapi.WithLanguage(tc.language))
			if err != nil {
				t.Fatalf("parse failed: %v", err)
			}
			res, err := prog.SyntaxFlowWithError(tc.sinkRule)
			if err != nil {
				t.Fatalf("syntaxflow failed: %v", err)
			}

			got := map[string]bool{}
			for _, v := range res.GetValues("target") {
				got[v.String()] = true
			}
			t.Logf("target values: %v", keysOf(got))
			if len(got) == 0 {
				t.Fatalf("sink matched nothing; the forward flow is missing")
			}
			for _, literal := range decoyLiterals {
				if got[literal] {
					t.Errorf("sibling field value %s reached the sink: %v", literal, keysOf(got))
				}
			}
		})
	}
}

func keysOf(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

var _ = fmt.Sprintf
