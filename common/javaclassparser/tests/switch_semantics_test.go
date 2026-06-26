package tests

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/javaclassparser"
)

// switchBatterySource exercises the switch shapes that stress CFG reconstruction:
//   - dense tableswitch and sparse lookupswitch
//   - fallthrough chains (cases without break) and multi-label cases (case A: case B:)
//   - string switch (compiled as a hashCode tableswitch + equals guard + second switch)
//   - char switch, negative-label switch (signed lookupswitch/tableswitch keys)
//   - nested switch and switch inside a loop
// Every method folds a value into the deterministic fingerprint, so the original and the
// decompiled+recompiled class are compared by execution; a dropped/merged case arm or a corrupted
// fallthrough silently changes behaviour and is caught here.
const switchBatterySource = `public class SwitchBattery {
    static int dense(int x){
        switch(x){ case 0: return 10; case 1: return 11; case 2: return 12; default: return -1; }
    }
    static int sparse(int x){
        switch(x){ case 1: return 1; case 100: return 2; case 10000: return 3; default: return 0; }
    }
    static int fall(int x){
        int r=0;
        switch(x){ case 0: r+=1; case 1: r+=2; case 2: r+=4; break; case 3: r+=8; default: r+=16; }
        return r;
    }
    static int multiLabel(int x){
        switch(x){ case 1: case 2: case 3: return 100; case 4: case 5: return 200; default: return 0; }
    }
    static String stringSwitch(String s){
        switch(s){ case "a": return "AA"; case "bb": return "BB"; case "ccc": return "CC"; default: return "ZZ"; }
    }
    static int charSwitch(char c){
        switch(c){ case 'a': return 1; case 'z': return 26; case '0': return 100; default: return -1; }
    }
    static int negative(int x){
        switch(x){ case -5: return 1; case -1: return 2; case 0: return 3; case 7: return 4; default: return 9; }
    }
    static int nested(int x, int y){
        switch(x){
            case 0: switch(y){ case 0: return 1; default: return 2; }
            case 1: return 3;
            default: return 4;
        }
    }
    static int loopSwitch(int n){
        int s=0;
        for(int i=0;i<n;i++){
            switch(i%3){ case 0: s+=1; break; case 1: s+=10; break; default: s+=100; }
        }
        return s;
    }

    public static void main(String[] z){
        StringBuilder sb = new StringBuilder();
        sb.append(dense(1)).append(",");
        sb.append(sparse(100)).append(",");
        sb.append(fall(1)).append(",");
        sb.append(multiLabel(3)).append(",");
        sb.append(stringSwitch("bb")).append(",");
        sb.append(charSwitch('z')).append(",");
        sb.append(negative(-5)).append(",");
        sb.append(nested(0,5)).append(",");
        sb.append(loopSwitch(6));
        System.out.println(sb.toString());
    }
}`

// hardSwitchSource collects switch-inside-loop shapes whose CFG reconstruction is not yet complete:
//   - loopSwitchTail: ordinary code after the switch in the loop body, so every break arm has to merge
//     to the post-switch statement and then continue the loop (currently "invalid if merge node").
//   - loopSwitchContinue: a `continue` issued from inside a case targets the loop increment edge,
//     interleaving the switch arm with the loop back-edge ("not found simulation stack for opcode 13").
// These degrade to a safe stub; the guard only requires that javac accepts the decompiled output, so a
// stub passes while a corrupted body is caught. Promote to switchBatterySource once fixed.
const hardSwitchSource = `public class HardSwitch {
    static int loopSwitchTail(int n){
        int s=0;
        for(int i=0;i<n;i++){
            switch(i%3){ case 0: s+=1; break; case 1: s+=10; break; default: s+=100; }
            s+=1000;
        }
        return s;
    }
    static int loopSwitchContinue(int n){
        int s=0;
        for(int i=0;i<n;i++){
            switch(i%3){ case 0: s+=1; break; case 1: s+=10; continue; default: s+=100; }
            s+=1000;
        }
        return s;
    }
    public static void main(String[] z){
        System.out.println(loopSwitchTail(6)+","+loopSwitchContinue(6));
    }
}`

