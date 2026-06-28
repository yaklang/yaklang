package tests

import (
	"archive/zip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/javaclassparser"
)

// cross_comparison_v2_test.go is the second-generation, stronger PK harness. It extends the
// original 4-axis comparison with the dimensions the maintainer asked for:
//
//   - TWO Yak modes measured side by side:
//       * "yak-syntax"  -> EnableDecompileSyntaxValidation = true  (default; ANTLR safety net
//                          guarantees the emitted Java parses, degrading malformed members to
//                          marked stubs instead of leaking unparseable text).
//       * "yak-raw"     -> EnableDecompileSyntaxValidation = false (no safety net; fastest, but
//                          a malformed member can leak unparseable Java).
//     Reporting both exposes the safety-net trade-off: completeness vs. guaranteed-parseable
//     output vs. throughput.
//
//   - REAL "decompile -> repackage to jar -> externally callable" usability:
//       every tool's recompiled .class files are OVERLAID back onto the original jar (binary
//       names match, so flat `Outer$Inner` recompiled classes are binary-compatible), producing
//       a rebuilt jar. An external JVM probe (LinkAll) then load+LINKS (verifies, without running
//       <clinit>) every class in the rebuilt jar. verify_fail counts bytecode the JVM verifier
//       rejects — the strongest automatic correctness signal short of execution.
//
//   - A semantic DIFFERENTIAL CALL for guava: the rebuilt-from-Yak jar is put ahead of the
//     original on the classpath and a probe runs real guava algorithms; the fingerprint must be
//     byte-identical to the original jar's.
//
//   - Cross/concurrent performance for every tool.
//
// OPT-IN: runs only when CROSS_PK=1 with CFR_JAR and VINEFLOWER_JAR set. Vineflower is the
// maintained Fernflower lineage (Fernflower -> Quiltflower -> Vineflower) and stands in as the
// Fernflower-family representative; this is stated in the report.

// --- v2 report model -------------------------------------------------------

type pk2Mode struct {
	Tool         string  `json:"tool"`
	OK           int     `json:"ok"`
	Stub         int     `json:"stub"`
	Err          int     `json:"err"`
	SerialSec    float64 `json:"serial_seconds"`
	ConcSec      float64 `json:"concurrent_seconds"`
	ClassPerSec  float64 `json:"classes_per_sec_concurrent"`
	RecompUnits  int     `json:"recompile_units"`
	RecompOK     int     `json:"recompile_ok"`
	RecompFail   int     `json:"recompile_fail"`
	RecompMissDp int     `json:"recompile_missing_dep"`
	RecompDecErr int     `json:"recompile_decompiler_err"`
	JarClasses   int     `json:"jar_classes"`
	Overlaid     int     `json:"overlaid_classes"`
	Linked       int     `json:"linked"`
	VerifyFail   int     `json:"verify_fail"`
	OtherFail    int     `json:"other_fail"`
	Sample       []string `json:"sample_fails,omitempty"`
}

type pk2Jar struct {
	Jar        string    `json:"jar"`
	Label      string    `json:"label"`
	ClassCount int       `json:"class_count"`
	Modes      []pk2Mode `json:"modes"`
	DiffCall   string    `json:"diff_call,omitempty"`
	Notes      []string  `json:"notes,omitempty"`
}

type pk2Report struct {
	GeneratedAt string   `json:"generated_at"`
	Java        string   `json:"java"`
	GoVersion   string   `json:"go_version"`
	NumCPU      int      `json:"num_cpu"`
	Workers     int      `json:"workers"`
	CFRVersion  string   `json:"cfr_version"`
	VFVersion   string   `json:"vineflower_version"`
	Jars        []pk2Jar `json:"jars"`
}

// --- per-jar dependency classpath (best-effort, for honest recompile/link) --

