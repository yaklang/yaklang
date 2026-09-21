package sca

import (
	"context"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/yaklang/yaklang/common/sca/model"
)

func TestBundlerOriginalRangeAndSource(t *testing.T) {
	lock := `GEM
  remote: https://rubygems.org/
  specs:
    faker (1.9.3)
      i18n (>= 0.7)
    i18n (1.6.0)
      concurrent-ruby (~> 1.0)
    concurrent-ruby (1.1.5)

PLATFORMS
  ruby

DEPENDENCIES
  faker (~> 1.9)

BUNDLED WITH
   1.17.2
`
	r, err := ScanReport(context.Background(), fstest.MapFS{"Gemfile.lock": {Data: []byte(lock)}}, WithSnapshotID("bundler-qual"))
	if r == nil || err != nil || !r.Complete {
		t.Fatalf("complete=%v err=%v", r != nil && r.Complete, err)
	}
	faker := mustNamed(t, r, "faker", "1.9.3")
	if faker.Key.Source != "https://rubygems.org/" {
		t.Fatalf("gem remote source %q", faker.Key.Source)
	}
	if faker.Key.Architecture != "" {
		t.Fatalf("ruby platform is not Architecture: %q", faker.Key.Architecture)
	}
	var from string
	for _, o := range r.Observations {
		if o.Component == faker.Key.ID() {
			from = o.ID()
		}
	}
	found := false
	for _, q := range r.Requirements {
		if q.From != from || q.Target != "i18n" {
			continue
		}
		found = true
		if q.Constraint != ">= 0.7" {
			t.Fatalf("original gem range lost: %+v", q)
		}
		if q.Condition != "i18n (>= 0.7)" {
			t.Fatalf("raw spec %q", q.Condition)
		}
		if len(q.Resolved) != 1 {
			t.Fatalf("unique i18n should resolve: %+v", q)
		}
	}
	if !found {
		t.Fatalf("missing i18n requirement: %s", extra5149JSON(t, r.Requirements))
	}
	assertSBOMReq(t, r, from, "i18n", ">= 0.7", "i18n (>= 0.7)", func() []string {
		for _, q := range r.Requirements {
			if q.From == from && q.Target == "i18n" {
				return q.Resolved
			}
		}
		return nil
	}())
}

func TestBundlerEmptyConstraintAndAmbiguousName(t *testing.T) {
	lock := `GEM
  remote: https://rubygems.org/
  specs:
    app (1.0.0)
      foo
      bar (~> 1.0)
    foo (1.0.0)
    bar (1.0.0)
    bar (2.0.0)

GIT
  remote: https://github.com/example/tool.git
  revision: abcdef
  specs:
    tool (1.0.0)

PATH
  remote: .
  specs:
    localgem (0.1.0)

DEPENDENCIES
  app
  tool
  localgem

BUNDLED WITH
   2.3.0
`
	r, err := ScanReport(context.Background(), fstest.MapFS{"Gemfile.lock": {Data: []byte(lock)}}, WithSnapshotID("bundler-amb"))
	if r == nil || err != nil || !r.Complete {
		t.Fatalf("complete=%v err=%v", r != nil && r.Complete, err)
	}
	app := mustNamed(t, r, "app", "1.0.0")
	tool := mustNamed(t, r, "tool", "1.0.0")
	local := mustNamed(t, r, "localgem", "0.1.0")
	if tool.Key.Source != "https://github.com/example/tool.git#abcdef" {
		t.Fatalf("git source %q", tool.Key.Source)
	}
	if local.Key.Source != "." {
		t.Fatalf("path source %q", local.Key.Source)
	}
	var from string
	for _, o := range r.Observations {
		if o.Component == app.Key.ID() {
			from = o.ID()
		}
	}
	var sawFoo, sawBar bool
	for _, q := range r.Requirements {
		if q.From != from {
			continue
		}
		if strings.HasPrefix(q.Target, "unresolved-") {
			t.Fatalf("synthetic target %+v", q)
		}
		switch q.Target {
		case "foo":
			sawFoo = true
			if q.Constraint != "" || q.Condition != "foo" || len(q.Resolved) != 1 {
				t.Fatalf("empty constraint foo: %+v", q)
			}
		case "bar":
			sawBar = true
			if q.Constraint != "~> 1.0" || q.Condition != "bar (~> 1.0)" {
				t.Fatalf("bar range %+v", q)
			}
			if len(q.Resolved) != 0 {
				t.Fatalf("ambiguous bar must not pick by order: %+v", q)
			}
		}
	}
	if !sawFoo || !sawBar {
		t.Fatalf("missing foo/bar: %s", extra5149JSON(t, r.Requirements))
	}
}

func TestBundlerPlatformVariantNotArchitecture(t *testing.T) {
	lock := `GEM
  remote: https://rubygems.org/
  specs:
    mini_portile2 (2.8.0)
    nokogiri (1.14.0-x86_64-darwin)
      mini_portile2 (~> 2.8.0)
    nokogiri (1.14.0-java)

GIT
  remote: https://github.com/example/tool.git
  revision: abcdef
  specs:
    tool (1.0.0)

PATH
  remote: .
  specs:
    localgem (0.1.0)
      nokogiri

DEPENDENCIES
  localgem
  tool

BUNDLED WITH
   2.3.0
`
	r, err := ScanReport(context.Background(), fstest.MapFS{"Gemfile.lock": {Data: []byte(lock)}}, WithSnapshotID("bundler-plat"))
	if r == nil || err != nil || !r.Complete {
		t.Fatalf("complete=%v err=%v", r != nil && r.Complete, err)
	}
	var darwin, java *model.Component
	for i := range r.Components {
		c := &r.Components[i]
		if c.Key.Name != "nokogiri" || c.Key.Version != "1.14.0" {
			continue
		}
		if c.Key.Architecture != "" {
			t.Fatalf("ruby platform is Variant, not Architecture: %+v", c.Key)
		}
		switch c.Key.Variant {
		case "x86_64-darwin":
			darwin = c
		case "java":
			java = c
		}
	}
	if darwin == nil || java == nil {
		t.Fatalf("platform identities missing: %+v", r.Components)
	}
	if darwin.Key.ID() == java.Key.ID() {
		t.Fatal("platform variants collapsed")
	}
	if darwin.Key.Source != "https://rubygems.org/" {
		t.Fatalf("gem remote %q", darwin.Key.Source)
	}
	tool := mustNamed(t, r, "tool", "1.0.0")
	if tool.Key.Source != "https://github.com/example/tool.git#abcdef" {
		t.Fatalf("git source %q", tool.Key.Source)
	}
	local := mustNamed(t, r, "localgem", "0.1.0")
	if local.Key.Source != "." {
		t.Fatalf("path source %q", local.Key.Source)
	}
	var from string
	for _, o := range r.Observations {
		if o.Component == local.Key.ID() {
			from = o.ID()
		}
	}
	found := false
	for _, q := range r.Requirements {
		if q.From != from || q.Target != "nokogiri" {
			continue
		}
		found = true
		if q.Constraint != "" || q.Condition != "nokogiri" {
			t.Fatalf("empty platform-unspecified range: %+v", q)
		}
		if len(q.Resolved) != 0 {
			t.Fatalf("two nokogiri platforms must not pick by order: %+v", q)
		}
	}
	if !found {
		t.Fatalf("missing nokogiri requirement: %s", extra5149JSON(t, r.Requirements))
	}
}
