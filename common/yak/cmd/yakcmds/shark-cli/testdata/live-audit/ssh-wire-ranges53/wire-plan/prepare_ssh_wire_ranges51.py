"""Independent SSH range plan from frozen source49/50, not a runtime producer.
No candidate SDK output is read. Proposed metadata schema is not a product claim.
"""
from pathlib import Path
import hashlib,json,base64,copy,collections,importlib.util
P=Path(r'C:/tmp/yak-shark-ESP-SSH-UI-20261028')
D=Path(__file__).resolve().parent
R=Path(r'C:/Users/V/.codex/worktrees/shark-protocol-infrastructure/yaklang')
INPUTS=[('source49',P/'SSH-SDK-CLI-source49-01/plan01/source-rows.json','d9421805893b738fa47d95b6baa177cc907df37475e2dae30c90dd45547dcf73'),('source50',P/'SSH-role-API-CLI-source50-03/plan03/source-rows.json','fbbcda79bd880324bfc541649f271580b626a764e88af14cd554e39eee5eac77')]
BASE=P/'SSH-SDK-CLI-source49-01/ssh_source_API49.py'
assert hashlib.sha256(BASE.read_bytes()).hexdigest()=='061ba6afdb24cd78c07302b89c406d3d507889c9f78e851258d1418de5a3ff68'
spec=importlib.util.spec_from_file_location('frozen_ssh_range_source49',BASE);source=importlib.util.module_from_spec(spec);spec.loader.exec_module(source)
def sha(p):return hashlib.sha256(Path(p).read_bytes()).hexdigest()
def digest(w):return hashlib.sha256(w).hexdigest()
def demand(v,msg):
    if not v:raise AssertionError(msg)
def put(p,x):p.write_text(json.dumps(x,ensure_ascii=False,sort_keys=True,indent=2)+'\n',encoding='utf-8')
def decode(leaf,w):
    lo,hi=leaf['span'];demand(0<=lo<=hi<=len(w),'span within owned PDU')
    b=w[lo:hi]
    if leaf['kind']=='unsigned':return int.from_bytes(b,'big')
    if leaf['kind']=='string':return b.decode('ascii')
    return base64.b64encode(b).decode()
def key(path):return '/'.join(path)
def leafgrid(leaves,w):
    out={};covered=[]
    for leaf in leaves:
        demand(leaf['value']==decode(leaf,w),'independent literal span/value')
        k=key(leaf['API_path']);demand(k not in out,'duplicate owner/path')
        out[k]=list(leaf['span']);covered.extend(range(*leaf['span']))
    demand(sorted(covered)==list(range(len(w))),'nonoverlapping literal partition of complete PDU')
    return out

def banner_ranges(w,partial=False,preline=False):
    if partial:return {}
    demand(w.endswith(b'\n'),'complete observed line required')
    end=len(w)-1-(len(w)>1 and w[-2]==13)
    if preline:return {'Text':[0,end]}
    demand(w.startswith((b'SSH-2.0-',b'SSH-1.99-')),'supported literal line prefix')
    version_end=7 if w.startswith(b'SSH-2.0-')else 8
    return {'Identification':[0,end],'Version':[4,version_end]}

