"""Execute an already-built external stats/API probe; never invokes Go."""
import argparse,hashlib,json,os,subprocess,time,traceback
from pathlib import Path

def sha(p):
    h=hashlib.sha256()
    with Path(p).open('rb') as f:
        for b in iter(lambda:f.read(1<<20),b''):h.update(b)
    return h.hexdigest()
def write(p,v):p.write_text(json.dumps(v,indent=2,ensure_ascii=False)+'\n',encoding='utf8')
def main():
    p=argparse.ArgumentParser(description=__doc__)
    for name in ['binary','build-receipt','capture','keylog','out']:p.add_argument('--'+name,type=Path,required=True)
    p.add_argument('--source',required=True);a=p.parse_args();a.out.mkdir(exist_ok=False)
    try:
        helper=Path(__file__).with_name('stats_api_probe.go')
        build=json.loads(a.build_receipt.read_text(encoding='utf-8-sig'))
        assert build['source']==a.source and build['exit_code']==0
        assert build['binary_SHA256']==sha(a.binary)
        assert build['source_file_SHA256']==sha(helper),'External helper build must bind this exact frozen source file'
        paths=[a.binary,a.build_receipt,a.capture,a.keylog,helper,Path(__file__)]
        before={str(p):sha(p) for p in paths}
        assert before[str(a.capture)]=='ecbca543fbe011a85c8876420fcb2c244b9290ad653d13cd1a574158167073f4'
        args=[str(a.binary),'-capture',str(a.capture),'-keylog',str(a.keylog),'-ignore-embedded-secrets=true']
        start=time.time()
        with (a.out/'probe.jsonl').open('xb') as stdout,(a.out/'stderr.log').open('xb') as stderr:
            result=subprocess.run(args,stdout=stdout,stderr=stderr)
        after={p:sha(p) for p in before}
        receipt=dict(arguments=args,source=a.source,exit_code=result.returncode,seconds=time.time()-start,
            binary_SHA256=sha(a.binary),probe_source_SHA256=sha(helper),build_receipt_SHA256=sha(a.build_receipt),
            source_before=before,source_after=after,sources_byte_equal=before==after,
            export_SHA256=sha(a.out/'probe.jsonl'),stderr_SHA256=sha(a.out/'stderr.log'),
            auditor_SHA256=sha(__file__),capture_SHA256=sha(a.capture),explicit_keylog_used=True,
            keylog_SHA256=sha(a.keylog),ignore_embedded_secrets=True,AllowIncomplete=True)
        write(a.out/'producer.json',receipt)
        assert before==after and result.returncode==0
        print(json.dumps(dict(producer=str(a.out/'producer.json'),SHA256=sha(a.out/'producer.json'))))
    except Exception:
        write(a.out/'failure.json',dict(failure=traceback.format_exc(),no_success_certificate=True));raise
if __name__=='__main__':main()
