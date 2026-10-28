"""Check actual isolated inspector responses against their captured source bytes.

Native header metadata remains generated (no fabricated byte range); captured
standard/GBP headers and stateless YAML headers must point to source zero.
This does not claim GPE payload support or policy execution.
"""
import base64,copy,hashlib,json
from pathlib import Path
import vxlan_parent_source_audit as a


def inspect(record,request,response):
    wire=record['wire'];assert base64.b64decode(request['Data'])==wire and a.integer(request['Length'])==len(wire)and a.integer(request['Link'])==record['link']and a.integer(request['Number'])==a.integer(response['Number'])==1
    parsed=a.scan(record);headers=parsed['headers'];leaf=parsed['leaf'];fields=response['Fields'];assert fields and fields[0]['Text'].startswith('Frame 1:')and fields[0]['Source']==0 and fields[0]['Start']==0 and fields[0]['End']==len(wire)and fields[0]['HasRange']is True
    for f in fields:
        assert type(f['HasRange'])is bool and type(f['Source'])is int and type(f['Start'])is int and type(f['End'])is int
        if f['HasRange']:
            source=f['Source'];assert source>=0 and source<=len(response.get('Sources')or[]);size=len(wire)if source==0 else len(base64.b64decode(response['Sources'][source-1]['Data']));assert 0<=f['Start']<f['End']<=size
    native=bool(request.get('Events'));texts=[f['Text']for f in fields]
    def exact(label,start=None,end=None,generated=False):
        found=[f for f in fields if f['Text']==label];assert len(found)==1,('unique literal field required',label);f=found[0]
        if generated:assert not f['HasRange'],'event metadata must not invent captured offsets'
        else:assert f['HasRange']is True and f['Source']==0 and f['Start']==start and f['End']==end,('captured field must point to its exact source-zero bytes',label)
        return f
    if leaf['kind']=='refused-VXLAN':
        assert response['Protocol']=='IPv4';assert not any('DNS'in t or'example.com'in t or'VXLAN.Inner'in t for t in texts)
        assert any('I bit is clear'in t or'incomplete fixed header'in t or'Ethernet child prefix incomplete'in t for t in texts)
        if not native:
            for h in headers[:-1]:
                assert h['selected'];exact('Admitted outer VNI: '+str(h['VNI']),h['offset']+4,h['offset']+7)
            bad=headers[-1];raw=bytes.fromhex(bad['header'])
            if raw:exact(f'Observed flags: 0x{raw[0]:02x}; I: {str(bool(raw[0]&8)).lower()}',bad['offset'],bad['offset']+1)
            if len(raw)<7:assert not any('VNI:'in t and'outer'not in t for t in texts)
        scope='precise parent refusal; no child identity or fields'
    elif leaf['kind']=='unselected-GPE-payload':
        assert response['Protocol']==('VXLAN-GPE'if native else'UDP');assert not any('DNS'in t or'example.com'in t or'.Inner'in t for t in texts);at=leaf['offset'];h=bytes.fromhex(leaf['header'])
        if native:
            exact('VXLAN-GPE VNI: '+str(leaf['VNI']),generated=True);exact('Payload Profile Implemented: false',generated=True)
        else:
            for label,lo,hi in[(f'VXLANGPEHeader.Version: {(h[0]>>4)&3}',0,1),('VXLANGPEHeader.I: 1',0,1),(f'VXLANGPEHeader.P: {int(bool(h[0]&4))}',0,1),(f'VXLANGPEHeader.Next Protocol: {h[3]}',3,4),(f'VXLANGPEHeader.VNI: {leaf["VNI"]}',4,7)]:exact(label,at+lo,at+hi)
            tail=[f for f in fields if f['Text'].startswith('Remaining Payload:')];assert len(tail)==1 and tail[0]['Source']==0 and tail[0]['HasRange']is True and(tail[0]['Start'],tail[0]['End'])==(at+8,len(wire))
        scope='verified GPE header only; payload remains unselected'
    elif headers:
        assert len(headers)==1 and headers[0]['selected']and leaf['kind']=='udp'and leaf['destination_port']==53 and response['Protocol']=='DNS';h=headers[0];at=h['offset'];raw=bytes.fromhex(h['header'])
        exact('Captured VXLAN header 1',at,at+8);assert fields[1]['Text']=='Captured VXLAN header 1';exact('I: true',at,at+1);exact('VNI: '+str(h['VNI']),at+4,at+7);exact('VXLAN.G: '+str(int(h['GBP_G'])),at,at+1);exact('VXLAN.I: 1',at,at+1);exact('VXLAN.VNI: '+str(h['VNI']),at+4,at+7)
        if h['GBP_G']:
            for label,lo,hi in[("GBP Don't Learn: "+str(h['GBP_D']).lower(),1,2),('GBP Applied: '+str(h['GBP_A']).lower(),1,2),('GBP Policy ID: '+str(h['GBP_ID']),2,4),('VXLAN.GBP.D: '+str(int(h['GBP_D'])),1,2),('VXLAN.GBP.A: '+str(int(h['GBP_A'])),1,2),('VXLAN.GBP.Group Policy ID: '+str(h['GBP_ID']),2,4)]:exact(label,at+lo,at+hi)
        else:assert not any(t.startswith('GBP ')or t.startswith('VXLAN.GBP.')for t in texts)
        assert not any(t.startswith('VXLANGPEHeader.')for t in texts),'receive-ignored standard-port bits do not select GPE'
        scope='admitted standard VXLAN/conditional GBP; real captured source-zero ranges'
    else:
        assert leaf['kind']=='udp'and leaf['source_port']==4789 and leaf['destination_port']not in[4789,4790]and response['Protocol']=='UDP';assert not any('Captured VXLAN header'in t or t.startswith('VXLAN.')or t=='DNS'for t in texts);scope='source-only service hint; no tunnel or child identity'
    return dict(scope=scope,source_wire_SHA256=hashlib.sha256(wire).hexdigest(),request_has_native_events=native,protocol=response['Protocol'],captured_fields_with_valid_ranges=sum(f['HasRange']and f['Source']==0 for f in fields),all_ranges_source_bound=True)


