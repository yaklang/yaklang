"""Use the same bounded dirty-tree snapshot/build policy for the GTPv2 probe."""
from pathlib import Path
base=Path(__file__).with_name('build_OSPFv2_guarded41.py');source=base.read_text('utf8')
assert source.count("root/'ospfv2_direct_probe41.go'")==1
source=source.replace("root/'ospfv2_direct_probe41.go'","root/'gtpv2_direct_probe44.go'")
assert source.count("'ospfv2-direct-probe.exe'")==1
source=source.replace("'ospfv2-direct-probe.exe'","'gtpv2-direct-probe.exe'")
exec(compile(source,str(Path(__file__)),'exec'),globals())