plan=[]
for label,path,pinned in INPUTS:
    demand(sha(path)==pinned,'frozen input hash '+label)
    for row in json.loads(path.read_text(encoding='utf-8')):
        w=Path(row['raw_path']).read_bytes();demand(digest(w)==row['raw_SHA256'],'source raw binding')
        a=row['anchor'];lo,hi=a['physical_TCP_payload_span']
        demand(hi-lo==len(w)==a['TCP_payload_length'],'full authored single-contributor physical boundary')
        q={'dataset':label,'sourcecase':row['sourcecase'],'frame':row['frame'],'raw_path':row['raw_path'],'raw_SHA256':digest(w),'raw_length':len(w),'physical_frame_SHA256':a['physical_SHA256'],'physical_TCP_payload_span':[lo,hi],'source_kind':'single authored physical contributor; proposed ranges remain logical-PDU offsets','API_fields_ranges':{},'Session_current_literal_ranges':{},'private_alias_ranges':{},'forbidden_context_or_derived_ranges':['ssh.protocol','ssh.kex.selected','ssh.host_key.selected','ssh.host_key.sha256','ssh.signature.verified','ssh.peer_identity.verified','Identification when carried by a binary packet','Kex Family','Host Key Fingerprint','Host Key SHA256','Signature Verified','Peer Identity Verified','Context Level','Negotiation Context']}
        if row['expert']=='Encrypted':
            q['classification']='post-NEWKEYS opaque stream chunk; no SSH packet boundary or plaintext fields'
            q['forbidden_all_plaintext_ranges']=True
            plan.append(q);continue
        if row['kind']=='identification':
            q['classification']='current identification line'
            q['API_fields_ranges']=banner_ranges(w)
            q['Session_current_literal_ranges']=dict(q['API_fields_ranges'])
            # Existing ssh.protocol is context-origin even on a banner, so no pdu_range.
            plan.append(q);continue
        demand(len(w)>=16 and int.from_bytes(w[:4],'big')+4==len(w) and len(w)%8==0,'independent common envelope')
        q['private_alias_ranges']['ssh.message_code']=[5,6]
        q['Session_current_literal_ranges']['Message Number']=[5,6]
        if row['expected_status']=='malformed':
            demand(row.get('source_role_assessment')=='known-role-contradiction','only source-proven message-role refusal')
            _,leaves,_=source.plain_fields(w,'transport')
            q['classification']='wrong sender role; common envelope only, GetFields must remain refused'
            q['evidence_only_envelope_ranges']=leafgrid(leaves,w)
            q['GetFields_required_nil']=True
            q['forbidden_standard_method_ranges']=True
            plan.append(q);continue
        q['classification']='accepted initial packet, selected method or explicit opaque generic payload'
        q['API_fields_ranges']=leafgrid(row['API_wire_leaves'],w)
        if row['session'].get('Initial Packet Fields') is not None:
            q['Session_initial_packet_ranges']={'Initial Packet Fields/'+k:v for k,v in q['API_fields_ranges'].items()}
        if w[5]==20:
            rename={s:'Offered '+s for s in('Encryption C2S','Encryption S2C','MAC C2S','MAC S2C','Compression C2S','Compression S2C')}
            for name in ('Kex Algorithms','Host Key Algorithms','Encryption C2S','Encryption S2C','MAC C2S','MAC S2C','Compression C2S','Compression S2C','Languages C2S','Languages S2C','First Kex Packet Follows','Reserved'):
                q['Session_current_literal_ranges'][rename.get(name,name)]=q['API_fields_ranges']['Payload/'+name]
            for alias,name in [('ssh.kex_algorithms','Kex Algorithms'),('ssh.server_host_key_algorithms','Host Key Algorithms')]:q['private_alias_ranges'][alias]=q['Session_current_literal_ranges'][name]
        if 'Host Key Format' in row['session']:
            # This literal format belongs to this successfully selected reply; the SHA never does.
            q['Session_current_literal_ranges']['Host Key Format']=q['API_fields_ranges']['Payload/Host Key/Host Key Format']
        plan.append(q)

# Tiny authored line-only span vectors, not new physical packets or admitted sessions.
line_specs=[('SSH1.99-CRLF',b'SSH-1.99-RangeProbe comment\r\n',False,False),('SSH2-LF',b'SSH-2.0-RangeProbe\n',False,False),('compat-hyphen-literal',b'SSH-2.0-Range-Probe comment\r\n',False,False),('server-preline',b'Public notice\r\n',False,True),('partial-banner-no-LF',b'SSH-1.99-RangeProbe\r',True,False)]
lines=[];(D/'authored-line-only').mkdir()
for name,w,partial,preline in line_specs:
    path=D/'authored-line-only'/f'{name}.bin';path.write_bytes(w)
    lines.append({'name':name,'input_path':str(path),'SHA256':sha(path),'length':len(w),'complete_observed_line':not partial,'admission_not_asserted':True,'preline_requires_separate_observed_server_role':preline,'compat_warning_requires_separate_admission_proof':name=='compat-hyphen-literal','literal_ranges':banner_ranges(w,partial,preline)})

# Narrow, importable comparison of the proposed neutral metadata; never compare outputs to themselves.
def check_range_projection(source_row,actual):
    for k in ('API_fields_ranges','Session_current_literal_ranges','private_alias_ranges'):
        demand(actual.get(k,{})==source_row.get(k,{}),'exact current-owner range set '+k)
    for k in ('Session_initial_packet_ranges','evidence_only_envelope_ranges'):
        demand(actual.get(k,{})==source_row.get(k,{}),'exact optional owner range set '+k)
    if source_row.get('GetFields_required_nil'):demand(actual.get('GetFields_required_nil')is True,'refusal cannot grant successful fields')
    return True

