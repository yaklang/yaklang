"""Reuse the dirty-tree exact-source build guard for the RSVP direct probe."""
from pathlib import Path
source=Path(__file__).with_name('build_OSPFv2_guarded41.py').read_text('utf8')
old="root/'ospfv2_direct_probe41.go'";assert source.count(old)==1
source=source.replace(old,"root/'rsvp_direct_probe46.go'")
old="'ospfv2-direct-probe.exe'";assert source.count(old)==1
source=source.replace(old,"'rsvp-direct-probe.exe'")
exec(compile(source,str(Path(__file__)),'exec'),globals())
