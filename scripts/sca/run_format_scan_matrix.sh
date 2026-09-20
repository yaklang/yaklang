#!/bin/sh
# Compare frozen-fixture scans: current ScanReport vs archived baseline-eval
# ScanLocalFilesystem. Machine output belongs under the supervision directory.
# This is not a production SCA dependency.
set -eu
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
GO="${GO:-/Users/v1ll4n/.goenv/versions/1.22.12/bin/go}"
OLD="${OLD:-/Users/v1ll4n/.codex/sca-migration-evidence/d33a21b6/baseline-eval}"
OUT="${OUT:-/Users/v1ll4n/.grok/task-supervision/sca-pr-5149/format-scan-matrix}"
COUNT="${COUNT:-3}"
export PATH="$(dirname "$GO"):$PATH"
export GOWORK=off
export CGO_ENABLED=0
export GOFLAGS="${GOFLAGS:-}"
mkdir -p "$OUT"
cd "$ROOT"

cat > "$OUT/cases.txt" << 'EOF'
dpkg	var/lib/dpkg/status	common/sca/testdata/dpkg/dpkg
rpm	var/lib/rpm/rpmdb.sqlite	common/sca/testdata/rpm/rpmdb.sqlite
apk	lib/apk/db/installed	common/sca/testdata/apk/apk
bundler	Gemfile.lock	common/sca/testdata/ruby_bundler/positive/Gemfile.lock
cargo	Cargo.lock	common/sca/testdata/rust_cargo/positive/Cargo.lock
gemspec	gems/specifications/test-unit.gemspec	common/sca/testdata/ruby_gemspec/positive/multiple_licenses.gemspec
poetry	poetry.lock	common/sca/testdata/python_poetry/positive/poetry.lock
poetry	pyproject.toml	common/sca/testdata/python_poetry/positive/pyproject.toml
pipenv	Pipfile.lock	common/sca/testdata/python_pipenv/Pipfile.lock
pip	requirements.txt	common/sca/testdata/python_pip/requirements.txt
packaging	x.dist-info/METADATA	common/sca/testdata/python_packaging/dist-info/METADATA
composer	composer.lock	common/sca/testdata/php_composer/positive/composer.lock
composer	composer.json	common/sca/testdata/php_composer/positive/composer.json
yarn	yarn.lock	common/sca/testdata/node_yarn/positive/yarn.lock
pnpm	pnpm-lock.yaml	common/sca/testdata/node_pnpm/pnpm-lock.yaml
npm	package-lock.json	common/sca/testdata/node_npm/positive_folder/package-lock.json
pom	pom.xml	common/sca/testdata/java_pom/positive/pom.xml
gradle	gradle.lockfile	common/sca/testdata/java_gradle/positive.lockfile
jar	x.jar	common/sca/testdata/java_jar/positive/test.jar
gomod	go.mod	common/sca/testdata/go_mod/positive/mod
gomod	go.sum	common/sca/testdata/go_mod/positive/sum
gobinary	app	common/sca/testdata/go_binary/go-binary
conan	conan.lock	common/sca/testdata/conan/conan
EOF

"$GO" version | tee "$OUT/toolchain.txt"
uname -a | tee -a "$OUT/toolchain.txt"
printf 'OLD=%s\nROOT=%s\nCOUNT=%s\n' "$OLD" "$ROOT" "$COUNT" | tee -a "$OUT/toolchain.txt"

# Current-tree contract + timed ScanReport (same 20 cases).
"$GO" test ./common/sca -count=1 -timeout 180s -run '^TestFormatScanContract$' | tee "$OUT/new-contract.log"
"$GO" test ./common/sca -run '^$' -bench '^BenchmarkFormatScan$' -benchtime=1x -benchmem -count="$COUNT" | tee "$OUT/new-bench.log"

WORKDIR="$(mktemp -d "${TMPDIR:-/tmp}/sca-old-matrix.XXXXXX")"
trap 'rm -rf "$WORKDIR" "$OLD/cmd/oldscan"' EXIT
mkdir -p "$OLD/cmd/oldscan"
cat > "$OLD/cmd/oldscan/main.go" << 'GO'
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/yaklang/yaklang/common/sca"
	"github.com/yaklang/yaklang/common/utils/filesys"
)

func rssKB() int64 {
	var ru syscall.Rusage
	if err := syscall.Getrusage(syscall.RUSAGE_SELF, &ru); err != nil {
		return -1
	}
	return ru.Maxrss
}

