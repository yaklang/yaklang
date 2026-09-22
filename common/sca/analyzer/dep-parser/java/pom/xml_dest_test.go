package pom

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/sca/core/budget"
	"github.com/yaklang/yaklang/common/sca/core/scanerr"
	"github.com/yaklang/yaklang/common/sca/core/xmlrecord"
)

func TestPOMDestinationRepeatedDependencies(t *testing.T) {
	body := func(n int) string {
		return `<project><modelVersion>4.0.0</modelVersion><groupId>g</groupId><artifactId>a</artifactId><version>1</version><dependencies>` +
			strings.Repeat(`<dependency><groupId>g</groupId><artifactId>d</artifactId><version>1</version></dependency>`, n) +
			`</dependencies></project>`
	}
	var out pomXML
	if err := xmlrecord.Decode(context.Background(), strings.NewReader(body(2)), &out); err != nil || len(out.Dependencies.Dependency) != 2 {
		t.Fatalf("small POM dest: n=%d err=%v", len(out.Dependencies.Dependency), err)
	}
	if out.GroupId != "g" || out.ArtifactId != "a" {
		t.Fatalf("identity %+v", out)
	}
	nested := `<project><groupId>g</groupId><artifactId>a</artifactId><version>1</version><dependencyManagement><dependencies>` +
		`<dependency><groupId>g</groupId><artifactId>m</artifactId><version>2</version><exclusions><exclusion><groupId>x</groupId><artifactId>y</artifactId></exclusion></exclusions></dependency>` +
		`</dependencies></dependencyManagement><dependencies><dependency><groupId>g</groupId><artifactId>d</artifactId><version>1</version></dependency></dependencies>` +
		`<properties><foo>bar</foo></properties></project>`
	out = pomXML{}
	if err := xmlrecord.Decode(context.Background(), strings.NewReader(nested), &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Dependencies.Dependency) != 1 || len(out.DependencyManagement.Dependencies.Dependency) != 1 {
		t.Fatalf("nested slices %+v %+v", out.Dependencies, out.DependencyManagement)
	}
	if len(out.DependencyManagement.Dependencies.Dependency[0].Exclusions.Exclusion) != 1 {
		t.Fatalf("exclusion %+v", out.DependencyManagement.Dependencies.Dependency[0].Exclusions)
	}
	if out.Properties["foo"] != "bar" {
		t.Fatalf("properties %+v", out.Properties)
	}

	l, err := (budget.Limits{MaxResultBytes: 8000}).Normalize()
	if err != nil {
		t.Fatal(err)
	}
	out = pomXML{}
	out.Dependencies.Dependency = []pomDependency{{GroupID: "stale"}}
	err = xmlrecord.Decode(budget.Bind(context.Background(), l), strings.NewReader(body(80)), &out)
	if err == nil || !errors.Is(err, scanerr.ErrResourceLimit) {
		t.Fatalf("POM dest growth: n=%d err=%v", len(out.Dependencies.Dependency), err)
	}
	if len(out.Dependencies.Dependency) != 1 || out.Dependencies.Dependency[0].GroupID != "stale" {
		t.Fatalf("filled POM dest: %+v", out.Dependencies.Dependency)
	}

	high, err := (budget.Limits{MaxResultBytes: 8 << 20}).Normalize()
	if err != nil {
		t.Fatal(err)
	}
	var ok pomXML
	if err := xmlrecord.Decode(budget.Bind(context.Background(), high), strings.NewReader(body(3)), &ok); err != nil || len(ok.Dependencies.Dependency) != 3 {
		t.Fatalf("high budget POM: n=%d err=%v", len(ok.Dependencies.Dependency), err)
	}
	charge := func(n int) int64 {
		t.Helper()
		ctx := budget.Bind(context.Background(), high)
		var p pomXML
		if err := xmlrecord.Decode(ctx, strings.NewReader(body(n)), &p); err != nil || len(p.Dependencies.Dependency) != n {
			t.Fatalf("n=%d out=%d err=%v", n, len(p.Dependencies.Dependency), err)
		}
		return budget.From(ctx).ResultBytes()
	}
	if charge(3) <= charge(2) {
		t.Fatalf("POM slice doubling 2->4 must increase charge")
	}
	propsXML := `<project><groupId>g</groupId><artifactId>a</artifactId><version>1</version><properties>` + strings.Repeat(`<k>v</k>`, 2) + `</properties></project>`
	var p2 pomXML
	if err := xmlrecord.Decode(context.Background(), strings.NewReader(propsXML), &p2); err != nil || p2.Properties["k"] != "v" {
		t.Fatalf("properties: %+v %v", p2.Properties, err)
	}
	many := `<project><groupId>g</groupId><artifactId>a</artifactId><version>1</version><properties>` + strings.Repeat(`<k>v</k>`, 80) + `</properties></project>`
	staleProps := pomXML{}
	staleProps.GroupId = "stale"
	err = xmlrecord.Decode(budget.Bind(context.Background(), l), strings.NewReader(many), &staleProps)
	if err == nil || !errors.Is(err, scanerr.ErrResourceLimit) {
		t.Fatalf("property dest: err=%v", err)
	}
	if staleProps.GroupId != "stale" || staleProps.Properties != nil {
		t.Fatalf("low budget changed POM properties: %+v", staleProps)
	}

	both := `<project><groupId>g</groupId><artifactId>a</artifactId><version>1</version>` +
		`<dependencyManagement><dependencies>` + strings.Repeat(`<dependency><groupId>g</groupId><artifactId>m</artifactId><version>2</version></dependency>`, 2) +
		`</dependencies></dependencyManagement><dependencies>` + strings.Repeat(`<dependency><groupId>g</groupId><artifactId>d</artifactId><version>1</version></dependency>`, 3) +
		`</dependencies></project>`
	var split pomXML
	if err := xmlrecord.Decode(context.Background(), strings.NewReader(both), &split); err != nil {
		t.Fatal(err)
	}
	if len(split.DependencyManagement.Dependencies.Dependency) != 2 || len(split.Dependencies.Dependency) != 3 {
		t.Fatalf("two dependency paths: mgmt=%d deps=%d", len(split.DependencyManagement.Dependencies.Dependency), len(split.Dependencies.Dependency))
	}
}

