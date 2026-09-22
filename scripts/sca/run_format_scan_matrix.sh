#!/bin/sh
# Compare frozen-fixture scans: current ScanReport vs archived baseline-eval
# ScanLocalFilesystem. Machine output belongs under the supervision directory.
# This is not a production SCA dependency.
#
# Usage:
#   OLD=<baseline-eval> OUT=<supervision-dir> [COUNT=3] scripts/sca/run_format_scan_matrix.sh
#   scripts/sca/run_format_scan_matrix.sh <OLD> <OUT>
#   MATRIX_SELFTEST=1 scripts/sca/run_format_scan_matrix.sh
#
# GO is taken from PATH (command -v go). OLD and OUT must be set; there are
# no personal-path defaults. Exclusive temps created this run are the only
# paths removed on exit. Pre-existing OLD/OUT files are not deleted. A failed
# old/new build does not execute a stale OUT binary.
set -eu

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
GO="$(command -v go || true)"
COUNT="${COUNT:-3}"
export GOWORK=off
export GOFLAGS="${GOFLAGS:-}"

# Newline-separated exclusive temp paths. Spaces in TMPDIR must not split.
TEMP_RECORD=""
add_temp() {
	if [ -z "$1" ]; then
		return 0
	fi
	if [ -z "$TEMP_RECORD" ]; then
		TEMP_RECORD="$(mktemp "${TMPDIR:-/tmp}/sca-temps.XXXXXX")"
	fi
	printf '%s\n' "$1" >>"$TEMP_RECORD"
}
cleanup_temps() {
	if [ -n "$TEMP_RECORD" ] && [ -f "$TEMP_RECORD" ]; then
		while IFS= read -r d || [ -n "$d" ]; do
			if [ -n "$d" ] && [ -d "$d" ]; then
				rm -rf "$d"
			fi
		done <"$TEMP_RECORD"
		rm -f "$TEMP_RECORD"
		TEMP_RECORD=""
	fi
}
trap cleanup_temps EXIT

die() {
	echo "$1" >&2
	exit "${2:-2}"
}

run_logged() {
	log="$1"
	shift
	if "$@" >"$log" 2>&1; then
		cat "$log"
		return 0
	fi
	cat "$log"
	return 1
}

# Old driver source is written only into an exclusive temp module, never OLD.
old_driver_src() {
	cat << 'GO'
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

func rssBytes() int64 {
	var ru syscall.Rusage
	if err := syscall.Getrusage(syscall.RUSAGE_SELF, &ru); err != nil {
		return -1
	}
	n := int64(ru.Maxrss)
	if n < 0 {
		return -1
	}
	switch runtime.GOOS {
	case "darwin", "ios":
		return n
	default:
		return n * 1024
	}
}

func cpuNs() int64 {
	var ru syscall.Rusage
	if err := syscall.Getrusage(syscall.RUSAGE_SELF, &ru); err != nil {
		return -1
	}
	return ru.Utime.Nano() + ru.Stime.Nano()
}

func inputHash(dir string) (sum string, files int, bytes int64, err error) {
	var paths []string
	err = filepath.Walk(dir, func(p string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info.IsDir() {
			return nil
		}
		paths = append(paths, p)
		return nil
	})
	if err != nil {
		return "", 0, 0, err
	}
	sort.Strings(paths)
	h := sha256.New()
	for _, p := range paths {
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return "", 0, 0, err
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return "", 0, 0, err
		}
		fmt.Fprintf(h, "%s %d\n", filepath.ToSlash(rel), len(b))
		h.Write(b)
		files++
		bytes += int64(len(b))
	}
	return hex.EncodeToString(h.Sum(nil)), files, bytes, nil
}

func projection(keys []string) (uniqueHash, multiHash string, unique []string) {
	counts := map[string]int{}
	for _, k := range keys {
		counts[k]++
	}
	unique = make([]string, 0, len(counts))
	for k := range counts {
		unique = append(unique, k)
	}
	sort.Strings(unique)
	uh := sha256.Sum256([]byte(strings.Join(unique, "\n")))
	multi := make([]string, 0, len(unique))
	for _, k := range unique {
		multi = append(multi, fmt.Sprintf("%s\t%d", k, counts[k]))
	}
	mh := sha256.Sum256([]byte(strings.Join(multi, "\n")))
	return hex.EncodeToString(uh[:]), hex.EncodeToString(mh[:]), unique
}