def negative_controls(cases):
    checks=[]
    def run(name,case,mutation):
        record,request,response=copy.deepcopy(cases[case]);inspect(record,request,response);mutation(request,response)
        try:inspect(record,request,response)
        except(AssertionError,KeyError):checks.append(dict(case=name,rejected=True));return
        raise AssertionError('negative accepted: '+name)
    def field(response,label):return next(v for v in response['Fields']if v['Text']==label)
    run('captured VNI range shifted to another plausible byte',('GBP-current-A',False),lambda q,r:field(r,'VNI: 42').__setitem__('Start',47))
    run('captured VNI source changed to a virtual logical payload',('GBP-current-A',False),lambda q,r:field(r,'VNI: 42').__setitem__('Source',1))
    run('actual GBP A bit 0x08 mislabeled as false',('GBP-current-A',False),lambda q,r:field(r,'GBP Applied: true').__setitem__('Text','GBP Applied: false'))
    run('reserved high bit 0x80 mislabeled as GBP A',('GBP-reserved-high-bit-not-A',False),lambda q,r:field(r,'GBP Applied: false').__setitem__('Text','GBP Applied: true'))
    run('invalid-I stateless worker revives DNS identity',('I-missing',True),lambda q,r:r.__setitem__('Protocol','DNS'))
    run('invalid-I inspector retains an apparent child DNS tree',('I-missing',True),lambda q,r:r['Fields'].append(dict(Text='DNS',Depth=0,Branch=True,HasRange=True,Source=0,Start=92,End=121)))
    run('source-only port creates a captured VXLAN header',('source-port-only',True),lambda q,r:r['Fields'].append(dict(Text='Captured VXLAN header 1',Depth=0,Branch=True,HasRange=True,Source=0,Start=42,End=50)))
    run('GPE payload gains a byte-backed but unselected DNS subtree',('GPE-Ethernet',True),lambda q,r:r['Fields'].append(dict(Text='DNS',Depth=0,Branch=True,HasRange=True,Source=0,Start=92,End=121)))
    run('generated native GPE metadata invents an exact frame range',('GPE-Ethernet',False),lambda q,r:field(r,'VXLAN-GPE VNI: 42').update(HasRange=True,Source=0,Start=46,End=49))
    run('nested parent uses inner invalid VNI as its admitted outer VNI',('nested-I-missing',True),lambda q,r:field(r,'Admitted outer VNI: 77').__setitem__('Text','Admitted outer VNI: 42'))
    run('worker request bytes changed without changing capture source',('I-missing',True),lambda q,r:q.__setitem__('Data',base64.b64encode(b'\0'+base64.b64decode(q['Data'])[1:]).decode()))
    return dict(auditor_SHA256=a.sha(Path(__file__)),negative_controls=len(checks),all_rejected=True,checks=checks)
