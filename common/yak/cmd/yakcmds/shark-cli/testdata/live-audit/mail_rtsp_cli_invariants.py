"""Certify Info-only IMAP/RTSP presentation changes in every ordered CLI row.

Bind each changed row to an exact SDK raw SHA, complete capture references and
endpoints/domain; CLI and SDK numeric PDU/connection namespaces are independent.
All metadata, labels, types, other columns and row order must stay literal.
The SDK has its separate capture/field audit; this is a presentation certificate.
"""
import argparse
import base64
import collections
import copy
import hashlib
import itertools
import json
from pathlib import Path

from mail_rtsp_export_invariants import canonical, rows, sha


def source_key(protocol, source, destination, domain, evidence):
    return canonical([protocol,source,destination,domain,
                      evidence.get('Kind'),evidence.get('SHA256'),
                      evidence.get('PacketRefs'),evidence.get('CompletedIn')])


def audit(before,after,sdk):
    native=collections.defaultdict(list)
    for e in rows(sdk):
        if e['protocol'] not in ['imap','rtsp'] or not e.get('raw'):continue
        wire=base64.b64decode(e['raw'],validate=True)
        assert hashlib.sha256(wire).hexdigest()==e['source_bytes']['SHA256']
        native[source_key(e['protocol'],e['source'],e['destination'],e['domain'],e['source_bytes'])].append(e)
    count=0;changed=[];protocols=collections.Counter()
    for old,new in itertools.zip_longest(rows(before),rows(after)):
        assert old is not None and new is not None,'all ordered physical rows present'
        count+=1;protocols[new['protocol']]+=1
        if canonical(old)==canonical(new):continue
        expected=copy.deepcopy(old);expected['info']=new['info']
        assert canonical(expected)==canonical(new),('only Info changes are authorized',new['number'])
        assert new['protocol'] in ['IMAP','RTSP'],'unrelated protocol summary cannot change'
        evidence=[]
        for pdu in new.get('pdus')or[]:
            if pdu['protocol']not in ['imap','rtsp']or pdu['status']not in ['decoded','deferred']:continue
            key=source_key(pdu['protocol'],new['message_source'],new['message_destination'],new['message_domain'],pdu['byte_source'])
            matches=native[key]
            assert len(matches)==1,'unique SDK message bound by bytes, contributors, endpoints and domain'
            e=matches[0];wire=base64.b64decode(e['raw'],validate=True);s=e.get('session')or{}
            assert e['status']=='decoded' and not e.get('error'),'accepted summary requires an accepted native PDU'
            assert new['number']in[r['Number']for r in e['source_bytes']['PacketRefs']],'row is an actual contributor'
            required=[]
            if e['protocol']=='rtsp':
                header=wire.split(b'\r\n\r\n',1)[0];lines=header.split(b'\r\n');parts=lines[0].split(b' ',2)
                cseq=[v.strip()for name,sep,v in (line.partition(b':')for line in lines[1:])if sep and name.lower()==b'cseq']
                if not cseq:continue
                assert len(cseq)==1
                method=parts[1].decode()if parts[0]==b'RTSP/1.0'else parts[0].decode()
                required=['RTSP '+method,'CSeq='+str(int(cseq[0]))]
                if method=='FLUSH':
                    rtp=[v.strip()for name,sep,v in (line.partition(b':')for line in lines[1:])if sep and name.lower()==b'rtp-info']
                    for value in rtp:
                        for token in value.split(b';'):
                            name,sep,value=token.strip().partition(b'=')
                            if sep and name in [b'seq',b'rtptime']:required.append(name.decode()+'='+str(int(value)))
            else:
                for key,label in [('SASL Stage','IMAP SASL'),('Namespace Stage','IMAP NAMESPACE'),('IDLE Stage','IMAP IDLE')]:
                    if key not in s:continue
                    required=[label,s[key],'tag='+s['Tag']]
                    if key=='SASL Stage':required.append(s['Mechanism'])
                    break
            if not required:continue
            assert all(token in new['info']for token in required),('specific observed control summary',new['number'],required)
            evidence.append(dict(CLI_PDU=pdu['id'],SDK_PDU=e['id'],protocol=e['protocol'],raw_sha256=e['source_bytes']['SHA256'],physical_frames=[r['Number']for r in e['source_bytes']['PacketRefs']]))
        assert evidence,'each changed summary has source-bound accepted control evidence'
        changed.append(dict(frame=new['number'],protocol=new['protocol'],before=old['info'],after=new['info'],evidence=evidence))
    assert count and changed,'nonempty full export and actual summary changes'
    return dict(scope=__doc__,before_sha256=sha(before),after_sha256=sha(after),SDK_sha256=sha(sdk),physical_rows_compared=count,
                protocol_label_counts=dict(sorted(protocols.items())),changed_rows=changed,changed_rows_count=len(changed),
                changes_by_protocol=dict(collections.Counter(e['protocol']for e in changed)),all_other_values_types_labels_order_and_IDs_exact=True,numeric_namespace_equality_assumed=False)


if __name__=='__main__':
    ap=argparse.ArgumentParser(description=__doc__)
    for name in ['before','after','sdk','out']:ap.add_argument('--'+name,type=Path,required=True)
    a=ap.parse_args()
    for p in vars(a).values():assert not any((q/'.git').exists()for q in[p.resolve(),*p.resolve().parents])
    proof=audit(a.before,a.after,a.sdk)
    a.out.write_text(json.dumps(proof,indent=2)+'\n',encoding='utf8')
    print(json.dumps({k:proof[k]for k in ['physical_rows_compared','changed_rows_count','changes_by_protocol','all_other_values_types_labels_order_and_IDs_exact']}))