func pk2JarDeps(jar string) string {
	m2 := os.Getenv("HOME") + "/.m2/repository"
	base := filepath.Base(jar)
	join := func(parts ...string) string { return strings.Join(parts, string(os.PathListSeparator)) }
	switch {
	case strings.HasPrefix(base, "guava"):
		return join(
			m2+"/org/checkerframework/checker-compat-qual/2.5.5/checker-compat-qual-2.5.5.jar",
			m2+"/com/google/errorprone/error_prone_annotations/2.3.4/error_prone_annotations-2.3.4.jar",
			m2+"/com/google/j2objc/j2objc-annotations/1.3/j2objc-annotations-1.3.jar",
			m2+"/com/google/guava/failureaccess/1.0.1/failureaccess-1.0.1.jar",
			m2+"/com/google/code/findbugs/jsr305/3.0.2/jsr305-3.0.2.jar",
		)
	case strings.HasPrefix(base, "spring-beans"):
		return join(
			m2+"/org/springframework/spring-core/5.3.33/spring-core-5.3.33.jar",
			m2+"/org/springframework/spring-jcl/5.3.33/spring-jcl-5.3.33.jar",
		)
	case strings.HasPrefix(base, "spring-core"):
		return join(m2 + "/org/springframework/spring-jcl/5.3.33/spring-jcl-5.3.33.jar")
	case strings.HasPrefix(base, "commons-text"):
		return join(m2 + "/org/apache/commons/commons-lang3/3.12.0/commons-lang3-3.12.0.jar")
	case strings.HasPrefix(base, "jackson-databind"):
		return join(
			m2+"/com/fasterxml/jackson/core/jackson-core/2.9.6/jackson-core-2.9.6.jar",
			m2+"/com/fasterxml/jackson/core/jackson-annotations/2.9.0/jackson-annotations-2.9.0.jar",
		)
	}
	return ""
}

// existing deps that are actually present on disk (skip missing so javac -cp stays valid).
func pk2ExistingDeps(deps string) string {
	if deps == "" {
		return ""
	}
	var out []string
	for _, p := range strings.Split(deps, string(os.PathListSeparator)) {
		if p == "" {
			continue
		}
		if _, err := os.Stat(p); err == nil {
			out = append(out, p)
		}
	}
	return strings.Join(out, string(os.PathListSeparator))
}

// --- the embedded link/verify probe ----------------------------------------

// pk2LinkAllSource is a tiny JVM probe: for every class in the target jar it does a
// load+LINK (resolve=true) WITHOUT initialization, so the bytecode verifier runs but no
// static initializer (which could System.exit / spawn threads / hang) is triggered. This makes
// the "externally callable" check both strong (the verifier rejects malformed bytecode) and
// safe (no arbitrary <clinit> side effects).
const pk2LinkAllSource = `import java.io.*;
import java.net.*;
import java.util.*;
import java.util.jar.*;

public class LinkAll extends URLClassLoader {
    public LinkAll(URL[] u, ClassLoader p) { super(u, p); }
    public Class<?> link(String n) throws ClassNotFoundException { return loadClass(n, true); }
    public static void main(String[] a) throws Exception {
        String jar = a[0];
        String parentCp = a.length > 1 ? a[1] : "";
        List<URL> us = new ArrayList<>();
        us.add(new File(jar).toURI().toURL());
        List<URL> ps = new ArrayList<>();
        for (String p : parentCp.split(File.pathSeparator)) if (!p.isEmpty()) ps.add(new File(p).toURI().toURL());
        URLClassLoader parent = new URLClassLoader(ps.toArray(new URL[0]), LinkAll.class.getClassLoader());
        LinkAll cl = new LinkAll(us.toArray(new URL[0]), parent);
        int total = 0, linked = 0, verify = 0, other = 0;
        try (JarFile jf = new JarFile(jar)) {
            Enumeration<JarEntry> e = jf.entries();
            while (e.hasMoreElements()) {
                JarEntry je = e.nextElement();
                String n = je.getName();
                if (!n.endsWith(".class") || n.contains("module-info") || n.contains("package-info")) continue;
                String cn = n.substring(0, n.length() - 6).replace('/', '.');
                total++;
                try { cl.link(cn); linked++; }
                catch (VerifyError | ClassFormatError ve) { verify++; }
                catch (Throwable t) { other++; }
            }
        }
        System.out.println("total=" + total + " linked=" + linked + " verify_fail=" + verify + " other_fail=" + other);
    }
}
`

