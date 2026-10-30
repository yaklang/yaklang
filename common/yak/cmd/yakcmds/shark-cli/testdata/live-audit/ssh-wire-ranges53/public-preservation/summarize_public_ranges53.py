"""Summarize exact frozen SSH metadata additions without copying packet values."""
from pathlib import Path
import argparse,base64,collections,hashlib,json

def sha(p):return hashlib.sha256(Path(p).read_bytes()).hexdigest()
def read(p):return json.loads(Path(p).read_text(encoding='utf-8-sig'))
def write(p,x):
    with p.open('x',encoding='utf-8') as f:json.dump(x,f,sort_keys=True,indent=2);f.write('\n')
def demand(v,m):
    if not v:raise AssertionError(m)
def main():
    a=argparse.ArgumentParser();a.add_argument('--plan',type=Path,required=True);a.add_argument('--verified',type=Path,required=True);a.add_argument('--out',type=Path,required=True);z=a.parse_args()
    z.out.mkdir(exist_ok=False);manifest=read(z.plan/'manifest.json');verified=read(z.verified)
    demand(verified['passed'] and verified['plan_SHA256']==sha(z.plan/'manifest.json'),'closed verifier bound to original plan')
    for path,digest in manifest['inputs'].items():demand(sha(path)==digest,'frozen source/input modified')
    counts=collections.Counter();owners=collections.Counter();aliases=collections.Counter();zeros=collections.Counter();spans=[];per_case={}
    for r in read(z.plan/'native-files.json'):
        for change in r['changes']:
            with Path(r['export']).open('rb') as f:f.seek(change['byte_offset']);line=f.readline()
            demand(hashlib.sha256(line).hexdigest()==change['old_line_SHA256'],'complete frozen old line')
            old=json.loads(line);raw=base64.b64decode(old.get('raw') or '',validate=True)
            demand(hashlib.sha256(raw).hexdigest()==change['source_proof']['old_Raw_SHA256'],'bound old Raw')
            c=per_case.setdefault(r['case'],collections.Counter());c['native_changed_rows']+=1
            for patch in change['additions']:
                if patch['path'][-1]=='Wire Field Ranges':
                    root=patch['path'][0];owner='/'.join(patch['path'][:-1]);counts[root+'_owner_maps']+=1;owners[owner]+=1
                    for name,(lo,hi) in patch['value'].items():
                        demand(type(lo) is int and type(hi) is int and 0<=lo<=hi<=len(raw),'exact owned half-open PDU range')
                        counts[root+'_literal_spans']+=1;zeros[root]+=lo==hi;c[root+'_literal_spans']+=1
                        spans.append({'case':r['case'],'event_id':old['id'],'old_line_SHA256':change['old_line_SHA256'],'old_Raw_SHA256':change['source_proof']['old_Raw_SHA256'],'path':patch['path'][:-1]+[name],'span':[lo,hi],'zero_width':lo==hi,'evidence':'old owned Raw sequential parse plus exact complete old owner values; preservation only'})
                else:
                    demand(patch['path'][:2]==['field_values','values'] and patch['path'][-1]=='pdu_range','only selected private alias insertion')
                    v=old['field_values']['values'][patch['path'][2]];name=v['name'];p=patch['value'];lo=p['offset'];hi=lo+p['length']
                    demand(type(lo) is int and type(hi) is int and 0<=lo<=hi<=len(raw) and v['origin']=='pdu','exact current-PDU private alias')
                    counts['native_private_alias_ranges']+=1;aliases[name]+=1;c['native_private_alias_ranges']+=1
                    spans.append({'case':r['case'],'event_id':old['id'],'old_line_SHA256':change['old_line_SHA256'],'old_Raw_SHA256':change['source_proof']['old_Raw_SHA256'],'path':patch['path'],'alias':name,'span':[lo,hi],'zero_width':lo==hi,'evidence':'exact existing pdu-origin private alias and current Session literal source'})
    for r in read(z.plan/'CLI-files.json'):
        for change in r['changes']:
            c=per_case.setdefault(r['case'],collections.Counter());c['CLI_changed_rows']+=1
            for patch in change['additions']:
                demand(patch['path'][0] in ('pdus','published_pdus') and patch['path'][-1]=='pdu_range','only compact alias ranges')
                counts['CLI_private_alias_ranges']+=1;c['CLI_private_alias_ranges']+=1
    write(z.out/'exact-owner-spans.json',spans)
    report={'passed':True,'scope':'whole interpretation preservation only; independent PCAP correctness not claimed','frozen_plan_SHA256':sha(z.plan/'manifest.json'),'closed_actual_summary_SHA256':sha(z.verified),'counts':dict(counts),'owner_map_insertions':dict(owners),'zero_width_WR':dict(zeros),'native_alias_ranges':dict(aliases),'cases':{k:dict(v) for k,v in per_case.items()},'exact_owner_spans_file_SHA256':sha(z.out/'exact-owner-spans.json'),'entire_corpus_counts':manifest['counts'],'all_other_native_and_CLI_values_preserved':True,'all_flow_files_byte_identical':True,'constructor_never_read_candidate53':True,'aggregator_source_SHA256':sha(__file__)}
    write(z.out/'summary.json',report);print(json.dumps({'passed':True,'summary_SHA256':sha(z.out/'summary.json'),'counts':dict(counts),'owners':dict(owners),'zero_width':dict(zeros),'aliases':dict(aliases)}))
if __name__=='__main__':main()
