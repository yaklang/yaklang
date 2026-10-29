"""Freeze a real missing-prefix page from independent packet/CID source bytes."""
from pathlib import Path
import hashlib, importlib.util, json, sys

phase=Path(__file__).parent
repo=Path(r'C:\Users\V\.codex\worktrees\shark-protocol-infrastructure\yaklang')
sha=lambda p:hashlib.sha256(Path(p).read_bytes()).hexdigest()
source=phase/'fresh-TLS-QUIC-source39-01/QUIC-source01-audit01/packet-source.jsonl'
record=next(json.loads(s)for s in source.read_text().splitlines()if json.loads(s)['frame']==1011)
assert record['authenticated'] and record['connection']==2
reader=repo/'common/yak/cmd/yakcmds/shark-cli/testdata/live-audit/vxlan_endpoint_projection_audit.py'
sys.path.insert(0,str(reader.parent));spec=importlib.util.spec_from_file_location('prefix_capture_reader40',reader);m=importlib.util.module_from_spec(spec);spec.loader.exec_module(m)
capture=phase/'fresh-QUIC-epochs39-01/live.pcap';records=list(m.capture_records(capture));raw=records[1010]['wire']
lo,hi=record['packet_range'];wire=raw[lo:hi]
assert hashlib.sha256(wire).hexdigest()==record['packet_SHA256'] and len(wire)==1200
cid=bytes.fromhex(record['destination_CID']);assert wire[1:1+len(cid)]==cid and len(cid)==8
out=phase/'Windows-HTTP3-prefix-source-plan40-01';out.mkdir();(out/'physical.bin').write_bytes(raw);(out/'PDU.bin').write_bytes(wire)
helper=phase/'windows-PIM-helper26/native-console.exe';renderer=phase/'windows-identity-1MiB-5c88a-02/frozen-inputs/renderer.py'
v=dict(scope=__doc__,frame=1011,pdu=1021,query='fields pdu.id == 1021',protocol='HTTP/3',physical_records=2018,
    capture=str(capture),capture_SHA256=sha(capture),keys=str(phase/'fresh-QUIC-epochs39-01/session.keys'),
    keylog_SHA256=sha(phase/'fresh-QUIC-epochs39-01/session.keys'),source_packet_grid_SHA256=sha(source),
    physical_file=str(out/'physical.bin'),physical_SHA256=sha(out/'physical.bin'),physical_bytes=len(raw),
    PDU_file=str(out/'PDU.bin'),PDU_SHA256=sha(out/'PDU.bin'),PDU_physical_range=[lo,hi],PDU_bytes=1200,
    field='DCID',field_range=[1,9],field_physical_range=[lo+1,lo+9],field_wire_hex=cid.hex(),
    helper=str(helper),helper_SHA256=sha(helper),renderer=str(renderer),renderer_SHA256=sha(renderer),
    default_history_mib=64,default_packet_cache=20000,no_budget_increase=True,
    progressive_stack=['Loopback','IPv4','UDP','QUIC','HTTP/3'],
    required_summary='stream prefix missing',required_scope='authenticated STREAM observation',
    identity_limit='h3 connection plus client bidirectional STREAM only; no decoded message, extension, request method or transaction',
    provenance='New actual page from same original real capture; source bytes frozen before new candidate exports; PDU identity is unchanged baseline slot, not renumbered')
f=out/'case.json';f.write_text(json.dumps(v,indent=2)+'\n',encoding='utf8');print(json.dumps(dict(case=str(f),SHA256=sha(f))))