// pk2CompileProbe compiles LinkAll.java once and returns its class dir.
func pk2CompileProbe(t *testing.T, javac string) (string, bool) {
	dir := filepath.Join(os.TempDir(), "pk2-linkall")
	_ = os.MkdirAll(dir, 0o755)
	src := filepath.Join(dir, "LinkAll.java")
	if err := os.WriteFile(src, []byte(pk2LinkAllSource), 0o644); err != nil {
		t.Logf("write LinkAll: %v", err)
		return "", false
	}
	out, err := exec.Command(javac, "-d", dir, src).CombinedOutput()
	if err != nil {
		t.Logf("compile LinkAll failed: %v\n%s", err, out)
		return "", false
	}
	return dir, true
}

// pk2BuildOverlayJar copies the original jar and overlays every recompiled .class found under
// classesDir (matched by relative path = jar entry name), returning the rebuilt jar's path and
// the number of .class entries it contains. Flat recompiled `Outer$Inner.class` files carry the
// exact binary name of the original nested class, so they are drop-in binary replacements.
func pk2BuildOverlayJar(origJar, classesDir, outJar string) (total int, overlaid int, err error) {
	// Map jar-entry path -> recompiled bytes.
	overlay := map[string][]byte{}
	_ = filepath.Walk(classesDir, func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(p, ".class") {
			return nil
		}
		rel, rerr := filepath.Rel(classesDir, p)
		if rerr != nil {
			return nil
		}
		b, rerr := os.ReadFile(p)
		if rerr == nil {
			overlay[filepath.ToSlash(rel)] = b
		}
		return nil
	})

	zr, oerr := zip.OpenReader(origJar)
	if oerr != nil {
		return 0, 0, oerr
	}
	defer zr.Close()
	of, cerr := os.Create(outJar)
	if cerr != nil {
		return 0, 0, cerr
	}
	defer of.Close()
	zw := zip.NewWriter(of)
	defer zw.Close()

	seen := map[string]bool{}
	for _, f := range zr.File {
		name := f.Name
		seen[name] = true
		w, werr := zw.Create(name)
		if werr != nil {
			continue
		}
		if repl, ok := overlay[name]; ok {
			_, _ = w.Write(repl)
			overlaid++
		} else {
			rc, ferr := f.Open()
			if ferr != nil {
				continue
			}
			_, _ = io.Copy(w, rc)
			rc.Close()
		}
		if strings.HasSuffix(name, ".class") {
			total++
		}
	}
	// Add recompiled classes that did not exist in the original (rare).
	for name, b := range overlay {
		if seen[name] {
			continue
		}
		w, werr := zw.Create(name)
		if werr != nil {
			continue
		}
		_, _ = w.Write(b)
		total++
		overlaid++
	}
	return total, overlaid, nil
}

// pk2LinkVerify runs the LinkAll probe over rebuiltJar with deps on the parent classpath and a
// hard wall-clock timeout. It returns (linked, verifyFail, otherFail, total).
func pk2LinkVerify(t *testing.T, probeDir, rebuiltJar, deps string) (linked, verifyFail, otherFail, total int) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "java", "-cp", probeDir, "LinkAll", rebuiltJar, deps)
	out, err := cmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		t.Logf("LinkAll timed out on %s", filepath.Base(rebuiltJar))
		return
	}
	line := strings.TrimSpace(string(out))
	_ = err
	for _, ln := range strings.Split(line, "\n") {
		if !strings.HasPrefix(ln, "total=") {
			continue
		}
		for _, kv := range strings.Fields(ln) {
			parts := strings.SplitN(kv, "=", 2)
			if len(parts) != 2 {
				continue
			}
			n, _ := strconv.Atoi(parts[1])
			switch parts[0] {
			case "total":
				total = n
			case "linked":
				linked = n
			case "verify_fail":
				verifyFail = n
			case "other_fail":
				otherFail = n
			}
		}
	}
	return
}

// --- a Yak full pass (one syntax-validation mode) --------------------------

