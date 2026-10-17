#!/usr/bin/env bash
set -euo pipefail

# ssa2llvm splits the Go text into many ELF sections. Both directions need
# logarithmic lookup: textOff is ordered by physical PC; textAddr uses a second
# map ordered by virtual offset, installed by SortFinalTextMap before execution.
python3 - <<'PY'
from pathlib import Path
import subprocess

GOROOT = Path(subprocess.check_output(["go", "env", "GOROOT"], text=True).strip())
path = GOROOT / "src/runtime/symtab.go"
text = path.read_text()

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
new_off = '''\t\t// ssa2llvm: textsect binary search\n\t\tlo, hi := 0, len(md.textsectmap)
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

old_addr = '''\t\tfor i, sect := range md.textsectmap {
\t\t\t// For the last section, include the end address (etext), as it is included in the functab.
\t\t\tif off >= sect.vaddr && off < sect.end || (i == len(md.textsectmap)-1 && off == sect.end) {
\t\t\t\tres = sect.baseaddr + off - sect.vaddr
\t\t\t\tbreak
\t\t\t}
\t\t}'''
new_addr = '''\t\t// ssa2llvm: virtual textsect binary search
\t\t// The final linker orders textsectmap by physical PC. Its virtual
\t\t// offsets are not sorted, so searching that map by vaddr is invalid.
\t\t// The ELF post-link patch installs an independent virtual-order map.
\t\tif md == &firstmoduledata && len(ssa2llvmTextAddrMap) > 1 {
\t\t\tsects := ssa2llvmTextAddrMap
\t\t\tlo, hi := 0, len(sects)
\t\t\tfor lo < hi {
\t\t\t\tmid := lo + (hi-lo)/2
\t\t\t\tif sects[mid].vaddr <= off {
\t\t\t\t\tlo = mid + 1
\t\t\t\t} else {
\t\t\t\t\thi = mid
\t\t\t\t}
\t\t\t}
\t\t\ti := lo - 1
\t\t\tif i >= 0 {
\t\t\t\tsect := sects[i]
\t\t\t\tif off < sect.end || (i == len(sects)-1 && off == sect.end) {
\t\t\t\t\tres = sect.baseaddr + off - sect.vaddr
\t\t\t\t}
\t\t\t}
\t\t} else {
''' + '\n'.join('\t' + line for line in old_addr.splitlines()) + '''
\t\t}'''

# Do not treat the older textOff-only marker as a complete patch.
for marker, old, new in [
    ('// ssa2llvm: textsect binary search', old_off, new_off),
    ('// ssa2llvm: virtual textsect binary search', old_addr, new_addr),
]:
    if marker not in text:
        if text.count(old) != 1:
            raise SystemExit(f"[yaklib] unsupported Go runtime source for {marker}: {path}")
        text = text.replace(old, new)

if 'var ssa2llvmTextAddrMap' not in text:
    # Nonzero slice header forces file-backed data, so the post-link patch can
    # install the map without adding fields to runtime.moduledata. Ordinary Go
    # binaries retain this one-entry sentinel and the original fallback.
    text += '\n// ssa2llvm: final-link virtual-order text map (firstmoduledata only).\nvar ssa2llvmTextAddrMap = []textsect{{}}\n'

path.write_text(text)
print(f"[yaklib] patched both Go runtime textsect lookups: {path}")
PY
