"""Derive explicit duplicate-opening and delayed-old-close capture observations.

All packet bytes, capture timestamps and record headers come unchanged from the
actual four-connection Windows capture. Copies represent duplicate observations,
not a claim that a network retransmission was injected or accepted by a peer.
No capture, receipt, key material or service output may be written inside Git.
"""
import argparse
import hashlib
import json
import struct
from pathlib import Path


def digest(value):return hashlib.sha256(value).hexdigest()


def classic_records(path):
    wire=path.read_bytes()
    assert wire[:4]in[b'\xd4\xc3\xb2\xa1',b'\xa1\xb2\xc3\xd4',b'\x4d\x3c\xb2\xa1',b'\xa1\xb2\x3c\x4d']
    endian='<'if wire[:4]in[b'\xd4\xc3\xb2\xa1',b'\x4d\x3c\xb2\xa1']else'>'
    at=24;records=[]
    while at<len(wire):
        assert at+16<=len(wire)
        _,_,captured,original=struct.unpack_from(endian+'IIII',wire,at)
        assert captured<=original and at+16+captured<=len(wire)
        records.append(wire[at:at+16+captured]);at+=16+captured
    assert at==len(wire)
    return wire[:24],records


def derive(capture,certificate,output):
    generator_digest=digest(Path(__file__).read_bytes())
    source=capture.read_bytes();proof=json.loads(certificate.read_text(encoding='utf8'))
    assert proof['capture_sha256']==digest(source)
    epochs=proof['original_TCP_epochs'];assert len(epochs)==4
    header,records=classic_records(capture)
    assert struct.unpack_from('<I',header,20)[0]==0,'bounded original Windows loopback profile'
    originals={i+1:block for i,block in enumerate(records)}
    syns={state['SYN_frame']for state in epochs};first_new_syn=epochs[1]['SYN_frame']
    resets=set()
    for number,block in originals.items():
        wire=block[16:]
        if len(wire)<44 or wire[:4]not in[b'\x02\0\0\0',b'\0\0\0\x02']:continue
        if wire[4]>>4!=4 or wire[13]!=6:continue
        at=4+4*(wire[4]&15)
        if wire[at+13]&4:resets.add(number)
    old_reset=next(n for n in epochs[0]['frames']if n in resets)
    assert len(resets)==4 and old_reset<first_new_syn
    output.mkdir(parents=True,exist_ok=False)
    for name,duplicates,delayed,retry in[
        ('actual',False,False,False),
        ('missing-close-opening-duplicates',True,False,False),
        ('late-old-close',False,True,False),
        ('late-old-close-with-duplicates',True,True,False),
        ('late-old-close-opening-retry',True,True,True),
    ]:
        selected=[]
        if name=='actual':selected=list(originals)
        else:
            for number in originals:
                if number in resets:continue
                selected.append(number)
                if duplicates and number in syns:selected.extend([number,number])
                if delayed and number==first_new_syn:
                    selected.append(old_reset)
                    if retry:selected.append(number)
        target=output/name;target.mkdir()
        result=header+b''.join(originals[number]for number in selected)
        (target/'observation.pcap').write_bytes(result)
        manifest=dict(scope=__doc__,generator_SHA256=generator_digest,source_capture_SHA256=digest(source),source_certificate_SHA256=digest(certificate.read_bytes()),capture_SHA256=digest(result),physical_frames=len(selected),original_frame_by_observation_frame={i+1:n for i,n in enumerate(selected)},removed_source_frames=sorted(set(originals)-set(selected)),duplicated_source_frames=sorted({n for n in selected if selected.count(n)>1}),delayed_old_reset_source_frame=old_reset if delayed else None,wire_or_timestamp_rewrite=False,network_retransmission_or_loss_injection=False)
        (target/'manifest.json').write_text(json.dumps(manifest,indent=2)+'\n',encoding='utf8')
        print(json.dumps(dict(case=name,frames=len(selected),capture_SHA256=manifest['capture_SHA256'])),flush=True)


if __name__=='__main__':
    ap=argparse.ArgumentParser(description=__doc__)
    for name in['capture','certificate','output']:ap.add_argument('--'+name,type=Path,required=True)
    args=ap.parse_args()
    assert not any((p/'.git').exists()for p in[args.output.resolve(),*args.output.resolve().parents])
    derive(args.capture,args.certificate,args.output)