func fail(payload map[string]any, err error, code int) {
	payload["ok"] = false
	payload["err"] = err.Error()
	_ = json.NewEncoder(os.Stdout).Encode(payload)
	os.Exit(code)
}

func main() {
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "usage: old-scan <case> <dir>")
		os.Exit(2)
	}
	name, dir := os.Args[1], os.Args[2]
	out := map[string]any{
		"side": "old", "case": name, "dir": dir,
		"rss_source": "getrusage(RUSAGE_SELF).ru_maxrss; darwin bytes, linux KiB converted to bytes; process peak including Go runtime",
		"duration_source": "wall time of ScanFilesystem(NewRelLocalFs) only; OS analyzers Match relative snapshot paths such as var/lib/dpkg/status",
		"cpu_source": "getrusage(RUSAGE_SELF) utime+stime delta around ScanFilesystem; includes Go runtime; not isolated parser CPU",
		"temp_files": 0,
		"temp_files_source": "old ScanFilesystem via RelLocalFs; this driver creates no extra temp files. Not a host syscall trace",
		"gomaxprocs": runtime.GOMAXPROCS(0),
		"num_cpu": runtime.NumCPU(),
		"file_opens": -1,
		"read_bytes": -1,
		"opens_source": "not instrumented on the archived ScanFilesystem adapter; -1 means unmeasured, not zero work",
	}
	sum, nfiles, nbytes, err := inputHash(dir)
	if err != nil {
		fail(out, fmt.Errorf("input walk/read: %w", err), 1)
	}
	out["input_sha256"] = sum
	out["input_files"] = nfiles
	out["input_bytes"] = nbytes
	cpu0 := cpuNs()
	start := time.Now()
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	alloc0, mallocs0 := ms.TotalAlloc, ms.Mallocs
	pkgs, err := sca.ScanFilesystem(filesys.NewRelLocalFs(dir))
	elapsed := time.Since(start)
	cpu1 := cpuNs()
	runtime.ReadMemStats(&ms)
	out["ns"] = elapsed.Nanoseconds()
	if cpu0 >= 0 && cpu1 >= cpu0 {
		out["cpu_ns"] = cpu1 - cpu0
	} else {
		out["cpu_ns"] = int64(-1)
	}
	out["alloc_bytes"] = ms.TotalAlloc - alloc0
	out["mallocs"] = ms.Mallocs - mallocs0
	out["rss_bytes"] = rssBytes()
	keys := []string{}
	for _, p := range pkgs {
		if p == nil || strings.TrimSpace(p.Name) == "" {
			continue
		}
		keys = append(keys, p.Name+"\t"+p.Version)
	}
	if os.Getenv("SCA_MATRIX_FULL_FIELDS") == "1" {
		fields := make([]map[string]any, 0, len(pkgs))
		for _, p := range pkgs {
			if p == nil { continue }
			up, down := []string{}, []string{}
			for _, q := range p.UpStreamPackages { if q != nil { up=append(up,q.Name+"@"+q.Version) } }
			for _, q := range p.DownStreamPackages { if q != nil { down=append(down,q.Name+"@"+q.Version) } }
			sort.Strings(up);sort.Strings(down)
			fields=append(fields,map[string]any{"Name":p.Name,"Version":p.Version,"Verification":p.Verification,"License":p.License,"Potential":p.Potential,"IsVersionRange":p.IsVersionRange,"FromFile":p.FromFile,"FromAnalyzer":p.FromAnalyzer,"DependsAnd":p.DependsOn.And,"DependsOr":p.DependsOn.Or,"UpNames":up,"DownNames":down})
		}
		out["fields"] = fields
	}
	uh, mh, unique := projection(keys)
	out["packages"] = len(pkgs)
	out["projection"] = len(unique)
	out["projection_sha256"] = uh
	out["multiplicity_sha256"] = mh
	out["identities"] = unique
	out["ok"] = err == nil
	if err != nil {
		out["err"] = err.Error()
	} else {
		out["err"] = ""
	}
	_ = json.NewEncoder(os.Stdout).Encode(out)
	if err != nil {
		os.Exit(1)
	}
}
GO
}

