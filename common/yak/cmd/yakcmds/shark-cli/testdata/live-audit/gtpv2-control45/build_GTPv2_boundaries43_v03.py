"""Source-only small GTPv2 boundary plan; authored controls, never live traffic.
Values/spans come only from literal input octets and pinned 3GPP framing rules.
No native/CLI/candidate implementation or output is imported or executed.
"""
import argparse,collections,hashlib,json,shutil,struct
from pathlib import Path

def sha(p):return hashlib.sha256(Path(p).read_bytes()).hexdigest()
def dg(w):return hashlib.sha256(w).hexdigest()
def put(p,d):
 with Path(p).open('x',encoding='utf8')as f:json.dump(d,f,indent=2);f.write('\n')
def checksum(w):
 if len(w)%2:w+=b'\0'
 s=sum(int.from_bytes(w[i:i+2],'big')for i in range(0,len(w),2))
 while s>>16:s=(s&65535)+(s>>16)
 return (~s)&65535

def fields(w):
 """Full source observations independent of any future collection resource cap."""
 leaves=[];messages=[];issues=[];total_ies=0;max_depth=0
 def scalar(path,at,size,limit,mask=None,shift=0):
  if at+size>min(limit,len(w)):return None
  v=int.from_bytes(w[at:at+size],'big');v=(v&mask)>>shift if mask is not None else v
  leaves.append({'path':path,'value':v,'span':[at,at+size],'mask':mask,'shift':shift});return v
 def opaque(path,start,declared_end,limit):
  end=min(declared_end,limit,len(w));end=max(start,end)
  leaves.append({'path':path,'owned_bytes_SHA256':dg(w[start:end]),'span':[start,end],'captured_bytes':end-start,'declared_bytes':declared_end-start})
 def ies(start,end,parent,depth):
  nonlocal total_ies,max_depth
  at=start;index=0
  while at<end:
   total_ies+=1;max_depth=max(max_depth,depth);assert total_ies<=4096 and depth<=32
   path=parent+[index];typ=scalar(path+['Type'],at,1,end);n=scalar(path+['Length'],at+1,2,end);instance=scalar(path+['Instance'],at+3,1,end,15);scalar(path+['Spare'],at+3,1,end,240,4)
   if n is None or instance is None:issues.append({'kind':'short-IE-header','at':at,'available':end-at});return
   finish=at+4+n
   if finish>end:
    opaque(path+['Captured Value'],at+4,finish,end);issues.append({'kind':'IE-declared-extent-overrun','at':at,'declared_end':finish,'available_end':end});return
   if typ==93:
    if n:ies(at+4,finish,path+['Grouped IEs'],depth+1)
   elif typ==3 and n==1:scalar(path+['Recovery Counter'],at+4,1,finish)
   elif typ==73 and n==1:
    scalar(path+['EPS Bearer ID'],at+4,1,finish,15);scalar(path+['Value Spare'],at+4,1,finish,240,4)
   elif typ==2 and n in(2,6):
    scalar(path+['Cause'],at+4,1,finish);scalar(path+['Cause Flags'],at+5,1,finish)
    if n==6:
     scalar(path+['Offending IE Type'],at+6,1,finish);scalar(path+['Offending IE Length'],at+7,2,finish);scalar(path+['Offending IE Instance'],at+9,1,finish,15)
   elif typ==87 and n>=5:
    flags=scalar(path+['F-TEID Flags'],at+4,1,finish);scalar(path+['Interface Type'],at+4,1,finish,63);scalar(path+['TEID'],at+5,4,finish);pos=at+9
    expected=5+4*bool(flags&128)+16*bool(flags&64)
    if n!=expected:issues.append({'kind':'F-TEID-value-length-contradiction','at':at,'expected':expected,'declared':n})
    if flags&128:scalar(path+['IPv4 Address Bits'],pos,4,finish);pos+=4
    if flags&64:scalar(path+['IPv6 Address Bits'],pos,16,finish)
   else:opaque(path+['Opaque Value'],at+4,finish,finish)
   at=finish;index+=1
 at=0;index=0
 if not w:issues.append({'kind':'no-complete-first-octet','at':0})
 while at<len(w):
  path=['Messages',index];flags=scalar(path+['Flags'],at,1,len(w));version=scalar(path+['Version'],at,1,len(w),224,5)
  if version!=2:issues.append({'kind':'not-GTPv2-version','at':at,'value':version});break
  p=scalar(path+['P'],at,1,len(w),16,4);t=scalar(path+['T'],at,1,len(w),8,3);mp=scalar(path+['MP Bit Observed'],at,1,len(w),4,2);scalar(path+['Header Spare Bits'],at,1,len(w),3);typ=scalar(path+['Message Type'],at+1,1,len(w));n=scalar(path+['Message Length'],at+2,2,len(w))
  if n is None:issues.append({'kind':'short-common-header','at':at});break
  end=at+4+n;minimum=12 if t else 8;limit=min(end,len(w));msg={'index':index,'start':at,'declared_end':end,'captured_end':limit,'header_bytes_required':minimum,'type_octet':typ,'T':t,'P':p};messages.append(msg)
  if t:scalar(path+['TEID'],at+4,4,limit)
  seq=at+(8 if t else 4);scalar(path+['Sequence Number'],seq,3,limit);last=scalar(path+['Priority or Spare Octet'],seq+3,1,limit)
  if t and mp:scalar(path+['Message Priority'],seq+3,1,limit,240,4)
  if last is not None:scalar(path+['Last Octet Spare'],seq+3,1,limit,15 if t and mp else 255)
  if end<at+minimum:issues.append({'kind':'message-length-under-header','at':at,'minimum':minimum,'declared_end':end});break
  if limit<at+minimum:issues.append({'kind':'short-conditional-header','at':at,'required_end':at+minimum,'captured_end':limit});break
  if index==1 and p:issues.append({'kind':'second-piggyback-P-must-be-zero','at':at})
  if end>len(w):issues.append({'kind':'message-declared-extent-overrun','at':at,'declared_end':end,'captured_end':len(w)})
  ies(at+minimum,limit,path+['IEs'],0)
  if end>len(w):break
  at=end;index+=1
  if not p:
   if at<len(w):issues.append({'kind':'unindicated-trailing-bytes','at':at,'bytes':len(w)-at})
   break
  if index==2:
   if at<len(w):issues.append({'kind':'third-message-not-permitted','at':at,'bytes':len(w)-at})
   break
  if at==len(w):issues.append({'kind':'P-without-following-message','at':at})
 return {'source_leaves':leaves,'source_message_headers':messages,'source_issues':issues,'source_observed_IE_nodes':total_ies,'source_max_group_depth':max_depth,'all_values_from_input_octets':True,'message_specific_mandatory_IE_or_bearer_semantics_validated':False}