func TestPOMDefaultNamespacePropertiesCharge(t *testing.T) {
	keys := func(n int) string {
		var b strings.Builder
		for i := 0; i < n; i++ {
			b.WriteString("<foo")
			b.WriteByte('A' + byte(i%26))
			b.WriteByte('0' + byte(i/26%10))
			b.WriteString(">v</foo")
			b.WriteByte('A' + byte(i%26))
			b.WriteByte('0' + byte(i/26%10))
			b.WriteString(">")
		}
		return b.String()
	}
	body := func(xmlns string, n int) string {
		ns := ""
		if xmlns != "" {
			ns = ` xmlns="` + xmlns + `"`
		}
		return `<project` + ns + `><groupId>g</groupId><artifactId>a</artifactId><version>1</version><properties>` + keys(n) + `</properties><dependencies><dependency><groupId>g</groupId><artifactId>d</artifactId><version>1</version></dependency></dependencies></project>`
	}
	high, err := (budget.Limits{MaxResultBytes: 8 << 20}).Normalize()
	if err != nil {
		t.Fatal(err)
	}
	charge := func(xml string) (int64, pomXML) {
		t.Helper()
		ctx := budget.Bind(context.Background(), high)
		var out pomXML
		if err := xmlrecord.Decode(ctx, strings.NewReader(xml), &out); err != nil {
			t.Fatal(err)
		}
		return budget.From(ctx).ResultBytes(), out
	}
	noneXML := body("", 8)
	mavenXML := body("http://maven.apache.org/POM/4.0.0", 8)
	cNone, pNone := charge(noneXML)
	cMaven, pMaven := charge(mavenXML)
	cNone40, _ := charge(body("", 40))
	cMaven40, pMaven40 := charge(body("http://maven.apache.org/POM/4.0.0", 40))
	t.Logf("charge none8=%d maven8=%d none40=%d maven40=%d keys40=%d", cNone, cMaven, cNone40, cMaven40, len(pMaven40.Properties))
	if pNone.Properties["fooA0"] != "v" || pMaven.Properties["fooA0"] != "v" {
		t.Fatalf("property values: none=%v maven=%v", pNone.Properties, pMaven.Properties)
	}
	if len(pNone.Properties) != 8 || len(pMaven.Properties) != 8 {
		t.Fatalf("distinct keys: none=%d maven=%d", len(pNone.Properties), len(pMaven.Properties))
	}
	if pNone.Dependencies.Dependency[0].ArtifactID != "d" || pMaven.Dependencies.Dependency[0].ArtifactID != "d" {
		t.Fatal("dependency lost under default xmlns")
	}
	dNone := cNone40 - cNone
	dMaven := cMaven40 - cMaven
	if dMaven*2 < dNone {
		t.Fatalf("default xmlns properties map growth undercharged: none Δ=%d maven Δ=%d (8→40 keys)", dNone, dMaven)
	}

	low, err := (budget.Limits{MaxResultBytes: 8000}).Normalize()
	if err != nil {
		t.Fatal(err)
	}
	stale := pomXML{GroupId: "stale"}
	err = xmlrecord.Decode(budget.Bind(context.Background(), low), strings.NewReader(body("http://maven.apache.org/POM/4.0.0", 80)), &stale)
	if err == nil || !errors.Is(err, scanerr.ErrResourceLimit) {
		t.Fatalf("namespaced properties dest: err=%v props=%d", err, len(stale.Properties))
	}
	if stale.GroupId != "stale" || stale.Properties != nil {
		t.Fatalf("low budget changed namespaced POM: %+v", stale)
	}
}