// pk2YakPass decompiles every class in the given mode, returns completeness + timings + the
// source units for the classes that produced compilable output.
func pk2YakPass(t *testing.T, classes map[string][]byte, syntaxNet bool, workers int) (mode pk2Mode, units map[string]string) {
	javaclassparser.EnableDecompileSyntaxValidation = syntaxNet
	defer func() { javaclassparser.EnableDecompileSyntaxValidation = true }()

	serialRes, serialDur := decompileYakSerial(classes)
	_, concDur := decompileYakConcurrent(classes, workers)

	units = map[string]string{}
	ok, stub, errc := 0, 0, 0
	for _, c := range serialRes {
		switch {
		case c.Ok:
			ok++
			units[c.Name] = mustGetYakSource(t, c.Name, classes[c.Name])
		case c.Stub:
			stub++
			// stubs still parse and usually compile; include them so the jar is whole.
			units[c.Name] = mustGetYakSource(t, c.Name, classes[c.Name])
		default:
			errc++
		}
	}
	tool := "yak-syntax"
	if !syntaxNet {
		tool = "yak-raw"
	}
	mode = pk2Mode{
		Tool:        tool,
		OK:          ok,
		Stub:        stub,
		Err:         errc,
		SerialSec:   serialDur.Seconds(),
		ConcSec:     concDur.Seconds(),
		ClassPerSec: float64(len(classes)) / safeDiv(concDur.Seconds()),
	}
	return
}

// --- top-level v2 test -----------------------------------------------------

func TestYakDecompilerCrossComparisonV2(t *testing.T) {
	if os.Getenv("CROSS_PK") != "1" {
		t.Skip("v2 cross-comparison PK is opt-in; set CROSS_PK=1 CFR_JAR=... VINEFLOWER_JAR=... to run")
	}
	if os.Getenv("CFR_JAR") == "" || os.Getenv("VINEFLOWER_JAR") == "" {
		t.Skip("CFR_JAR and VINEFLOWER_JAR must both be set")
	}
	javaBin, err := exec.LookPath("java")
	if err != nil {
		t.Skip("java not found")
	}
	javac, err := exec.LookPath("javac")
	if err != nil {
		t.Skip("javac not found")
	}

	outDir := os.Getenv("PK_OUT")
	if outDir == "" {
		outDir = "/tmp/yak-decompiler-cross-comparison-v2"
	}
	_ = os.MkdirAll(outDir, 0o755)
	workers := runtime.NumCPU()
	if w := os.Getenv("YAK_WORKERS"); w != "" {
		fmt.Sscanf(w, "%d", &workers)
	}
	if workers < 1 {
		workers = 1
	}
	probeDir, probeOK := pk2CompileProbe(t, javac)

	jars := pkCorpusJars(t)
	if mj := os.Getenv("PK_MAX_JARS"); mj != "" {
		if n, e := strconv.Atoi(mj); e == nil && n > 0 && n < len(jars) {
			jars = jars[:n]
		}
	}
	t.Logf("v2 PK: %d jars, workers=%d, probe=%v, out=%s", len(jars), workers, probeOK, outDir)

	report := pk2Report{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Java:        javaVersion(javaBin),
		GoVersion:   runtime.Version(),
		NumCPU:      runtime.NumCPU(),
		Workers:     workers,
		CFRVersion:  cfrVersion(),
		VFVersion:   vineflowerVersion(),
	}

	for _, jar := range jars {
		if _, err := os.Stat(jar); err != nil {
			t.Logf("SKIP missing jar: %s", jar)
			continue
		}
		report.Jars = append(report.Jars, runPK2Jar(t, jar, javac, workers, outDir, probeDir, probeOK))
	}

	pk2WriteJSON(t, outDir, report)
	pk2WriteMarkdown(t, outDir, report)
}