def ie(t,value=b'',instance=0,spare=0):return bytes([t])+len(value).to_bytes(2,'big')+bytes([(spare<<4)|instance])+value
def message(t,body=b'',teid=None,seq=1,p=0,mp=0,last=0,spare=0):
 h=bytes([0x40|(p<<4)|(8 if teid is not None else 0)|(mp<<2)|spare,t]);rest=(teid.to_bytes(4,'big')if teid is not None else b'')+seq.to_bytes(3,'big')+bytes([last])+body
 return h+len(rest).to_bytes(2,'big')+rest

def envelope(w,src=1,dst=2,original_payload=None):
 # Documentation-only RFC5737 peer addresses, fixed local MACs; authored traffic.
 original=w if original_payload is None else original_payload
 source=bytes([192,0,2,src]);destination=bytes([192,0,2,dst]);length=8+len(original);uh=struct.pack('!HHHH',2123,2123,length,0);us=checksum(source+destination+b'\0\x11'+length.to_bytes(2,'big')+uh+original)or 65535;uh=uh[:6]+us.to_bytes(2,'big');ip=bytes.fromhex('4500')+(20+length).to_bytes(2,'big')+b'\0\1\0\0\x40\x11\0\0'+source+destination;ip=ip[:10]+checksum(ip).to_bytes(2,'big')+ip[12:];eth=bytes.fromhex('0200000000020200000000010800');return eth+ip+uh+w,len(eth+ip+uh+original)

