"""Same frozen598 inputs against the declared-length checksum-helper repair."""
from pathlib import Path
import hashlib
base=Path(__file__).with_name('run_OSPFv2_direct41.py')
assert hashlib.sha256(base.read_bytes()).hexdigest()=='9fab7b797be2c9255da14cc9f087629eb850a32f34e22506b1e50319cc59ec45'
source=base.read_text('utf8');assert source.count('OSPFv2-direct-build41-01')==1
source=source.replace('OSPFv2-direct-build41-01','OSPFv2-direct-build43-01')
exec(compile(source,str(Path(__file__)),'exec'),globals())
