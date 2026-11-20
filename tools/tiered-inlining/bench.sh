#!/usr/bin/env bash
# Reproducible benchmark script comparing binary size (Full/Slim, raw/zip) and
# real system runtime performance across 4 Go inlining strategies:
#   default | tierA | tierB | all (-gcflags=all=-l)
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/../.." && pwd)"
OUT_DIR="${1:-$(mktemp -d "${TMPDIR:-/tmp}/yak-tiered-inline.XXXXXX")}"
mkdir -p "$OUT_DIR"
cd "$ROOT_DIR"

echo "Output dir: $OUT_DIR"

build_variant() {
  local mode="$1" profile="$2"
  local tags="gzip_embed"
  if [[ "$mode" == "slim" ]]; then
    tags="gzip_embed,irify_exclude"
  fi
  local flags=()
  while IFS= read -r line; do
    [[ -n "$line" ]] && flags+=("$line")
  done < <(go run ./tools/tiered-inlining --profile="$profile")

  local out="$OUT_DIR/yak_${mode}_${profile}"
  echo "Building $out ..."
  go build -trimpath "${flags[@]}" -tags "$tags" -ldflags "-s -w" -o "$out" common/yak/cmd/yak.go
  rm -f "$out.zip"
  zip -q -j "$out.zip" "$out"
}

for mode in full slim; do
  for profile in default tierA tierB all; do
    build_variant "$mode" "$profile"
  done
done

echo "=== Size Summary ==="
ls -lh "$OUT_DIR"/yak_*

cat > "$OUT_DIR/bench_yakvm.yak" <<'EOF'
t0 = time.Now()
acc = 0
for i = 0; i < 50000; i++ { acc += (i * 3 + 7) % 13 }
for i = 0; i < 5000; i++ {
    b = codec.EncodeBase64(sprintf("yak-bench-item-%d-%d", i, acc))
    _, _ = codec.DecodeBase64(b)
}
for i = 0; i < 3000; i++ {
    m = json.loads(`{"id":12345,"name":"yaklang","tags":["ssa","vm","http"],"ok":true}`)
    _ = json.dumps(m)
}
printf("YAKVM_MS=%d\n", time.Since(t0).Milliseconds())
EOF

cat > "$OUT_DIR/bench_ssa_sf.yak" <<'EOF'
phpCode = `<?php
class UserController {
    public function handle($req) {
        $id = $_GET['id'];
        $clean = htmlspecialchars($_POST['name']);
        mysql_query("SELECT * FROM users WHERE id = " . $id);
        echo $clean;
        eval($_REQUEST['cmd']);
    }
}
`
t0 = time.Now()
for i = 0; i < 15; i++ {
    prog, err = ssa.Parse(phpCode, ssa.withLanguage("php"))
    die(err)
}
ssaMs = time.Since(t0).Milliseconds()

prog, err = ssa.Parse(phpCode, ssa.withLanguage("php"))
die(err)
t1 = time.Now()
for i = 0; i < 30; i++ {
    res = prog.SyntaxFlow(`$_GET as $src; $_REQUEST as $src; $src --> * as $sink; eval($sink) as $rce;`)
    assert res != nil
}
printf("PHP15_SSA_MS=%d SF30_MS=%d\n", ssaMs, time.Since(t1).Milliseconds())
EOF

HOME_WARM="$OUT_DIR/home_warm"
rm -rf "$HOME_WARM"
YAKIT_HOME="$HOME_WARM" "$OUT_DIR/yak_full_default" -e 'println("warmed")' >/dev/null 2>&1

echo "=== Runtime Performance (3 runs each, warm YAKIT_HOME) ==="
for profile in default tierA tierB all; do
  echo "--- $profile ---"
  for _ in 1 2 3; do
    YAKIT_HOME="$HOME_WARM" "$OUT_DIR/yak_full_$profile" "$OUT_DIR/bench_yakvm.yak" 2>/dev/null | grep YAKVM_MS
    YAKIT_HOME="$HOME_WARM" "$OUT_DIR/yak_full_$profile" "$OUT_DIR/bench_ssa_sf.yak" 2>/dev/null | grep PHP15_SSA_MS
  done
done
