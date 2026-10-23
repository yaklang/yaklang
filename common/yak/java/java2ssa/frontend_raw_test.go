package java2ssa

import "testing"

func TestFrontendRawRejectsMalformedDecompilerOutput(t *testing.T) {
	cache := CreateBuilder().(*SSABuilder).GetAntlrCache()
	defer cache.Clear()
	for _, src := range []string{
		`class Sample { void run((Class)(Class var1)) {} }`,
		`class Sample { void run() { try {} catch (Exception e) {} else {} } }`,
		`class Sample { void run() { call("unterminated); } }`,
	} {
		if _, err := FrontendRaw(src, cache); err == nil {
			t.Fatalf("raw parser accepted malformed producer output: %s", src)
		}
	}
	if _, err := FrontendRaw(`class Sample { void run(Class<?> type) { try { call(type); } catch (Exception e) { if (type != null) throw e; else call(null); } } }`, cache); err != nil {
		t.Fatal(err)
	}
}