tests=[]
def bad(name,row,edit):
    altered=copy.deepcopy(row);edit(altered)
    try:check_range_projection(row,altered)
    except AssertionError:tests.append(name);return
    raise AssertionError('mutation accepted '+name)
for row in plan:check_range_projection(row,copy.deepcopy(row))
ordinary=next(x for x in plan if 'Payload/Message Number'in x['API_fields_ranges'])
kex=next(x for x in plan if 'Payload/Kex Algorithms'in x['API_fields_ranges'])
reply=next(x for x in plan if 'Payload/Host Key/Host Key Format'in x['API_fields_ranges'])
unknown=next(x for x in plan if 'Payload/Opaque Message Data'in x['API_fields_ranges'])
refusal=next(x for x in plan if x.get('GetFields_required_nil'))
protected=next(x for x in plan if x.get('forbidden_all_plaintext_ranges'))
for label,owner,name in [('carried-identification','Session_current_literal_ranges','Identification'),('selected-cipher','Session_current_literal_ranges','Encryption C2S'),('derived-fingerprint','Session_current_literal_ranges','Host Key Fingerprint'),('context-private-alias','private_alias_ranges','ssh.kex.selected')]:bad(label,ordinary,lambda x,o=owner,n=name:x[o].update({n:[0,1]}))
bad('header-off-by-one',ordinary,lambda x:x['API_fields_ranges'].update({'Packet Length':[1,5]}))
bad('include-name-list-length-in-text',kex,lambda x:x['API_fields_ranges'].update({'Payload/Kex Algorithms':[x['API_fields_ranges']['Payload/Kex Algorithms'][0]-4,x['API_fields_ranges']['Payload/Kex Algorithms'][1]]}))
bad('same-none-value-is-not-same-occurence',kex,lambda x:x['API_fields_ranges'].update({'Payload/Compression S2C':x['API_fields_ranges']['Payload/Compression C2S']}))
bad('zero-width-language-grows-byte',kex,lambda x:x['API_fields_ranges'].update({'Payload/Languages C2S':[x['API_fields_ranges']['Payload/Languages C2S'][0],x['API_fields_ranges']['Payload/Languages C2S'][0]+1]}))
bad('nested-hostkey-owner-moved',reply,lambda x:x['API_fields_ranges'].update({'Host Key Format':x['API_fields_ranges'].pop('Payload/Host Key/Host Key Format')}))
bad('private-unknown-gets-hostkey-range',unknown,lambda x:x['API_fields_ranges'].update({'Payload/Host Key/Host Key Format':[10,21]}))
bad('role-refusal-reinterpreted-success',refusal,lambda x:x.update(GetFields_required_nil=False))
bad('protected-prefix-parsed-as-packet',protected,lambda x:x['API_fields_ranges'].update({'Packet Length':[0,4]}))

