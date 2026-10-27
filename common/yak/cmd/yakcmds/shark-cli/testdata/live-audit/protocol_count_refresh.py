"""Refresh literal counts after exact-message recovery, without normalizing labels.
Recheck original capture digests, complete exports and their constrained SDK/CLI
certificates. Reuse frozen reference counts only for those exact same inputs.
"""
import argparse,collections,hashlib,json
from pathlib import Path
ap=argparse.ArgumentParser(description=__doc__)
for n in['prior','evidence-root','weber-capture','sharon-capture','json-out','markdown-out']:ap.add_argument('--'+n,type=Path,required=True)
ap.add_argument('--source',required=True);a=ap.parse_args()
def sha(p):
 h=hashlib.sha256()
 with p.open('rb')as f:
  for w in iter(lambda:f.read(1<<20),b''):h.update(w)
 return h.hexdigest()
def load(p):return json.loads(p.read_text(encoding='utf-8-sig'))
def counts(p):
 out=collections.Counter()
 for line in p.open(encoding='utf-8-sig'):out[json.loads(line)['protocol']]+=1
 return dict(out)
prior_sha=sha(a.prior);out=load(a.prior);out.update(scope=__doc__,source=a.source,generator_sha256=sha(Path(__file__)),prior_count_snapshot_sha256=prior_sha)
for name,capture in[('weber',a.weber_capture),('sharon',a.sharon_capture)]:
 assert not any((p/'.git').exists()for p in[capture.resolve(),*capture.resolve().parents])
 v=out['inputs'][name];assert sha(capture)==v['capture_sha256'];d=a.evidence_root/name;sdk=load(d/'compat-export.json');cli=load(d/'compat-CLI.json');sp=d/'events.jsonl';cp=d/'yak.jsonl';assert sdk['after_sha256']==sha(sp)and cli['after_sha256']==sha(cp)and cli['SDK_sha256']==sdk['after_sha256'];assert not sdk['other_differences']and not cli['other_connection_differences'];sc,cc=counts(sp),counts(cp);assert sum(sc.values())==sdk['new_PDUs'];assert sum(cc.values())==cli['physical_rows']==v['physical_frames'];assert cc==v['current_labels'],'review actual raw label changes before replacing frozen counts';expected=dict(v['native_message_protocols']);expected['ssh']=expected.get('ssh',0)+10;assert sc==expected,'review any other actual native protocol-count delta';v.update(native_message_records=sdk['new_PDUs'],native_message_protocols=sc,SDK_export_sha256=sha(sp),CLI_export_sha256=sha(cp),SDK_invariants_sha256=sha(d/'compat-export.json'),CLI_invariants_sha256=sha(d/'compat-CLI.json'),exact_capture_digest_rechecked=True,reference_counts_reused_for_same_immutable_capture=True)
a.json_out.write_text(json.dumps(out,indent=2)+'\n',encoding='utf8');lines=[f'当前显示标签计数，源码 `{a.source}`。输入原包不入库。','','这里逐字保留 Wireshark 和 Shark 的列标签，帧数与原生消息数分开。同名计数不代表解析正确；大小写、子协议、分片及拒绝策略也会产生标签差异。字段、流、重组和来源的验证见主报告及各协议证书。','']
for name,v in out['inputs'].items():
 lines += [f'## {name}','',f"物理帧 {v['physical_frames']:,}；原生消息 {v['native_message_records']:,}。",'','| 排名 | WS 原始标签 | WS 帧数 | Shark 完全同名帧数 | 同名差值 |','|---:|---|---:|---:|---:|']
 for i,r in enumerate(v['raw_label_differences'][:30],1):
  label=r['label'].replace('|','\\|').replace('\n',' ');lines.append(f"| {i} | {label} | {r['reference_frames']:,} | {r['same_label_native_frames']:,} | {r['same_label_delta']:+,} |")
 lines.append('')
a.markdown_out.write_text('\n'.join(lines),encoding='utf8');print(json.dumps({'source':a.source,'aggregate_sha256':sha(a.json_out),'frames':{n:v['physical_frames']for n,v in out['inputs'].items()},'native_messages':{n:v['native_message_records']for n,v in out['inputs'].items()},'raw_display_label_changes':[]}))
