"""Extract original malformed-version witnesses without altering packet bytes.

Usage: ip_version_original_controls.py SOURCE_PCAPNG FRAGMENT_PROOF OUTPUT_DIR
The source proof selects frames; every selected version mismatch is independently
read from the original carrier bytes. Filtering renumbers frames and maps the
single Ethernet interface to classic PCAP interface zero, explicitly in a ledger.
"""
import hashlib,json,struct,sys
from pathlib import Path
sys.path.insert(0,str(Path(__file__).resolve().parents[1]))
from capture_clock_wire_audit import physical


def sha(w):return hashlib.sha256(w).hexdigest()
def run(source,proof,out):
    for p in[source,proof,out]:assert not any((v/'.git').exists()for v in[p.resolve(),*p.resolve().parents])
    evidence=json.loads(proof.read_text('utf8'));assert evidence['capture_SHA256']==sha(source.read_bytes())
    wanted={v['source']['frame']:v['source']for v in evidence['known_invalid_IP_version_diagnostics']if v['side']=='after'}
    assert len(wanted)==269
    original=physical(source);out.mkdir(parents=True,exist_ok=False);ledger=[]
    with(out/'observation.pcap').open('xb')as f:
        f.write(struct.pack('<IHHIIII',0xa1b23c4d,2,4,0,0,65535,1))
        for number,entry in sorted(wanted.items()):
            record=original[number-1];raw=record['wire'];assert record['link']==1 and len(raw)>=34 and raw[12:14]==b'\x08\0'
            got=raw[14]>>4;assert got!=4 and got==entry['observed_IP_version']and entry['carrier_expected_IP_version']==4 and sha(raw)==entry['raw_SHA256']
            stamp=record['time_ns'];f.write(struct.pack('<IIII',stamp//1000000000,stamp%1000000000,len(raw),len(raw)));f.write(raw)
            ledger.append(dict(number=len(ledger)+1,source_frame=number,source_domain=dict(Section=record['section'],Interface=record['interface'],Encapsulation=''),output_domain=dict(Section=0,Interface=0,Encapsulation=''),time_ns=stamp,wire_SHA256=sha(raw),observed_IP_version=got,expected_IP_version=4))
    manifest=dict(scope=__doc__,generator_SHA256=sha(Path(__file__).read_bytes()),source_capture_SHA256=evidence['capture_SHA256'],source_proof_SHA256=sha(proof.read_bytes()),capture_SHA256=sha((out/'observation.pcap').read_bytes()),records=len(ledger),ledger=ledger,transport_bytes_or_headers_modified=False,declared_changes=['original-record filtering and frame renumbering','single source Ethernet interface mapped to PCAP interface zero'])
    (out/'manifest.json').write_text(json.dumps(manifest,indent=2)+'\n',encoding='utf8')
    print(json.dumps(dict(original_wire_witnesses=len(ledger),capture_SHA256=manifest['capture_SHA256'])),flush=True)


if __name__=='__main__':run(*map(Path,sys.argv[1:4]))