func runPK2Jar(t *testing.T, jar, javac string, workers int, outDir, probeDir string, probeOK bool) pk2Jar {
	label := pkJarLabel(jar)
	work := filepath.Join(outDir, label)
	_ = os.MkdirAll(work, 0o755)
	t.Logf("=== v2 PK %s ===", label)

	classes := readJarClassBytes(t, jar)
	res := pk2Jar{Jar: jar, Label: label, ClassCount: len(classes)}

	deps := pk2ExistingDeps(pk2JarDeps(jar))
	cp := jar
	if deps != "" {
		cp = cp + string(os.PathListSeparator) + deps
	}
	if extra := os.Getenv("PK_CP"); extra != "" {
		cp = cp + string(os.PathListSeparator) + extra
	}

	// --- Yak two modes ---
	for _, syntaxNet := range []bool{true, false} {
		mode, units := pk2YakPass(t, classes, syntaxNet, workers)
		toolDir := "yak-syntax"
		if !syntaxNet {
			toolDir = "yak-raw"
		}
		srcDir := filepath.Join(work, toolDir+"-src")
		_ = os.RemoveAll(srcDir)
		_ = os.MkdirAll(srcDir, 0o755)
		nUnits, _ := writeYakUnits(srcDir, units)
		clsDir := filepath.Join(work, toolDir+"-classes")
		_ = os.RemoveAll(clsDir)
		fails := recompileJavaDir(t, javac, cp, srcDir, clsDir)
		rc := summarizeRecompile(toolDir, nUnits, fails)
		mode.RecompUnits = rc.Units
		mode.RecompOK = rc.Compiled
		mode.RecompFail = rc.Failed
		mode.RecompMissDp = rc.MissingDep
		mode.RecompDecErr = rc.Decompiler
		mode.Sample = rc.SampleFails
		// package + link/verify
		if probeOK {
			rebuilt := filepath.Join(work, toolDir+"-rebuilt.jar")
			jc, ov, perr := pk2BuildOverlayJar(jar, clsDir, rebuilt)
			if perr != nil {
				t.Logf("overlay jar %s: %v", toolDir, perr)
			} else {
				mode.JarClasses = jc
				mode.Overlaid = ov
				linked, vf, of, _ := pk2LinkVerify(t, probeDir, rebuilt, deps)
				mode.Linked = linked
				mode.VerifyFail = vf
				mode.OtherFail = of
			}
		}
		res.Modes = append(res.Modes, mode)
	}

	// --- CFR + Vineflower: decompile (timed), recompile, package, link ---
	for _, tool := range []string{"cfr", "vineflower"} {
		srcDir := filepath.Join(work, tool)
		dur, files := timeExternalDecompile(t, tool, jar, srcDir)
		clsDir := filepath.Join(work, tool+"-classes")
		_ = os.RemoveAll(clsDir)
		fails := recompileJavaDir(t, javac, cp, srcDir, clsDir)
		rc := summarizeRecompile(tool, files, fails)
		mode := pk2Mode{
			Tool:         tool,
			OK:           files,
			ConcSec:      dur.Seconds(),
			SerialSec:    dur.Seconds(),
			ClassPerSec:  float64(files) / safeDiv(dur.Seconds()),
			RecompUnits:  rc.Units,
			RecompOK:     rc.Compiled,
			RecompFail:   rc.Failed,
			RecompMissDp: rc.MissingDep,
			RecompDecErr: rc.Decompiler,
			Sample:       rc.SampleFails,
		}
		if probeOK {
			rebuilt := filepath.Join(work, tool+"-rebuilt.jar")
			jc, ov, perr := pk2BuildOverlayJar(jar, clsDir, rebuilt)
			if perr == nil {
				mode.JarClasses = jc
				mode.Overlaid = ov
				linked, vf, of, _ := pk2LinkVerify(t, probeDir, rebuilt, deps)
				mode.Linked = linked
				mode.VerifyFail = vf
				mode.OtherFail = of
			}
		}
		res.Modes = append(res.Modes, mode)
	}

	// --- differential call (guava only, semantic) ---
	if strings.HasPrefix(label, "guava") {
		res.DiffCall = pk2GuavaDiffCall(t, javac, jar, work, deps)
	}

	return res
}

// --- guava differential call ----------------------------------------------