# Build old-scan in an exclusive module (copy go.mod/go.sum, symlink common).
# Writes the binary to $1 only after a successful compile. Never writes into OLD.
# Never overwrites dest on failure.
install_old_scan() {
	dest="$1"
	old_root="$2"
	log="$3"
	mod="$(mktemp -d "${TMPDIR:-/tmp}/sca-old-mod.XXXXXX")"
	scratch="$(mktemp -d "${TMPDIR:-/tmp}/sca-old-bin.XXXXXX")"
	add_temp "$mod"
	add_temp "$scratch"
	if [ ! -f "$old_root/go.mod" ] || [ ! -d "$old_root/common" ]; then
		echo "OLD is not a baseline-eval module" >"$log"
		return 1
	fi
	cp "$old_root/go.mod" "$mod/go.mod"
	if [ -f "$old_root/go.sum" ]; then
		cp "$old_root/go.sum" "$mod/go.sum"
	fi
	ln -s "$old_root/common" "$mod/common"
	mkdir -p "$mod/cmd/oldscan"
	old_driver_src >"$mod/cmd/oldscan/main.go"
	built="$scratch/old-scan"
	if (
		cd "$mod"
		CGO_ENABLED=1 GOPROXY=off GOSUMDB=off "$GO" build -o "$built" ./cmd/oldscan
	) >"$log" 2>&1; then
		cp "$built" "$dest"
		chmod +x "$dest"
		return 0
	fi
	return 1
}

install_new_scan() {
	dest="$1"
	log="$2"
	scratch="$(mktemp -d "${TMPDIR:-/tmp}/sca-new-bin.XXXXXX")"
	add_temp "$scratch"
	built="$scratch/new-scan"
	if (
		cd "$ROOT"
		CGO_ENABLED=0 GOPROXY=off GOSUMDB=off "$GO" build -o "$built" ./scripts/sca/format_scan_matrix
	) >"$log" 2>&1; then
		cp "$built" "$dest"
		chmod +x "$dest"
		return 0
	fi
	return 1
}

