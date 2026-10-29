"""Compile the native console QA source with the existing frozen-source guard."""
from pathlib import Path
source=Path(__file__).with_name('build_OSPFv2_guarded41.py').read_text('utf8')
old="root/'ospfv2_direct_probe41.go'";assert source.count(old)==1
source=source.replace(old,"repo/'common/yak/cmd/yakcmds/shark-cli/testdata/live-audit/discovery/console/main_windows.go'")
old="'ospfv2-direct-probe.exe'";assert source.count(old)==1
source=source.replace(old,"'native-console.exe'")
exec(compile(source,str(Path(__file__)),'exec'),globals())