// Guard the fixed production schema against DTO drift. Reflection belongs in
// this test only: no input or caller can request general layout binding.
func TestFixedXMLSchemaLayout(t *testing.T) {
	want := map[string]int64{
		"licenses>license": xmlrecord.POMLicenseBytes,
		"modules>module":   xmlrecord.POMModuleBytes,
		"dependencyManagement>dependencies>dependency":                      xmlrecord.POMDependencyBytes,
		"dependencyManagement>dependencies>dependency>exclusions>exclusion": xmlrecord.POMExclusionBytes,
		"dependencies>dependency":                                           xmlrecord.POMDependencyBytes,
		"dependencies>dependency>exclusions>exclusion":                      xmlrecord.POMExclusionBytes,
		"repositories>repository":                                           xmlrecord.POMRepositoryBytes,
	}
	var walk func(reflect.Type, string)
	maps := 0
	walk = func(typ reflect.Type, path string) {
		for i := 0; i < typ.NumField(); i++ {
			f := typ.Field(i)
			tag := strings.Split(f.Tag.Get("xml"), ",")[0]
			if tag == "-" {
				continue
			}
			fp := path
			if tag != "" {
				if fp != "" {
					fp += ">"
				}
				fp += tag
			}
			switch f.Type.Kind() {
			case reflect.Slice:
				max, ok := want[fp]
				if !ok || int64(f.Type.Elem().Size()) > max {
					t.Fatalf("unaccounted slice %s size=%d bound=%d", fp, f.Type.Elem().Size(), max)
				}
				delete(want, fp)
				if f.Type.Elem().Kind() == reflect.Struct {
					walk(f.Type.Elem(), fp)
				}
			case reflect.Struct:
				walk(f.Type, fp)
			case reflect.Map:
				if fp != "properties" || f.Type.Key().Kind() != reflect.String || f.Type.Elem().Kind() != reflect.String {
					t.Fatalf("unaccounted map %s", fp)
				}
				maps++
			case reflect.Pointer, reflect.Interface:
				t.Fatalf("unaccounted indirection %s", fp)
			}
		}
	}
	walk(reflect.TypeOf(pomXML{}), "")
	if len(want) != 0 || maps != 1 {
		t.Fatalf("stale schema slices=%v maps=%d", want, maps)
	}
}

func TestPOMRejectsUncountedDependencyWrapper(t *testing.T) {
	for _, prefix := range []string{"", ` xmlns="http://maven.apache.org/POM/4.0.0"`} {
		raw := `<project` + prefix + `><dependencies><wrapper><dependency><groupId>g</groupId><artifactId>a</artifactId><version>1</version></dependency></wrapper></dependencies></project>`
		var out pomXML
		err := xmlrecord.Decode(context.Background(), strings.NewReader(raw), &out)
		if scanerr.CodeOf(err) != scanerr.UnsupportedSyntax || len(out.Dependencies.Dependency) != 0 {
			t.Fatalf("uncounted wrapper: %+v %v", out, err)
		}
		raw = strings.ReplaceAll(strings.ReplaceAll(raw, "<wrapper>", ""), "</wrapper>", "")
		if err := xmlrecord.Decode(context.Background(), strings.NewReader(raw), &out); err != nil || len(out.Dependencies.Dependency) != 1 || out.Dependencies.Dependency[0].ArtifactID != "a" {
			t.Fatalf("direct dependency: %+v %v", out, err)
		}
	}
	var missing *pomXML
	if err := xmlrecord.Decode(context.Background(), strings.NewReader(`<project/>`), missing); scanerr.CodeOf(err) != scanerr.UnsupportedSyntax {
		t.Fatalf("nil POM: %v", err)
	}
}
