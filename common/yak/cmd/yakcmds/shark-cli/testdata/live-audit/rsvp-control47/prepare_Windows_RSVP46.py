"""Freeze official RSVP source-field console checks before any candidate page."""
from pathlib import Path
import hashlib, json, struct

root = Path(__file__).parent
out = root / 'Windows-RSVP-source-plan46-01'
out.mkdir(exist_ok=False)

def sha(p): return hashlib.sha256(Path(p).read_bytes()).hexdigest()
def read(p): return json.loads(Path(p).read_text('utf8'))
def put(p, value):
    with p.open('x', encoding='utf8') as f:
        json.dump(value, f, indent=2)
        f.write('\n')

grid = root / 'RSVP-baseline-source45-01/source-grid.json'
assert sha(grid) == '9d31302c649ded298b26474aa98d1e245e2116086ac839bd48355852606262ad'
official = root / 'RSVP-primary-download45-01/rsvp-PATH-RESV.pcap'
# The downloaded name is read from the frozen download manifest, never guessed.
downloads = read(root / 'RSVP-primary-download45-01/download-manifest.json')
captures = list((root / 'RSVP-primary-download45-01').glob('*.pcap'))
assert len(captures) == 1
official = captures[0]
assert sha(official) == 'aa7d8448cd5eaf17112e67adf3e801cb45119e62c63c75f132ad596edfc10361'
helper = root / 'Windows-console-ack-build45-01/native-console.exe'
renderer = root / 'windows-identity-1MiB-5c88a-02/frozen-inputs/renderer.py'
assert sha(helper) == '12e0aa1b8b3cfb4696e1d680334dae490e269de37773e01aafa35bc4a6d8b68c'
assert sha(renderer) == '427379a3b9c1ac7e8257a81d3d6e478904fd96659468f231bbec57a55e336e83'
cases = []
for name, number, key, text, expected in [
    ('Official-Path-TIME-VALUES', 1, 'Refresh Period Milliseconds', '30000', 30000),
    ('Official-Resv-Fixed-Filter', 7, 'Option Vector', '10 (Fixed Filter)', 10),
]:
    row = next(r for r in read(grid) if r['frame'] == number)
    raw = Path(row['physical_path']).read_bytes()
    wire = Path(row['PDU_path']).read_bytes()
    assert hashlib.sha256(raw).hexdigest() == row['physical_SHA256']
    assert hashlib.sha256(wire).hexdigest() == row['PDU_SHA256']
    first, last = row['PDU_physical_range']
    assert raw[first:last] == wire
    leaves = [v for v in row['leaves'] if v['path'].endswith('.' + key)]
    assert len(leaves) == 1
    leaf = leaves[0]
    assert leaf['literal'] and leaf['value'] == expected
    lo, hi = leaf['range']
    assert int.from_bytes(wire[lo:hi], 'big') == expected
    physical = out / (name + '.frame')
    physical.write_bytes(raw)
    capture = out / (name + '.pcap')
    sec, nanos = divmod(row['time_ns'], 10**9)
    assert nanos % 1000 == 0 and row['captured_length'] == row['original_length'] == len(raw)
    capture.write_bytes(struct.pack('<IHHIIII', 0xa1b2c3d4, 2, 4, 0, 0, 65535, 1) + struct.pack('<IIII', sec, nanos // 1000, len(raw), len(raw)) + raw)
    cases.append(dict(name=name, original_public_capture=str(official), original_public_capture_SHA256=sha(official), original_public_frame=number, source_grid=str(grid), source_grid_SHA256=sha(grid), capture=str(capture), capture_SHA256=sha(capture), physical_file=str(physical), physical_SHA256=sha(physical), physical_bytes=len(raw), PDU_range=[first,last], PDU_SHA256=sha(row['PDU_path']), PDU_bytes=len(wire), query='proto:rsvp', field_name=key, field_value=text, field_down=36, field_PDU_range=[lo,hi], field_physical_range=[first+lo,first+hi], field_hex=wire[lo:hi].hex(), progressive_stack=['Ethernet','IPv4','RSVP'], helper=str(helper), helper_SHA256=sha(helper), renderer=str(renderer), renderer_SHA256=sha(renderer), packet_cache_default=20000, history_budget_default_mib=64, derived_one_packet_not_live=True, authored_control=False))
put(out / 'cases.json', cases)
put(out / 'plan.json', dict(scope=__doc__, candidate_read=False, source_cases=len(cases), cases_SHA256=sha(out/'cases.json'), constructor_SHA256=sha(__file__), source_plan_sha256=sha(grid), console_cells_not_terminal_pixels=True))
print(json.dumps(dict(plan=str(out/'plan.json'), SHA256=sha(out/'plan.json'), cases=[dict(name=c['name'], field=c['field_name'], pdu_range=c['field_PDU_range']) for c in cases])))