const pk2GuavaProbeSource = `import com.google.common.math.IntMath;
import com.google.common.math.LongMath;
import com.google.common.primitives.Ints;
import com.google.common.primitives.Longs;
import com.google.common.primitives.UnsignedInts;
import com.google.common.primitives.UnsignedLongs;
import com.google.common.base.Ascii;
import com.google.common.base.Strings;
import java.math.RoundingMode;

public class GuavaProbeV2 {
    public static void main(String[] a) {
        StringBuilder sb = new StringBuilder();
        sb.append("IntMath.gcd=").append(IntMath.gcd(12, 18)).append(',').append(IntMath.gcd(7, 13)).append(';');
        sb.append("IntMath.pow=").append(IntMath.pow(3, 7)).append(';');
        sb.append("IntMath.log2=").append(IntMath.log2(513, RoundingMode.CEILING)).append(';');
        sb.append("IntMath.sqrt=").append(IntMath.sqrt(1000, RoundingMode.FLOOR)).append(';');
        sb.append("LongMath.binomial=").append(LongMath.binomial(20, 5)).append(';');
        sb.append("LongMath.gcd=").append(LongMath.gcd(462L, 1071L)).append(';');
        sb.append("Ints.max=").append(Ints.max(3, 9, -2, 7)).append(';');
        sb.append("Ints.join=").append(Ints.join("-", 5, -3, 9, 0)).append(';');
        sb.append("Longs.max=").append(Longs.max(3L, 9L, -2L)).append(';');
        sb.append("UnsignedInts.toString=").append(UnsignedInts.toString(-1, 16)).append(';');
        sb.append("UnsignedLongs.toString=").append(UnsignedLongs.toString(-1L, 16)).append(',').append(UnsignedLongs.toString(-8L, 7)).append(';');
        sb.append("UnsignedLongs.divide=").append(UnsignedLongs.divide(-1L, 7L)).append(';');
        sb.append("Ascii.toUpperCase=").append(Ascii.toUpperCase("Hello, Guava")).append(';');
        sb.append("Ascii.truncate=").append(Ascii.truncate("Hello, Guava World", 10, "...")).append(';');
        sb.append("Strings.repeat=").append(Strings.repeat("ab", 5)).append(';');
        sb.append("Strings.padStart=").append(Strings.padStart("7", 4, '0')).append(';');
        sb.append("Strings.commonPrefix=").append(Strings.commonPrefix("flower", "flow")).append(';');
        System.out.println(sb.toString());
    }
}
`

// pk2GuavaDiffCall compiles GuavaProbeV2 against the original guava, runs it once against the
// original jar and once with the Yak-rebuilt jar shadowing it; returns a human-readable verdict.
func pk2GuavaDiffCall(t *testing.T, javac, jar, work, deps string) string {
	rebuilt := filepath.Join(work, "yak-syntax-rebuilt.jar")
	if _, err := os.Stat(rebuilt); err != nil {
		return "skip (no rebuilt jar)"
	}
	probeDir := filepath.Join(work, "guava-probe")
	_ = os.MkdirAll(probeDir, 0o755)
	src := filepath.Join(probeDir, "GuavaProbeV2.java")
	if err := os.WriteFile(src, []byte(pk2GuavaProbeSource), 0o644); err != nil {
		return "skip (write probe: " + err.Error() + ")"
	}
	cpOrig := jar
	if deps != "" {
		cpOrig = cpOrig + string(os.PathListSeparator) + deps
	}
	if out, err := exec.Command(javac, "-cp", cpOrig, "-d", probeDir, src).CombinedOutput(); err != nil {
		return "skip (probe compile failed: " + firstLine(string(out)) + ")"
	}
	run := func(classpath string) (string, error) {
		out, err := exec.Command("java", "-cp", classpath, "GuavaProbeV2").CombinedOutput()
		return strings.TrimSpace(string(out)), err
	}
	origCP := strings.Join([]string{probeDir, jar, deps}, string(os.PathListSeparator))
	rebuiltCP := strings.Join([]string{probeDir, rebuilt, deps}, string(os.PathListSeparator))
	golden, gerr := run(origCP)
	got, rerr := run(rebuiltCP)
	if gerr != nil {
		return "FAIL original probe: " + firstLine(golden)
	}
	if rerr != nil {
		return "FAIL rebuilt probe run: " + firstLine(got)
	}
	if golden == got {
		return fmt.Sprintf("IDENTICAL (%d chars, %d assertions)", len(got), strings.Count(got, ";"))
	}
	return "DIVERGED: golden!=rebuilt"
}