source_files=['common/bin-parser/parser/stream_parser/ssh_plaintext.go','common/bin-parser/parser/stream_parser/tls_certificate_tree.go','common/pcapx/pcaputil/bin_parser_ssh.go','common/pcapx/pcaputil/bin_parser.go','common/pcapx/pcaputil/bin_parser_session.go','common/pcapx/pcaputil/protocol_field_registry.go','common/yak/cmd/yakcmds/shark-cli/protocol_events.go','common/yak/cmd/yakcmds/shark-cli/protocol_detail_metadata.go','common/yak/cmd/yakcmds/shark-cli/decode_worker.go','common/yak/cmd/yakcmds/shark-cli/field_bytes.go']
review={
 'codec':'ssh_plaintext.go45..55 already owns exact byte spans;297..324 values() discards them. Export per-map direct literal ranges from this one tree; no byte searching or schema inference from selected name.',
 'public_rule':'tls_certificate_tree.go61/107 public Node positions are bit offsets + staged origin; standalone codec ranges must remain byte offsets relative to that logical PDU.',
 'native':'bin_parser.go1642..1647 generic transport projection then1676..1691 selected/identification replacement; binSSH.consume396..510 must attach only accepted current grammar ranges and preserve private clone ownership.',
 'context_collision':'KEXINIT Fields.Payload.Encryption C2S/MAC/Compression are offers; root Session same names are selected context. Rename current root ranges only to Offered ...; never flatten all Payload keys into root WR.',
 'private_aliases':'protocol_field_registry.go295..310 has only message_code/kex_algorithms/server_host_key_algorithms with origin=pdu; other9 SSH aliases remain range-free. Root lookup899..921 can use explicit current root semanticSession WR, but not nested same-name search.',
 'UI':'protocol_events.go240 and662 lack ssh nested WR support. At729..843 root ranges are taken only from Session, then Fields+Session merged and root WR deleted. Explicitly combine only proven source-owner root ranges or render fields and context in separate scopes; clear inherited ranges when entering any SSH map without local WR.',
 'workers':'packetDecodeRequest includes physical bytes and current event snapshots. Worker IPC serializes maps; nested uint64 spans become JSON Number/float and must go through strict detailWireRanges integer checks. Validate bounds against owned PDU source, not physical frame. ByteFields preserve typed []byte separately.',
 'immutable_ownership':'GetFields/private semanticFields and DisplayFieldValues/private semanticSession must clone nested WR; caller edits to public Session/Fields/returned maps cannot change later ranges. Extend existing protocol_ssh_field_projection_test.go ownership controls.',
 'refusals':'Wrong-role GetFields remains nil/error; message_code is an independently observed current byte and may retain5..6 via private Session. No method fields. Ignored guess/private unknown get only generic common envelope and opaque data ranges, no standard layouts.',
 'encrypted':'protectedSSHEvent is not a clear PDU; do not call codec on preview even if it syntactically resembles a reply. No header/message/method ranges; current Length may exceed retained Raw. Physical frame bytes remain separate.',
 'contributors':'All existing49/50 authored fields happen to be single-packet contributions. PDU ranges are not captured frame offsets; for split/reordered data use owned reconstructed PDU source + PacketRefs. Never rebase by one TCP payload offset or search equal octets across unrelated frames.',
 'budget':'Grammar has bounded fixed field counts and35000B plaintext cap, but additive WR metadata and duplicate selected trees must still be charged by existing sessionSnapshotSize/protocol history/IPC limits; do not bypass limited snapshots to reparse encrypted or refused fields.',
 'not_implemented':'This is source-only audit and proposed range schema, not evidence of product GetFields/typed/UI range support.'}
put(D/'range-plan.json',plan);put(D/'line-only-source-plan.json',lines);put(D/'integration-review.json',review)
counts={'source_payload_rows':len(plan),'accepted_binary_rows':sum(bool(x['API_fields_ranges']) and x['classification'].startswith('accepted')for x in plan),'API_binary_literal_spans':sum(len(x['API_fields_ranges']) for x in plan if x['classification'].startswith('accepted')),'banner_rows':sum(x['classification']=='current identification line'for x in plan),'role_refusal_rows':sum(bool(x.get('GetFields_required_nil'))for x in plan),'opaque_post_NEWKEYS_rows':sum(bool(x.get('forbidden_all_plaintext_ranges'))for x in plan),'existing_private_alias_ranges':sum(len(x['private_alias_ranges'])for x in plan),'line_only_vectors':len(lines),'mutation_controls_rejected':len(tests)}
put(D/'selftest.json',{'scope':'source metadata comparison tests only; no product execution','counts':counts,'negative_controls':tests,'all_passed':True})
manifest={'scope':'source49/50 derived independent source-range plan; no new Go/SDK/TShark/runtime; no candidate outputs read','existing_evidence_not_modified':True,'counts':counts,'inputs':[{'dataset':label,'path':str(path),'SHA256':sha(path)}for label,path,_ in INPUTS]+[{'path':str(BASE),'SHA256':sha(BASE)}],'script':{'path':str(Path(__file__)),'SHA256':sha(__file__)},'source_snapshot':[{'path':str(R/x),'SHA256':sha(R/x)}for x in source_files],'artifacts':[{'path':str(D/x),'SHA256':sha(D/x)}for x in ('range-plan.json','line-only-source-plan.json','integration-review.json','selftest.json')],'future_required_controls':['split field across two source packets: owned PDU span unchanged; no invented single physical range','out of order/retransmission contributor refs stay source-bound; equal bytes in different source packets do not dedup identity','quickDetail/detail/worker JSON nested ranges give same owner and source; no current fields from carried identification or selected cipher','LF/CRLF/SSH1.99 and compatibility literal warning do not shift/normalize captured offsets','negative protected preview64/192 cannot become plaintextSSHPacket even when cleartext-looking sentinel happens to fit codec']}
put(D/'manifest.json',manifest)
print(json.dumps({'manifest':str(D/'manifest.json'),'SHA256':sha(D/'manifest.json'),'counts':counts,'selftest_SHA256':sha(D/'selftest.json')},ensure_ascii=False))