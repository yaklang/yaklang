"""Exact598-result and sole production-call bridge for the public helper fix."""
from pathlib import Path
import hashlib,json,subprocess
root=Path(__file__).parent;repo=Path(r'C:\Users\V\.codex\worktrees\shark-protocol-infrastructure\yaklang')
out=root/'OSPFv2-helper-closure43-01';out.mkdir(exist_ok=False)
def sha(p):return hashlib.sha256(Path(p).read_bytes()).hexdigest()
before=root/'OSPFv2-direct-probe-candidate41-01';after=root/'OSPFv2-helper-direct-probe43-01'
old=before/'direct.jsonl';new=after/'direct.jsonl';assert old.read_bytes()==new.read_bytes()and sha(old)==sha(new)=='e922c4409235827168d3e61bb4f0521de380ba6d43624c1b4fec5b06fb5f8112'
path='common/bin-parser/parser/stream_parser/ospfv2_value.go'
committed=subprocess.check_output(['git','show','cd7af6a67e998ed1a6466a11f2b37c66fad742a8:'+path],cwd=repo).decode('utf8').replace('\r\n','\n')
current=(repo/path).read_text('utf8').replace('\r\n','\n')
guard='\tn := int(binary.BigEndian.Uint16(w[2:4]))\n\tif n < 24 || n > len(w) {\n\t\treturn 0xffff\n\t}\n\tw = w[:n]\n'
assert current.count(guard)==1 and current.replace(guard,'',1)==committed
assert committed.count('OSPFv2Checksum(w[:n])')==1
assert 'if !ok || n < 24 || n > uint64(len(w)) {'in committed
audit=root/'OSPFv2-helper-direct-field-audit43-01/summary.json';a=json.loads(audit.read_text('utf8'));assert a['failure_count']==0 and a['counts']['scenarios']==598
result=dict(scope=__doc__,prior_native_source='cd7af6a67e998ed1a6466a11f2b37c66fad742a8',new_native_or_CLI_or_UI_execution_claim=False,whole_result_lines=598,literal_export_SHA256=sha(new),ID_mapping=False,normalization=False,public_helper_new_guard_only=True,sole_packet_integrity_caller_already_validates_n_and_passes_exact_w_slice_n=True,all598_fields_and_errors_literal=True,independent_after_audit_SHA256=sha(audit),before_producer_SHA256=sha(before/'producer.json'),after_producer_SHA256=sha(after/'producer.json'),checker_SHA256=sha(__file__))
(out/'summary.json').write_text(json.dumps(result,indent=2)+'\n',encoding='utf8')
dest=repo/'common/yak/cmd/yakcmds/shark-cli/testdata/live-audit/ospfv2-control42';mp=dest/'source-manifest.json';manifest=json.loads(mp.read_text('utf8'));initial=sha(mp);assert initial=='2cd61567df719cd35ae41cc2711066409c9765968c7911fa8d1025bcc6f204a9'
for name in('run_OSPFv2_direct43.py','close_OSPFv2_helper43.py'):
 src=root/name;target=dest/name;assert not target.exists();target.write_bytes(src.read_bytes());assert sha(target)==sha(src)
 manifest['files'].append(dict(path=name,original_path=str(src),bytes=src.stat().st_size,SHA256=sha(src),source_language='Python'))
manifest['source_files']=len(manifest['files']);manifest['source_bytes']=sum(r['bytes']for r in manifest['files']);manifest['initial41_source_manifest_SHA256']=initial;manifest['helper_followup']='Public checksum API declared-length guard; new46-check run and exact598 direct-result bridge, no new native/UI run claim.'
mp.write_text(json.dumps(manifest,indent=2)+'\n',encoding='utf8')
print(json.dumps(dict(summary=str(out/'summary.json'),SHA256=sha(out/'summary.json'),source_manifest_SHA256=sha(mp),source_files=manifest['source_files'],source_bytes=manifest['source_bytes'])))