// TestSwitchSemanticsRoundTrip compiles+runs the switch battery for a ground-truth fingerprint, then
// decompiles, recompiles, and runs the result, asserting identical output. switch/tableswitch is a
// frequent source of CFG corruption (collapsed arms, broken fallthrough, lost default), so an
// execution-level check is the most reliable guard. Gated on javac/java.
func TestSwitchSemanticsRoundTrip(t *testing.T) {
	javac, err1 := exec.LookPath("javac")
	java, err2 := exec.LookPath("java")
	if err1 != nil || err2 != nil {
		t.Skip("javac/java not available; skipping switch semantics round-trip")
	}

	origDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(origDir, "SwitchBattery.java"), []byte(switchBatterySource), 0644); err != nil {
		t.Fatalf("write source: %v", err)
	}
	golden, ok := runJava(t, javac, java, origDir, "SwitchBattery")
	if !ok {
		t.Fatalf("failed to compile/run the original battery")
	}
	t.Logf("golden fingerprint: %s", golden)

	raw, err := os.ReadFile(filepath.Join(origDir, "SwitchBattery.class"))
	if err != nil {
		t.Fatalf("read compiled class: %v", err)
	}
	decompiled, err := javaclassparser.Decompile(raw)
	if err != nil {
		t.Fatalf("decompile failed: %v", err)
	}
	if strings.Contains(decompiled, javaclassparser.DecompileStubMarker) {
		t.Fatalf("a switch method degraded to a stub; cannot verify semantics\n----- decompiled -----\n%s", decompiled)
	}
	src := regexp.MustCompile(`(?m)^package\s+[^;]+;\s*$`).ReplaceAllString(decompiled, "")

	rtDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(rtDir, "SwitchBattery.java"), []byte(src), 0644); err != nil {
		t.Fatalf("write decompiled source: %v", err)
	}
	got, ok := runJava(t, javac, java, rtDir, "SwitchBattery")
	if !ok {
		t.Fatalf("decompiled battery failed to compile/run\n----- decompiled -----\n%s", src)
	}
	if got != golden {
		t.Fatalf("switch semantics diverged after decompilation\n  golden: %s\n  got:    %s\n----- decompiled -----\n%s", golden, got, src)
	}
	t.Logf("switch semantics preserved: %s", got)
}

// TestSwitchHardCasesNoCorruption guards the switch shapes that still degrade to a stub (a continue
// from inside a switch case targeting the loop increment). It decompiles HardSwitch and requires only
// that javac ACCEPTS the decompiled source: a safe stub compiles cleanly and passes, whereas a
// corrupted, uncompilable body is rejected. Gated on javac.
func TestSwitchHardCasesNoCorruption(t *testing.T) {
	javac, err := exec.LookPath("javac")
	if err != nil {
		t.Skip("javac not available; skipping hard switch no-corruption guard")
	}

	origDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(origDir, "HardSwitch.java"), []byte(hardSwitchSource), 0644); err != nil {
		t.Fatalf("write source: %v", err)
	}
	if out, err := exec.Command(javac, "-J-Duser.language=en", "-d", origDir, filepath.Join(origDir, "HardSwitch.java")).CombinedOutput(); err != nil {
		t.Fatalf("failed to compile the original hard battery: %v\n%s", err, out)
	}

	raw, err := os.ReadFile(filepath.Join(origDir, "HardSwitch.class"))
	if err != nil {
		t.Fatalf("read compiled class: %v", err)
	}
	decompiled, err := javaclassparser.Decompile(raw)
	if err != nil {
		t.Fatalf("decompile failed: %v", err)
	}
	src := regexp.MustCompile(`(?m)^package\s+[^;]+;\s*$`).ReplaceAllString(decompiled, "")

	rtDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(rtDir, "HardSwitch.java"), []byte(src), 0644); err != nil {
		t.Fatalf("write decompiled source: %v", err)
	}
	if out, err := exec.Command(javac, "-J-Duser.language=en", "-d", rtDir, filepath.Join(rtDir, "HardSwitch.java")).CombinedOutput(); err != nil {
		t.Fatalf("decompiled hard switch battery does not compile (corruption, not a safe stub):\n%s\n----- decompiled -----\n%s", out, src)
	}
	t.Logf("hard switch cases decompiled without corruption (stub-or-correct)")
}