compare_py() {
	cat << 'PY'
import json, os, sys, hashlib, shutil, tempfile, subprocess, time

def parse_run(run):
    if run.get("exit") != 0 or not run.get("stdout"):
        return None
    try:
        return json.loads(run["stdout"].splitlines()[-1])
    except Exception:
        return None

def side_status(runs, fixture_hash):
    if not runs:
        return False, "no runs", []
    parsed = []
    for i, run in enumerate(runs):
        rec = parse_run(run)
        if rec is None or rec.get("ok") is not True:
            return False, "repeat %d failed" % i, parsed
        parsed.append(rec)
    inputs = {p.get("input_sha256") for p in parsed}
    if inputs != {fixture_hash}:
        return False, "canonical input hash differs from fixture or across repeats", parsed
    proj = {p.get("projection_sha256") for p in parsed}
    multi = {p.get("multiplicity_sha256") for p in parsed}
    if len(proj) != 1 or len(multi) != 1:
        return False, "nondeterministic projection across repeats", parsed
    return True, "", parsed

def decide(rec):
    fixture = rec.get("input_sha256")
    old_ok, old_reason, olds = side_status(rec.get("old_runs") or [], fixture)
    new_ok, new_reason, news = side_status(rec.get("new_runs") or [], fixture)
    rec["old_ok"] = old_ok
    rec["new_ok"] = new_ok
    rec["old"] = olds[-1] if olds else None
    rec["new"] = news[-1] if news else None
    rec["same_input"] = bool(olds and news and all(p.get("input_sha256") == fixture for p in olds + news))
    rec["deterministic"] = old_ok and new_ok
    rec["projection_equal"] = False
    rec["comparable"] = False
    rec["new_failed"] = False
    if not new_ok:
        rec["reason"] = new_reason or "new scan failed"
        rec["new_failed"] = True
        return rec
    if news and news[0].get("complete") is False:
        rec["reason"] = "new scan incomplete"
        rec["new_failed"] = True
        rec["new_ok"] = False
        return rec
    if not old_ok:
        rec["reason"] = old_reason or "old scan failed"
        return rec
    if not rec["same_input"]:
        rec["reason"] = "canonical input hash differs"
        return rec
    old_p, new_p = olds[0].get("projection_sha256"), news[0].get("projection_sha256")
    old_m, new_m = olds[0].get("multiplicity_sha256"), news[0].get("multiplicity_sha256")
    rec["old_identities"] = olds[0].get("identities")
    rec["new_identities"] = news[0].get("identities")
    if old_p != new_p or old_m != new_m:
        rec["reason"] = "name@version projection differs; not a fabricated match"
        return rec
    rec["projection_equal"] = True
    rec["comparable"] = True
    rec["reason"] = ""
    return rec

def _run(identities, ok=True, complete=True, inp="abc", proj=None, multi=None, exit=0):
    if proj is None:
        proj = "p:" + ",".join(identities)
    if multi is None:
        multi = "m:" + ",".join(identities)
    payload = {
        "ok": ok, "complete": complete, "input_sha256": inp,
        "projection_sha256": proj, "multiplicity_sha256": multi,
        "identities": identities,
    }
    return {"exit": exit, "stdout": json.dumps(payload), "stderr": ""}

def selftest_compare():
    fixture = "abc"
    good = decide({
        "input_sha256": fixture,
        "old_runs": [_run(["a\t1"]), _run(["a\t1"])],
        "new_runs": [_run(["a\t1"]), _run(["a\t1"])],
    })
    if not good["comparable"] or not good["same_input"] or not good["projection_equal"]:
        raise SystemExit("expected comparable equal projections")
    unequal = decide({
        "input_sha256": fixture,
        "old_runs": [_run(["a\t1"]), _run(["a\t1"])],
        "new_runs": [_run(["b\t1"]), _run(["b\t1"])],
    })
    if unequal["comparable"] or unequal["projection_equal"]:
        raise SystemExit("unequal projections marked comparable")
    failed = decide({
        "input_sha256": fixture,
        "old_runs": [_run(["a\t1"]), _run(["a\t1"], ok=False, exit=1)],
        "new_runs": [_run(["a\t1"]), _run(["a\t1"])],
    })
    if failed["comparable"] or failed["old_ok"]:
        raise SystemExit("failed repeat marked comparable")
    newfail = decide({
        "input_sha256": fixture,
        "old_runs": [_run(["a\t1"]), _run(["a\t1"])],
        "new_runs": [_run(["a\t1"], ok=False, complete=False, exit=1), _run(["a\t1"])],
    })
    if newfail["comparable"] or not newfail["new_failed"]:
        raise SystemExit("new failure not retained")
    mismatch = decide({
        "input_sha256": fixture,
        "old_runs": [_run(["a\t1"], inp="zzz"), _run(["a\t1"], inp="zzz")],
        "new_runs": [_run(["a\t1"]), _run(["a\t1"])],
    })
    if mismatch["comparable"] or mismatch["same_input"]:
        raise SystemExit("input hash mismatch marked comparable")
    nondet = decide({
        "input_sha256": fixture,
        "old_runs": [_run(["a\t1"]), _run(["a\t2"])],
        "new_runs": [_run(["a\t1"]), _run(["a\t1"])],
    })
    if nondet["comparable"] or nondet["deterministic"]:
        raise SystemExit("nondeterministic projection marked comparable")
    print("compare selftest ok")

def fixture_hash(files):
    h = hashlib.sha256()
    for dest, data in sorted(files):
        h.update(("%s %d\n" % (dest.replace("\\", "/"), len(data))).encode())
        h.update(data)
    return h.hexdigest()

def run_matrix(root, out, count):
    sys.path.insert(0, os.path.join(root, "scripts/sca"))
    from fixture_store import FixtureStore
    corpus = FixtureStore(os.path.join(root, "common/sca"))
    def logical(src):
        return os.path.relpath(src, os.path.join(root, "common/sca")).replace("\\", "/")
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
    new_failed = False
    for name in order:
        files = cases[name]
        missing = [src for _, src in files if logical(src) not in corpus.rows]
        if missing:
            rows.append({"case": name, "comparable": False, "new_failed": True,
                         "reason": "missing fixture " + ",".join(missing)})
            new_failed = True
            continue
        tdir = tempfile.mkdtemp(prefix="sca-fx-"+name+"-")
        rec = {"case": name, "files": [d for d, _ in files], "old_runs": [], "new_runs": []}
        try:
            copied = []
            for dest, src in files:
                path = os.path.join(tdir, dest)
                os.makedirs(os.path.dirname(path), exist_ok=True)
                data = corpus.read(logical(src))
                open(path, "wb").write(data)
                copied.append((dest, data))
            rec["input_sha256"] = fixture_hash(copied)
            sides = (("old", old_bin, "old_runs"), ("new", new_bin, "new_runs"))
            # Alternate paired order to reduce thermal/cache/time drift bias.
            for repeat in range(count):
                for side, binpath, key in (sides if repeat % 2 == 0 else sides[::-1]):
                    if not os.path.isfile(binpath):
                        rec[side+"_missing_binary"] = True
                        continue
                    t0 = time.perf_counter()
                    p = subprocess.run([binpath, name, tdir], capture_output=True, text=True)
                    rec[key].append({
                        "exit": p.returncode,
                        "stdout": p.stdout.strip(),
                        "stderr": p.stderr.strip()[:2000],
                        "wall_s": time.perf_counter() - t0,
                    })
            decide(rec)
            if rec.get("new_failed"):
                new_failed = True
        finally:
            shutil.rmtree(tdir, ignore_errors=True)
        rows.append(rec)
    summary = {
        "cases": len(rows),
        "comparable": sum(1 for r in rows if r.get("comparable")),
        "incomparable": [{"case": r["case"], "reason": r.get("reason")} for r in rows if not r.get("comparable")],
        "new_failed": new_failed,
        "measurement": {
            "duration": "wall time of ScanReport (new) / ScanFilesystem(RelLocalFs) (old) inside the scan subprocess",
            "rss_bytes": "process peak RSS via getrusage ru_maxrss; Darwin bytes, Linux KiB converted to bytes; includes Go runtime",
            "comparable_requires": [
                "every repeat exit 0 and ok",
                "new complete semantic contract",
                "same canonical input hash as the fixture copy and across repeats",
                "same unique name@version projection and multiplicity across repeats and between old and new",
            ],
            "projection_equal_is_not": "full-field semantic equivalence or same-semantics 1.10 performance acceptance",
        },
        "claim": "not a completed old/new semantic matrix; incomparable rows are not fabricated matches",
    }
    open(os.path.join(out, "matrix.json"), "w").write(json.dumps({"summary": summary, "rows": rows}, indent=2) + "\n")
    print(json.dumps(summary, indent=2))
    if new_failed:
        sys.exit(1)

if __name__ == "__main__":
    if len(sys.argv) > 1 and sys.argv[1] == "--selftest":
        selftest_compare()
    else:
        run_matrix(sys.argv[1], sys.argv[2], int(sys.argv[3]))
PY
}

