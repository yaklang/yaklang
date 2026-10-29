"""Build the separately hashed direct-codec probe from the clean current source."""
from pathlib import Path
root=Path(__file__).parent
source=(root/'build_stats_api_probe.py').read_text('utf8')
old="helper=root/'ESP-NATT-whole-plan-before-01'/'stats_api_probe.go'"
assert source.count(old)==1;source=source.replace(old,"helper=root/'ospfv3_direct_boundary_probe38.go'")
old="binary=out/'stats-api-probe.exe'"
assert source.count(old)==1;source=source.replace(old,"binary=out/'ospf-direct-probe.exe'")
exec(compile(source,str(Path(__file__)),'exec'),globals())
