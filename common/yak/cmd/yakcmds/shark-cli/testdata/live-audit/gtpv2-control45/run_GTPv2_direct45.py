"""Recheck unchanged frozen30 facts after selected IE receive-rule fixes."""
from pathlib import Path
source=Path(__file__).with_name('run_GTPv2_direct44.py').read_text('utf8')
old='GTPv2-direct-build44-01/gtpv2-direct-probe.exe';assert source.count(old)==1
source=source.replace(old,'GTPv2-direct-build45-01/gtpv2-direct-probe.exe')
exec(compile(source,str(Path(__file__)),'exec'),globals())