selftest() {
	echo "matrix driver selftest"
	compare_py | python3 - --selftest

	out="$(mktemp -d "${TMPDIR:-/tmp}/sca-matrix-out.XXXXXX")"
	old="$(mktemp -d "${TMPDIR:-/tmp}/sca-matrix-old.XXXXXX")"
	add_temp "$out"
	add_temp "$old"
	echo sentinel-out >"$out/SENTINEL"
	echo sentinel-old >"$old/SENTINEL"
	mkdir -p "$old/cmd/oldscan"
	echo keep >"$old/cmd/oldscan/KEEP"
	# Stale binary would succeed if executed; it must not run on build failure.
	printf '#!/bin/sh\necho ran > "%s/RAN"\necho {"ok":true}\n' "$out" >"$out/old-scan"
	chmod +x "$out/old-scan"
	if [ -z "$GO" ]; then
		die "go not on PATH"
	fi
	if install_old_scan "$out/old-scan.next" "$old" "$out/old-build.log"; then
		die "old build should fail without go.mod"
	fi
	if [ ! -f "$out/SENTINEL" ] || [ ! -f "$old/SENTINEL" ] || [ ! -f "$old/cmd/oldscan/KEEP" ]; then
		die "sentinel or pre-existing OLD file was removed"
	fi
	if [ -f "$out/RAN" ]; then
		die "stale old-scan executed after build failure"
	fi
	if [ -f "$out/old-scan.next" ]; then
		die "failed build installed a binary"
	fi
	# Pre-existing stale dest is left in place and must still not be used by install.
	if [ ! -x "$out/old-scan" ]; then
		die "pre-existing OUT/old-scan was deleted"
	fi
	echo "sentinel/stale-binary selftest ok"

	base="${TMPDIR:-/tmp}"
	root="$(mktemp -d "$base/sca matrix trap.XXXXXX")"
	echo sentinel >"$root/SENTINEL"
	set +e
	TMPDIR="$root" MATRIX_TRAP_CHILD=1 "$0"
	st=$?
	set -e
	if [ "$st" -ne 0 ]; then
		rm -rf "$root"
		die "trap child failed"
	fi
	if [ ! -f "$root/SENTINEL" ]; then
		rm -rf "$root"
		die "space-root sentinel removed by trap"
	fi
	child=""
	if [ -f "$root/trap-child-temp" ]; then
		child="$(cat "$root/trap-child-temp")"
	fi
	if [ -z "$child" ] || [ -d "$child" ]; then
		rm -rf "$root"
		die "exclusive temp with spaces survived trap: $child"
	fi
	rm -rf "$root"
	echo "trap/space-tmpdir selftest ok"
}