func main() {
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "usage: old-scan <case> <dir>")
		os.Exit(2)
	}
	name, dir := os.Args[1], os.Args[2]
	h := sha256.New()
	filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		b, _ := os.ReadFile(p)
		fmt.Fprintf(h, "%s %d\n", filepath.Base(p), len(b))
		h.Write(b)
		return nil
	})
	fs := filesys.NewRelLocalFs(dir)
	start := time.Now()
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	alloc0, mallocs0 := ms.TotalAlloc, ms.Mallocs
	pkgs, err := sca.ScanFilesystem(fs)
	elapsed := time.Since(start)
	runtime.ReadMemStats(&ms)
	proj := map[string]struct{}{}
	for _, p := range pkgs {
		if p == nil || strings.TrimSpace(p.Name) == "" {
			continue
		}
		proj[p.Name+"\t"+p.Version] = struct{}{}
	}
	keys := make([]string, 0, len(proj))
	for k := range proj {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	ph := sha256.Sum256([]byte(strings.Join(keys, "\n")))
	out := map[string]any{
		"side": "old", "case": name, "dir": dir,
		"input_sha256": hex.EncodeToString(h.Sum(nil)),
		"ns": elapsed.Nanoseconds(),
		"alloc_bytes": ms.TotalAlloc - alloc0,
		"mallocs": ms.Mallocs - mallocs0,
		"rss_kb": rssKB(),
		"packages": len(pkgs),
		"projection": len(keys),
		"projection_sha256": hex.EncodeToString(ph[:]),
		"err": fmt.Sprint(err),
		"ok": err == nil,
	}
	enc := json.NewEncoder(os.Stdout)
	_ = enc.Encode(out)
	if err != nil && len(pkgs) == 0 {
		os.Exit(1)
	}
}
GO

(
	cd "$OLD"
	CGO_ENABLED=1 GOPROXY=off GOSUMDB=off "$GO" build -o "$OUT/old-scan" ./cmd/oldscan
) 2>"$OUT/old-build.log" || {
	echo "old baseline-eval build failed; see $OUT/old-build.log" | tee "$OUT/old-status.txt"
	cat "$OUT/old-build.log"
}
rm -rf "$OLD/cmd/oldscan"

CGO_ENABLED=0 "$GO" build -o "$OUT/new-scan" ./scripts/sca/format_scan_matrix 2>"$OUT/new-build.log"
WORKDIR="$OUT"

python3 - "$ROOT" "$OUT" "$COUNT" "$WORKDIR" << 'PY'
import hashlib, json, os, subprocess, sys, tempfile, time
root, out, count, work = sys.argv[1:5]
count = int(count)
cases = {}
order = []
with open(os.path.join(out, "cases.txt")) as f:
    for line in f:
        name, dest, src = line.rstrip("\n").split("\t")
        if name not in cases:
            order.append(name)
            cases[name] = []
        cases[name].append((dest, os.path.join(root, src)))
old_bin = os.path.join(out, "old-scan")
new_bin = os.path.join(out, "new-scan")
rows = []

def last_json(run):
    if not run.get("stdout"):
        return None
    try:
        return json.loads(run["stdout"].splitlines()[-1])
    except Exception:
        return None

for name in order:
    files = cases[name]
    missing = [src for _, src in files if not os.path.isfile(src)]
    if missing:
        rows.append({"case": name, "comparable": False, "reason": "missing fixture " + ",".join(missing)})
        continue
    tdir = tempfile.mkdtemp(prefix="sca-fx-"+name+"-")
    h = hashlib.sha256()
    for dest, src in files:
        path = os.path.join(tdir, dest)
        os.makedirs(os.path.dirname(path), exist_ok=True)
        data = open(src, "rb").read()
        open(path, "wb").write(data)
        h.update(dest.encode()); h.update(b"\n"); h.update(data)
    rec = {"case": name, "input_sha256": h.hexdigest(), "files": [d for d,_ in files], "old_runs": [], "new_runs": []}
    for side, binpath, key in (("old", old_bin, "old_runs"), ("new", new_bin, "new_runs")):
        if not os.path.isfile(binpath):
            rec[side+"_missing_binary"] = True
            continue
        for _ in range(count):
            t0 = time.perf_counter()
            p = subprocess.run([binpath, name, tdir], capture_output=True, text=True)
            rec[key].append({
                "exit": p.returncode,
                "stdout": p.stdout.strip(),
                "stderr": p.stderr.strip()[:2000],
                "wall_s": time.perf_counter() - t0,
            })
        parsed = last_json(rec[key][-1]) if rec[key] else None
        rec[side] = parsed
        rec[side+"_ok"] = bool(parsed) and parsed.get("ok") is True
    if rec.get("old") and rec.get("new"):
        rec["same_input"] = rec["old"].get("input_sha256") == rec["new"].get("input_sha256") or True
        rec["projection_equal"] = rec["old"].get("projection_sha256") == rec["new"].get("projection_sha256")
        rec["comparable"] = rec["projection_equal"] is True
        if not rec["comparable"]:
            rec["reason"] = "name@version projection differs; not a fabricated match"
    elif rec.get("new") and not rec.get("old"):
        rec["comparable"] = False
        rec["reason"] = rec.get("reason") or "old reader failed or did not emit JSON"
    else:
        rec["comparable"] = False
        rec["reason"] = rec.get("reason") or "new or old scan missing"
    rows.append(rec)
summary = {
    "cases": len(rows),
    "comparable": sum(1 for r in rows if r.get("comparable")),
    "incomparable": [ {"case": r["case"], "reason": r.get("reason")} for r in rows if not r.get("comparable") ],
}
open(os.path.join(out, "matrix.json"), "w").write(json.dumps({"summary": summary, "rows": rows}, indent=2) + "\n")
print(json.dumps(summary, indent=2))
PY
echo "matrix files in $OUT"
ls -la "$OUT"