// --- v2 report writers -----------------------------------------------------

func pk2WriteJSON(t *testing.T, outDir string, r pk2Report) {
	b, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		t.Logf("json: %v", err)
		return
	}
	p := filepath.Join(outDir, "report-v2.json")
	if err := os.WriteFile(p, b, 0o644); err != nil {
		t.Logf("write %s: %v", p, err)
		return
	}
	t.Logf("wrote %s", p)
}

func pk2Pct(n, d int) string {
	if d == 0 {
		return "0/0"
	}
	return fmt.Sprintf("%d/%d (%.0f%%)", n, d, 100*float64(n)/float64(d))
}

func pk2FindMode(modes []pk2Mode, tool string) (pk2Mode, bool) {
	for _, m := range modes {
		if m.Tool == tool {
			return m, true
		}
	}
	return pk2Mode{}, false
}

func pk2WriteMarkdown(t *testing.T, outDir string, r pk2Report) {
	var sb strings.Builder
	sb.WriteString("# Yak Java Decompiler — Cross-Comparison Report v2 (machine-generated)\n\n")
	sb.WriteString(fmt.Sprintf("- Generated: %s\n- Host: %d CPUs, Go %s\n- Java: %s\n- CFR: %s\n- Vineflower (Fernflower lineage): %s\n- Yak workers (concurrent): %d\n\n",
		r.GeneratedAt, r.NumCPU, r.GoVersion, r.Java, r.CFRVersion, r.VFVersion, r.Workers))
	sb.WriteString("> Tools: **yak-syntax** = Yak with the ANTLR syntax safety net (default); ")
	sb.WriteString("**yak-raw** = Yak with the safety net OFF (fastest, may leak unparseable Java); ")
	sb.WriteString("**cfr** 0.152; **vineflower** = the maintained Fernflower lineage (Fernflower→Quiltflower→Vineflower).\n\n")

	// Performance.
	sb.WriteString("## 1. Performance — decompile wall-clock (lower is better)\n\n")
	sb.WriteString("| Jar | classes | yak-raw conc | yak-syntax conc | yak-raw serial | yak-syntax serial | cfr | vineflower |\n")
	sb.WriteString("|-----|---------|--------------|-----------------|----------------|-------------------|-----|------------|\n")
	for _, j := range r.Jars {
		ys, _ := pk2FindMode(j.Modes, "yak-syntax")
		yr, _ := pk2FindMode(j.Modes, "yak-raw")
		cfr, _ := pk2FindMode(j.Modes, "cfr")
		vf, _ := pk2FindMode(j.Modes, "vineflower")
		sb.WriteString(fmt.Sprintf("| %s | %d | %.2fs | %.2fs | %.2fs | %.2fs | %.2fs | %.2fs |\n",
			j.Label, j.ClassCount, yr.ConcSec, ys.ConcSec, yr.SerialSec, ys.SerialSec, cfr.ConcSec, vf.ConcSec))
	}

	// Throughput (classes/sec, concurrent) + speedups.
	sb.WriteString("\n## 2. Throughput — classes/sec (concurrent) and speedup vs CFR\n\n")
	sb.WriteString("| Jar | yak-raw c/s | yak-syntax c/s | cfr c/s | vineflower c/s | yak-syntax vs cfr |\n")
	sb.WriteString("|-----|-------------|----------------|---------|----------------|-------------------|\n")
	for _, j := range r.Jars {
		ys, _ := pk2FindMode(j.Modes, "yak-syntax")
		yr, _ := pk2FindMode(j.Modes, "yak-raw")
		cfr, _ := pk2FindMode(j.Modes, "cfr")
		vf, _ := pk2FindMode(j.Modes, "vineflower")
		speed := ""
		if cfr.ClassPerSec > 0 {
			speed = fmt.Sprintf("%.1fx", ys.ClassPerSec/cfr.ClassPerSec)
		}
		sb.WriteString(fmt.Sprintf("| %s | %.0f | %.0f | %.0f | %.0f | %s |\n",
			j.Label, yr.ClassPerSec, ys.ClassPerSec, cfr.ClassPerSec, vf.ClassPerSec, speed))
	}

	// Completeness.
	sb.WriteString("\n## 3. Completeness — Yak ok / stub / err per mode\n\n")
	sb.WriteString("| Jar | classes | yak-syntax ok | stub | err | yak-raw ok | stub | err |\n")
	sb.WriteString("|-----|---------|---------------|------|-----|------------|------|-----|\n")
	for _, j := range r.Jars {
		ys, _ := pk2FindMode(j.Modes, "yak-syntax")
		yr, _ := pk2FindMode(j.Modes, "yak-raw")
		sb.WriteString(fmt.Sprintf("| %s | %d | %d | %d | %d | %d | %d | %d |\n",
			j.Label, j.ClassCount, ys.OK, ys.Stub, ys.Err, yr.OK, yr.Stub, yr.Err))
	}

	// Recompilability.
	sb.WriteString("\n## 4. Recompilability — `javac` of decompiled source (jar+deps on classpath)\n\n")
	sb.WriteString("> Unit = each tool's native source layout (Yak: one flat top-level unit per class incl. nested `Outer$Inner`; CFR/Vineflower: one file per outer class with nested classes inlined). Each unit is compiled with the WHOLE decompiled tree on -sourcepath (intra-jar refs resolve against sibling sources, the standard whole-program round-trip) and jar+deps as classpath, one unit per javac (-implicit:none) so a single bad unit never zeroes the batch. recompile-OK == overlaid by construction (ground-truth: the unit's .class was actually emitted). Failures split into decompiler_err vs missing_dep.\n\n")
	sb.WriteString("| Jar | yak-syntax | yak-raw | cfr | vineflower |\n")
	sb.WriteString("|-----|------------|---------|-----|------------|\n")
	for _, j := range r.Jars {
		cell := func(tool string) string {
			m, ok := pk2FindMode(j.Modes, tool)
			if !ok {
				return "-"
			}
			return pk2Pct(m.RecompOK, m.RecompUnits)
		}
		sb.WriteString(fmt.Sprintf("| %s | %s | %s | %s | %s |\n", j.Label, cell("yak-syntax"), cell("yak-raw"), cell("cfr"), cell("vineflower")))
	}

	// Repackage + callable.
	sb.WriteString("\n## 5. Repackage → jar → externally callable (load+verify every class)\n\n")
	sb.WriteString("> Each tool's recompiled .class files are overlaid back onto the original jar; an external JVM ")
	sb.WriteString("loads+links (verifies, no <clinit>) every class. `linked` = JVM-verifiable; `verify_fail` = bytecode ")
	sb.WriteString("the verifier rejects (strongest correctness signal); `other` = missing-dep/linkage (often benign).\n\n")
	sb.WriteString("| Jar | tool | jar classes | rebuilt (overlaid) | linked | verify_fail | other |\n")
	sb.WriteString("|-----|------|-------------|--------------------|--------|-------------|-------|\n")
	for _, j := range r.Jars {
		for _, tool := range []string{"yak-syntax", "yak-raw", "cfr", "vineflower"} {
			m, ok := pk2FindMode(j.Modes, tool)
			if !ok {
				continue
			}
			sb.WriteString(fmt.Sprintf("| %s | %s | %d | %s | %s | %d | %d |\n",
				j.Label, tool, m.JarClasses, pk2Pct(m.Overlaid, m.JarClasses), pk2Pct(m.Linked, m.JarClasses), m.VerifyFail, m.OtherFail))
		}
	}

	// Differential call.
	sb.WriteString("\n## 6. Semantic differential call (guava)\n\n")
	for _, j := range r.Jars {
		if j.DiffCall != "" {
			sb.WriteString(fmt.Sprintf("- **%s**: Yak-rebuilt jar called by an external probe vs original → %s\n", j.Label, j.DiffCall))
		}
	}

	p := filepath.Join(outDir, "report-v2.md")
	if err := os.WriteFile(p, []byte(sb.String()), 0o644); err != nil {
		t.Logf("write %s: %v", p, err)
		return
	}
	t.Logf("wrote %s\n%s", p, sb.String())
}