def main():
 ap=argparse.ArgumentParser();ap.add_argument('--phase',type=Path,required=True);ap.add_argument('--out',type=Path,required=True);a=ap.parse_args();assert not a.out.exists();assert shutil.disk_usage('C:/').free>20*2**30;a.out.mkdir();a.out.joinpath('pdu').mkdir();official=a.phase/'GTP-official-source43-02/official-source-facts.json';known=json.loads(official.read_text('utf8'))[0];cap=a.phase/'GTP-official-download43-01/project-upload.pcap';rawcap=cap.read_bytes();assert sha(cap)=='b62192ea1eba8e3bce2bff600222ce1ac41caf01867f0336a026f161da355030';caplen=int.from_bytes(rawcap[32:36],'little');frame=rawcap[40:40+caplen];start,end=known['payload_physical_span'];real=frame[start:end];assert dg(real)==known['payload_SHA256'];echo=message(1,ie(3,b'\x07'));pair1=message(33,ie(2,b'\x10\0'),teid=0x01020304,seq=5,p=1);pair2=message(95,ie(73,b'\x05'),teid=0x01020304,seq=6);mp=bytearray(real);mp[0]|=4;mp[11]=0xf0;sp=bytearray(real);sp[0]|=3;sp[11]=0xa5
 specs=[]
 def add(name,w,expect,params=None,notes='',peers=None,original=None):specs.append({'name':name,'wires':w if isinstance(w,list)else[w],'expectation':expect,'parameters':params or {'IE_budget':4096,'group_depth_budget':8},'notes':notes,'peers':peers,'original_payload':original})
 add('echo-T0-recovery',echo,'bounded-wire-complete',notes='3GPP7.1.1 Recovery IE; no TEID fabricated')
 add('official-T1-TEID-zero',real,'bounded-wire-complete',notes='original official frame1 PDU unchanged; TEID0 is observed, no session association')
 add('MP1-priority15',bytes(mp),'bounded-wire-complete',notes='official frame1 PDU changes only MPflag and priority octet; priority15 lowest, no traffic class or session proof')
 add('receive-nonzero-spares-MP0',bytes(sp),'bounded-wire-complete',notes='receiver ignores first low2bits and whole last octet when MP0; do not fabricate priority10')
 add('two-message-piggyback',pair1+pair2,'bounded-wire-complete',notes='header grammar pair33/95 only; does not claim required full procedure IEs/session context')
 add('second-P1-third-message',pair1+bytes([pair2[0]|16])+pair2[1:]+echo,'format',notes='secondP1 and third message prohibited by5.5.1; no third message delivery')
 add('P1-following-missing',pair1,'format',notes='captured parent complete; no second header may be invented')
 add('P0-unindicated-trailer',bytes([pair1[0]&~16])+pair1[1:]+ie(250,b'X'),'format',notes='extra TLIV is outside first declared PDU; cannot extend first message body')
 add('other-version1',bytes.fromhex('3001000401020304'),'not-GTPv2',notes='separate version family; no v2-specific fields/identity beyond raw common observations')
 add('zero-byte-header',b'','incomplete',notes='no first complete scalar, no zero default fields')
 add('one-byte-flags',b'\x48','incomplete',notes='only flags-derived bits are physically complete; no type/length/TEID/sequence defaults')
 add('three-byte-common-prefix',b'\x48\x20\x00','incomplete',notes='length high octet alone is not complete uint16')
 add('T1-sequence-two-byte-prefix',real[:10],'incomplete',notes='complete TEID exists; sequence needs all3bytes, no partial uint24 fabricated')
 add('capture-short-Recovery-body',echo[:-1],'incomplete',notes='true record caplen<original; IEheader observed but no Recovery Counter value',original=echo)
 add('IE-extent-overrun-complete-parent',message(1,b'\x5d\x00\x09\x00X'),'format',notes='complete UDP/GTP parent, IE declares9 but captures1; not packet snaplen or safe group-child boundary')
 grouped=ie(93,ie(73,b'\x05')+ie(250,b'ABC',instance=2))+ie(3,b'\x09')
 add('group93-unknown-child-then-known',message(32,grouped,teid=0x01020304),'bounded-wire-complete',notes='unknown/spare250 body opaque; continue by exact IE length to later Recovery; fullmessage semantics not asserted')
 add('IE-count-budget1',message(1,ie(3,b'\x01')+ie(3,b'\x02')),'budget',params={'IE_budget':1,'group_depth_budget':8},notes='2 observed rows require2tokens; no verified absence of secondcounter; duplicate recovery grammar not asserted')
 nested=ie(73,b'\x05')
 for _ in range(10):nested=ie(93,nested)
 add('group-depth-budget8',message(32,nested,teid=0x01020304),'budget',notes='adversarial grouped framing, not valid bearer semantic nesting; bounded depth8 cannot claim completeness')
 order=[]
 for seq in(10,9,10,11):
  q=bytearray(real);q[8:11]=seq.to_bytes(3,'big');order.append(bytes(q))
 add('reorder-and-duplicate-independent-wire',order,'independent-wire-no-inheritance',notes='arrival10,9,duplicate10,11; every original wire remains independently observable; no reassembly, request matching or tunnel inheritance claimed')
 add('same-TEID-distinct-peers', [message(32,ie(250,b'A'),teid=0x01020304,seq=1),message(32,ie(250,b'B'),teid=0x01020304,seq=1)],'independent-wire-no-inheritance',notes='sameTEID/sequence across distinct peer tuples; opaque A cannot be copied into B; not complete mandatory-IE validation',peers=[(1,2),(3,4)])
 assert len(specs)==20;facts=[];records=[];inputs=[];physical=0
 for ci,s in enumerate(specs,1):
  rows=[]
  for wi,w in enumerate(s['wires'],1):
   q=a.out/'pdu'/('%02d-%02d.bin'%(ci,wi));q.write_bytes(w);f=fields(w);f.update(sourcecase=s['name'],wire_index=wi,raw_file=str(q),raw_SHA256=dg(w),captured_PDU_bytes=len(w),scenario_expectation=s['expectation'],resource_parameters=s['parameters'],notes=s['notes']);physical+=1;peer=s['peers'][wi-1]if s['peers']else(1,2);wire,original_len=envelope(w,*peer,original_payload=s['original_payload']);records.append((wire,original_len));f['authored_capture_frame']=physical;f['authored_domain']={'section':0,'interface':0,'link':1};f['outer_source_peer']='192.0.2.%d'%peer[0];f['outer_destination_peer']='192.0.2.%d'%peer[1];f['captured_frame_SHA256']=dg(wire);rows.append(f);inputs.append({'sourcecase':s['name'],'wire_index':wi,'raw_path':str(q),'raw_SHA256':dg(w),'captured_PDU_bytes':len(w),'IE_budget':s['parameters']['IE_budget'],'group_depth_budget':s['parameters']['group_depth_budget']})
  facts.append({'case':ci,'sourcecase':s['name'],'expectation':s['expectation'],'notes':s['notes'],'wires':rows})
 pcap=a.out/'authored-controls.pcap'
 with pcap.open('xb')as f:
  f.write(struct.pack('<IHHIIII',0xa1b2c3d4,2,4,0,0,65535,1))
  for n,(w,orig)in enumerate(records):f.write(struct.pack('<IIII',1700000000,n*1000,len(w),orig)+w)
 put(a.out/'source-grid.json',facts);put(a.out/'probe-inputs.json',inputs)
 norms={'standard_URL':'https://www.etsi.org/deliver/etsi_ts/129200_129299/129274/18.06.00_60/ts_129274v180600p.pdf','standard_SHA256':'c9eefb4832b1549b7fdb9d3abdf9bf7afe4f7da611942ae6e3149e0a807d3be2','scope':'GTPv2-C bounded wire observations; not complete message-specific mandatory/conditional IE grammar, authentication, NAS, bearer establishment or session state','rules':[{'clause':'5.1/5.3/5.4/5.5.1','rule':'Version2; conditional TEID; T0 header8, T1 header12; length excludesfirst4, includesTEID/sequence/spare; T0 path-management1/2/3 omitTEID; ordinaryEPC T1 mayhaveTEID0 per5.5.2'},{'clause':'5.5.1/4.2.0','rule':'P1 exactlyoneadditionalmessage, secondP0; eachownlength+sequence; first33->95 or96->34EPC procedure pairing separately requires context; no third/trailing information'},{'clause':'5.5.1','rule':'EPC MPflag0x04; MP1 lastoctethigh4=priority0highest15lowest; firstoctetlow2,lastoctetlow4 andMP0lastoctethigh4 ignoredbyreceiver; preserve rawspare but do notreject or fabricate inactivepriority'},{'clause':'8.2.1/6.1.2/6.1.3','rule':'IEtype1/length2/spare4bits+instance4bits, unaligned4byteheader, length excludes4; groupboundaryexact and includes fullnestedIEheaders; scope+type+instance identifiesrow, duplicates preservedas observed lists'},{'clause':'8.1/8.2.1A/7.7.9','rule':'future/spareIEs ownedopaque and exactlengthskip; IE254 has2byte typeextension, not NAS; IEhighnibble is spare in pinnedEPCformat; do not infer criticality from WS historicalCRlabel'},{'clause':'7.7.1/7.7.3/7.7.7','rule':'completeparent internalextentcontradiction distinct capture-short; preserve only complete bounded scalarprefix, no default fields, no scanresync acrossbadlength'},{'clause':'7.6/7.7.10','rule':'sourcearrivalorder and duplicates only; no request/response correlation nor bearer/session owner from TEID alone; raw source slot never inherited'}],'resource_policy':'direct controls request globalIErow budget1 and groupdepth8; source facts record all independently observable wire fields and required count/depth. Future result must indicate budget and incomplete coverage, not false absent values; exact retained cutoff needs producer API contract before whole-field oracle.'}
 put(a.out/'normative-contract.json',norms)
 frozen={str(p):sha(p)for p in [Path(__file__),official,cap,a.out/'source-grid.json',a.out/'probe-inputs.json',a.out/'normative-contract.json',pcap]};summary={'source_inputs':frozen,'source_only_before_candidate':True,'scenarios':len(specs),'physical_authored_records':len(records),'real_official_derived_PDU_control_cases':5,'other_scenarios_are_authored_not_live':True,'source_leaves':sum(len(w['source_leaves'])for c in facts for w in c['wires']),'authored_capture_bytes':pcap.stat().st_size,'wire_payload_bytes':sum(len(w)for s in specs for w in s['wires']),'expectation_counts':dict(collections.Counter(s['expectation']for s in specs)),'no_native_CLI_Go_repo_write':True,'no_bearer_authentication_or_session_claim':True};put(a.out/'summary.json',summary);print(json.dumps({'summary':str(a.out/'summary.json'),'SHA256':sha(a.out/'summary.json'),'source_grid_SHA256':sha(a.out/'source-grid.json'),'scenarios':20,'physical':len(records),'source_leaves':summary['source_leaves']}))
if __name__=='__main__':main()