write_cases() {
	out="$1"
	cat > "$out/cases.txt" << 'EOF'
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
}

if [ "${MATRIX_TRAP_CHILD:-}" = "1" ]; then
	t="$(mktemp -d "${TMPDIR:-/tmp}/sca-trap-child.XXXXXX")"
	add_temp "$t"
	echo inside >"$t/marker"
	printf '%s\n' "$t" >"${TMPDIR}/trap-child-temp"
	exit 0
fi

if [ "${MATRIX_SELFTEST:-}" = "1" ]; then
	if [ -z "$GO" ]; then
		die "go not on PATH"
	fi
	selftest
	exit 0
fi

OLD="${1:-${OLD:-}}"
OUT="${2:-${OUT:-}}"
if [ -z "$GO" ]; then
	die "go not on PATH; put Go 1.22.12 on PATH"
fi
if [ -z "$OLD" ] || [ -z "$OUT" ]; then
	die "OLD and OUT must be set as arguments or environment variables"
fi
if [ ! -d "$OLD" ]; then
	die "OLD is not a directory: $OLD"
fi
mkdir -p "$OUT"
cd "$ROOT"

write_cases "$OUT"
{
	"$GO" version
	uname -a
	printf 'OLD=%s\nROOT=%s\nCOUNT=%s\nGO=%s\n' "$OLD" "$ROOT" "$COUNT" "$GO"
	printf 'HEAD=%s\n' "$(git -C "$ROOT" rev-parse HEAD 2>/dev/null || echo unknown)"
	printf 'GOMAXPROCS=%s\nNumCPU=%s\n' "$("$GO" env GOMAXPROCS)" "$(getconf _NPROCESSORS_ONLN 2>/dev/null || echo unknown)"
	if command -v sysctl >/dev/null 2>&1; then
		printf 'cpu_brand=%s\n' "$(sysctl -n machdep.cpu.brand_string 2>/dev/null || echo unknown)"
		printf 'hw_ncpu=%s\n' "$(sysctl -n hw.ncpu 2>/dev/null || echo unknown)"
	fi
	printf 'workers_default=5\n'
	printf 'cache=GOPROXY=off GOSUMDB=off; no product network; each matrix repeat is a new process\n'
	printf 'budget=resource-policy.json defaults including MaxResultBytes=268435456 logical working-set\n'
	printf 'cgo_old=1 cgo_new=0\n'
} >"$OUT/toolchain.txt"
cat "$OUT/toolchain.txt"

if ! run_logged "$OUT/new-contract.log" env CGO_ENABLED=0 "$GO" test ./common/sca -count=1 -timeout 180s -run '^TestFormatScanContract$'; then
	die "TestFormatScanContract failed" 1
fi
if ! run_logged "$OUT/new-bench.log" env CGO_ENABLED=0 "$GO" test ./common/sca -run '^$' -bench '^BenchmarkFormatScan$' -benchtime=1x -benchmem -count="$COUNT"; then
	die "BenchmarkFormatScan failed" 1
fi

if ! install_old_scan "$OUT/old-scan" "$OLD" "$OUT/old-build.log"; then
	cat "$OUT/old-build.log" >&2
	echo "old baseline-eval build failed; refusing stale OUT/old-scan" | tee "$OUT/old-status.txt" >&2
	exit 1
fi
echo "old baseline-eval build ok" | tee "$OUT/old-status.txt"

if ! install_new_scan "$OUT/new-scan" "$OUT/new-build.log"; then
	cat "$OUT/new-build.log" >&2
	echo "new matrix driver build failed; refusing stale OUT/new-scan" >&2
	exit 1
fi

compare_py | python3 - "$ROOT" "$OUT" "$COUNT"
echo "matrix files in $OUT"
ls -la "$OUT"
