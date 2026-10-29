"""Run unchanged34 source inputs through source-frozen RSVP build02 and RFC-corrected oracle02."""
from pathlib import Path
import hashlib

root = Path(__file__).parent
source = (root / 'run_RSVP_direct46.py').read_text('utf8')
changes = {
    'RSVP-direct-API-source-plan46-01': 'RSVP-direct-API-source-plan46-02',
    'rsvp_direct_api_checker46.py': 'rsvp_direct_api_checker46_v02.py',
    '2b5b6c499ddd6f14456ea39f62e6ef3b20ee04f343893ea7ebefcc9b216c32fa': 'beef22165958b504c2f2bcbdeec194227fa92dd86f949918c7c20fe873d44781',
    '0a4146d707184cd7426e472cdd9636258b09a2f47a9c28a62898d19f1d32690a': 'bcb932e9499d55ae2c5538c7e425fa4755d62faea993c2171a38e32fb11b6e1d',
    'RSVP-direct-build46-01/rsvp-direct-probe.exe': 'RSVP-direct-build46-02/rsvp-direct-probe.exe',
}
assert hashlib.sha256((root / 'run_RSVP_direct46.py').read_bytes()).hexdigest() == '205dfed57bc9dbe8726892f1182424bf48aeba05e3ccf6f87c8371293be0c4c1'
for old, new in changes.items():
    assert source.count(old) == 1, old
    source = source.replace(old, new)
exec(compile(source, str(Path(__file__)), 'exec'), globals())
