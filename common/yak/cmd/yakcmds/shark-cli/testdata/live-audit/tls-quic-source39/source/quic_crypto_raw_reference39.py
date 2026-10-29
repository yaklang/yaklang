"""Raw JSON reference for CRYPTO FT_NONE values absent from PDML attributes.

Source CRYPTO bytes/digests were independently AEAD-authenticated before this
export. This closes a reference-selector gap, not a parser/capture correction.
"""
import argparse,collections,hashlib,json,os,shutil,subprocess,time,traceback
from pathlib import Path

def sha(b):return hashlib.sha256(b).hexdigest()
def file_sha(p):return sha(Path(p).read_bytes())
def read(p):return json.loads(Path(p).read_text('utf8'))
def write(p,v):
    with Path(p).open('x',encoding='utf8')as f:json.dump(v,f,indent=2);f.write('\n')
def containers(tree,key):
    if isinstance(tree,dict):
        if key in tree:yield tree
        for name,value in tree.items():
            if not name.endswith('_raw'):yield from containers(value,key)
    elif isinstance(tree,list):
        for value in tree:yield from containers(value,key)
def one(v):
    if isinstance(v,list):assert len(v)==1;v=v[0]
    return v
def raws(v):return[v]if isinstance(v,list)and v and isinstance(v[0],str)else v

def main():
    ap=argparse.ArgumentParser(description=__doc__)
    for n in('source','reference','out'):ap.add_argument('--'+n,type=Path,required=True)
    a=ap.parse_args();assert not a.out.exists();a.out.mkdir();assert shutil.disk_usage('C:/').free>20*2**30
    assert file_sha(a.source/'summary.json')=='6020e5c719c3d02eb42881671e0dfe8845a59ec0cebbac1d49769a5a8d826859';source=read(a.source/'summary.json');assert file_sha(a.source/'packet-source.jsonl')==source['packet_source_SHA256']
    rp=read(a.reference/'producer.json');base=rp['executions'][0]['arguments'];assert base[-2:]==['-T','pdml'];args=base[:-2]+['-Y','quic.crypto.length','-T','json','-x','--no-duplicate-keys']
    expected=collections.defaultdict(collections.Counter)
    for line in(a.source/'packet-source.jsonl').read_text('utf8').splitlines():
        r=json.loads(line)
        for fr in r.get('frames',[]):
            if fr['name']=='CRYPTO':expected[r['frame']][fr['offset'],fr['length'],fr['data']['SHA256']]+=1
    bound={str(p):file_sha(p)for p in[Path(__file__),a.reference/'producer.json',a.source/'summary.json',a.source/'packet-source.jsonl',Path(args[0])]}
    for key,value in rp['inputs_before'].items():assert file_sha(key)==value
    cfg=a.out/'empty-config';cfg.mkdir();env=os.environ.copy();env.update(APPDATA=str(cfg),WIRESHARK_CONFIG_DIR=str(cfg));env.pop('SSLKEYLOGFILE',None)
    try:
        start=time.monotonic()
        with(a.out/'reference.json').open('xb')as out,(a.out/'stderr').open('xb')as err:p=subprocess.run(args,stdout=out,stderr=err,env=env,creationflags=0x08000000,timeout=120)
        assert p.returncode==0;actual=collections.defaultdict(collections.Counter)
        for packet in read(a.out/'reference.json'):
            layers=packet['_source']['layers'];frame=int(one(layers['frame']['frame.number']))
            for c in containers(layers,'quic.crypto.crypto_data_raw'):
                offset=int(one(c['quic.crypto.offset']),0);n=int(one(c['quic.crypto.length']),0)
                for raw in raws(c['quic.crypto.crypto_data_raw']):
                    data=bytes.fromhex(raw[0]);assert len(data)==n and raw[2]==n;actual[frame][offset,n,sha(data)]+=1
        assert actual==expected
        assert bound=={p:file_sha(p)for p in bound}
        for key,value in rp['inputs_before'].items():assert file_sha(key)==value
        report=dict(scope=__doc__,source_summary_SHA256=file_sha(a.source/'summary.json'),source_packet_grid_SHA256=file_sha(a.source/'packet-source.jsonl'),parent_reference_producer_SHA256=file_sha(a.reference/'producer.json'),arguments=args,exit_code=p.returncode,seconds=round(time.monotonic()-start,3),export_SHA256=file_sha(a.out/'reference.json'),stderr_SHA256=file_sha(a.out/'stderr'),physical_reference_frames=len(actual),complete_source_CRYPTO_occurrences=sum(sum(c.values())for c in expected.values()),all_source_offset_length_and_raw_digest_occurrences_equal=True,source_not_modified=True,no_native_or_Yak_output_read=True,inputs=bound,inputs_unchanged=True,FT_NONE_PDML_omission_is_not_empty_data=True)
        write(a.out/'producer.json',report);print(json.dumps(dict(producer=str(a.out/'producer.json'),SHA256=file_sha(a.out/'producer.json'),CRYPTO_occurrences=report['complete_source_CRYPTO_occurrences'],all_equal=True)),flush=True)
    except Exception:
        write(a.out/'controller-failure.json',dict(error=traceback.format_exc()));raise
if __name__=='__main__':main()
