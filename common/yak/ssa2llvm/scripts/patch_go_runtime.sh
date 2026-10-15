#!/usr/bin/env bash
set -euo pipefail

# ssa2llvm splits the Go text into many ELF sections. The runtime's linear
# textsectmap scan makes traceback-heavy AOT binaries far too slow, so patch the
# lookup in the active GOROOT before building the c-archive.
python3 - <<'PY'
from pathlib import Path
import subprocess

GOROOT = Path(subprocess.check_output(["go", "env", "GOROOT"], text=True).strip())
path = GOROOT / "src/runtime/symtab.go"
marker = "// ssa2llvm: textsect binary search"
text = path.read_text()

if marker in text:
    print(f"[yaklib] Go runtime already patched: {path}")
    raise SystemExit(0)

old_addr = '''\t\tfor i, sect := range md.textsectmap {
\t\t\t// For the last section, include the end address (etext), as it is included in the functab.
\t\t\tif off >= sect.vaddr && off < sect.end || (i == len(md.textsectmap)-1 && off == sect.end) {
\t\t\t\tres = sect.baseaddr + off - sect.vaddr
\t\t\t\tbreak
\t\t\t}
\t\t}'''
new_addr = '''\t\t// ssa2llvm: textsect binary search
\t\tlo, hi := 0, len(md.textsectmap)-1
\t\tfor lo <= hi {
\t\t\tmid := lo + (hi-lo)/2
\t\t\tsect := md.textsectmap[mid]
\t\t\tif off < sect.vaddr {
\t\t\t\thi = mid - 1
\t\t\t\tcontinue
\t\t\t}
\t\t\t// For the last section, include the end address (etext), as it is included in the functab.
\t\t\tif off >= sect.end && !(mid == len(md.textsectmap)-1 && off == sect.end) {
\t\t\t\tlo = mid + 1
\t\t\t\tcontinue
\t\t\t}
\t\t\tres = sect.baseaddr + off - sect.vaddr
\t\t\tbreak
\t\t}'''

old_off = '''\t\tfor i, sect := range md.textsectmap {
\t\t\tif sect.baseaddr > pc {
\t\t\t\t// pc is not in any section.
\t\t\t\treturn 0, false
\t\t\t}
\t\t\tend := sect.baseaddr + (sect.end - sect.vaddr)
\t\t\t// For the last section, include the end address (etext), as it is included in the functab.
\t\t\tif i == len(md.textsectmap)-1 {
\t\t\t\tend++
\t\t\t}
\t\t\tif pc < end {
\t\t\t\tres = uint32(pc - sect.baseaddr + sect.vaddr)
\t\t\t\tbreak
\t\t\t}
\t\t}'''
new_off = '''\t\tlo, hi := 0, len(md.textsectmap)
\t\tfor lo < hi {
\t\t\tmid := lo + (hi-lo)/2
\t\t\tif md.textsectmap[mid].baseaddr <= pc {
\t\t\t\tlo = mid + 1
\t\t\t} else {
\t\t\t\thi = mid
\t\t\t}
\t\t}
\t\ti := lo - 1
\t\tif i < 0 {
\t\t\t// pc is not in any section.
\t\t\treturn 0, false
\t\t}
\t\tsect := md.textsectmap[i]
\t\tend := sect.baseaddr + (sect.end - sect.vaddr)
\t\t// For the last section, include the end address (etext), as it is included in the functab.
\t\tif i == len(md.textsectmap)-1 {
\t\t\tend++
\t\t}
\t\tif pc < end {
\t\t\tres = uint32(pc - sect.baseaddr + sect.vaddr)
\t\t} else if i < len(md.textsectmap)-1 {
\t\t\t// pc is in a gap between two text sections.
\t\t\treturn 0, false
\t\t}'''

if text.count(old_addr) != 1 or text.count(old_off) != 1:
    raise SystemExit(f"[yaklib] unsupported Go runtime source: expected one textAddr and textOff loop in {path}")

path.write_text(text.replace(old_addr, new_addr).replace(old_off, new_off))
print(f"[yaklib] patched Go runtime textsect lookups: {path}")
PY
