"""Use the native Windows runner without changing frozen GTPv2 UI facts."""
from pathlib import Path
source=Path(__file__).with_name('run_Windows_OSPFv2_42.py').read_text('utf8')
assert 'OSPFv2' in source
source=source.replace('OSPFv2','GTPv2-C')
exec(compile(source,str(Path(__file__)),'exec'),globals())
