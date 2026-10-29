"""Freeze the full older per-PDU JSON shape, including its optional fields."""
from pathlib import Path
import hashlib,json,subprocess
root=Path(__file__).parent
repo=Path(r'C:\Users\V\.codex\worktrees\shark-protocol-infrastructure\yaklang')
path='common/yak/cmd/yakcmds/shark-cli/packet_observation.go'
old=subprocess.check_output(['git','show','68527c4d9355d2df37c5c0c9b8bc19f7766ca9c6:'+path],cwd=repo)
current=subprocess.check_output(['git','show','d6772965c669e288705f7e0d0e25c62972226832:'+path],cwd=repo)
assert old==current and b'row["tcp_flow_id"] = e.TCPFlowID'in old and b'row["summary"], row["error"], row["expert"] = e.Summary, e.Error, e.ExpertCode'in old
before=root/'audit_fresh_CLI39_02.py';after=root/'audit_fresh_CLI39_03.py';assert not after.exists()
s=before.read_text('utf8')
anchor="            event=byid[ident];assert set(pdu)=={'id','protocol','status','completeness','response_to','byte_source','fields','field_values'}"
assert s.count(anchor)==1
s=s.replace(anchor,"""            event=byid[ident]
            expected_keys={'id','protocol','status','completeness','response_to','byte_source','fields','field_values'}
            if event.get('tcp_flow_id',0):
                expected_keys.add('tcp_flow_id');assert pdu['tcp_flow_id']==event['tcp_flow_id']
            if event.get('error','')or event.get('expert',''):
                expected_keys.update(('summary','error','expert'))
                for key in('summary','error','expert'):assert pdu[key]==event.get(key,'')
            assert set(pdu)==expected_keys,(name,frame,ident,'public PDU shape')""")
after.write_text(s,encoding='utf8')
proof=dict(scope=__doc__,historical_failure='Checker02 omitted documented tcp_flow_id and error/summary/expert optional keys;not a product defect.',old_auditor_SHA256=hashlib.sha256(before.read_bytes()).hexdigest(),new_auditor_SHA256=hashlib.sha256(after.read_bytes()).hexdigest(),immutable_API_SHA256=hashlib.sha256(old).hexdigest(),old_current_API_equal=True,source_packet_values_labels_and_ranges_unchanged=True)
(root/'fresh-CLI-source39-audit02/failure-shape-bridge.json').write_text(json.dumps(proof,indent=2)+'\n',encoding='utf8');print(json.dumps(proof))
