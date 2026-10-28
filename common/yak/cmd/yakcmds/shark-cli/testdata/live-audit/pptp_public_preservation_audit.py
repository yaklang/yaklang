"""Audit the PPTP correction without normalizing wire fields or connection IDs.

The direct public PPTP refusals are independently recomputed from original
capture bytes and source-zero reference fields. All other complete SDK events
are paired by literal source, protocol, bytes, endpoints and connection domain.
Only generated PDU references in explicit native schemas may be translated.
The CLI is then checked through that same SDK-attested bijection followed by
the independent closed-TCP projection auditor. A failed CLI stage produces a
complete changed-field inventory and cannot be reported as preservation.
"""
import argparse,collections,copy,hashlib,io,itertools,json
from pathlib import Path
from clock_public_preservation_audit import canonical,key
import pptp_parent_source_audit as pptp
import gre_parent_source_audit as parents
import closed_tcp_projection_audit as closed


def rows(path):
    with path.open(encoding='utf-8-sig')as f:
        for line in f:yield json.loads(line)
def sha(path):
    h=hashlib.sha256()
    with path.open('rb')as f:
        for block in iter(lambda:f.read(1<<20),b''):h.update(block)
    return h.hexdigest()
def digest(value):return hashlib.sha256(canonical(value).encode()).hexdigest()
def number(value):assert type(value)is int and value>=0;return value
def target(event,targets):
    refs=event['source_bytes'].get('PacketRefs')or[]
    selected=[r['Number']for r in refs if r['Number']in targets]
    if not selected:return None
    assert len(refs)==len(selected)==1,'adjudicated parent source cannot contribute to another message'
    return selected[0]


def generated(value,ids):
    number(value)
    if not value:return value
    assert value in ids,'generated references must belong to an actual source-paired SDK PDU'
    return ids[value]


def source_aliases(source,ids):
    source['ParentPDU']=generated(source.get('ParentPDU',0),ids)
    if source.get('ParentPDUs')is not None:source['ParentPDUs']=[generated(n,ids)for n in source['ParentPDUs']]


def typed_aliases(fields,protocol,ids):
    fields['pdu']=generated(fields['pdu'],ids);source_aliases(fields['source'],ids)
    for field in fields.get('values')or[]:
        if field['name']=='pdu.id'or field['name']=='pdu.response_to'and protocol in['http','quic','http3']:
            old=number(field['value']);field['value']=generated(old,ids)
            if'exact_value'in field:
                assert field['exact_value']==str(old),'typed exact-value alias must agree with its actual numeric value'
                field['exact_value']=str(field['value'])


def translate(event,ids):
    event=copy.deepcopy(event);protocol=event['protocol'];event['id']=generated(event['id'],ids)
    typed_aliases(event['field_values'],protocol,ids);source_aliases(event['source_bytes'],ids)
    if protocol=='http'and event['transaction']:
        assert event['response_to']in[0,event['transaction']]
        event['transaction']=generated(event['transaction'],ids);event['response_to']=generated(event['response_to'],ids)
    if protocol in['quic','http3']:
        session=event['session']or{}
        def conformance(value):
            if'Header PDU ID'in value:value['Header PDU ID']=generated(value['Header PDU ID'],ids)
        def quic_source(value):
            assert value['Kind']=='quic-authenticated-stream-reassembly'
            value['Parent PDUs']=[generated(n,ids)for n in value['Parent PDUs']]
            for span in value['Ranges']or[]:span['Parent PDU']=generated(span['Parent PDU'],ids)
        def application_sources(frames):
            for frame in frames:
                if'Message Conformance'in frame:conformance(frame['Message Conformance'])
                for name in['Byte Source','Payload Byte Source']:
                    if name in frame:quic_source(frame[name])
        request_ids={f['Request PDU ID']for f in session.get('Frames')or[]if f.get('Request PDU ID',0)}
        if event['transaction']:
            assert request_ids=={event['transaction']}and event['response_to']in[0,event['transaction']]
            event['transaction']=generated(event['transaction'],ids);event['response_to']=generated(event['response_to'],ids)
        if'HTTP3 Message Conformance'in session:conformance(session['HTTP3 Message Conformance'])
        application_sources(session.get('HTTP3 Frames')or[])
        for frame in session.get('Frames')or[]:
            for name in['Request PDU ID','Transaction ID']:
                if name in frame:frame[name]=generated(frame[name],ids)
            if'HTTP3 Message Conformance'in frame:conformance(frame['HTTP3 Message Conformance'])
            source=frame.get('Byte Source')
            if source is not None:
                assert source['Kind']=='quic-authenticated-plaintext';source['Parent PDU']=generated(source['Parent PDU'],ids)
            if'Delivered Byte Source'in frame:quic_source(frame['Delivered Byte Source'])
            application_sources(frame.get('HTTP3 Frames')or[])
    return event


def projection(event,publication=False,fallback=False):
    fields=collections.defaultdict(list)
    for field in event['field_values'].get('values')or[]:fields[field['name']].append(field['value'])
    projected=dict(byte_source=event['source_bytes'],completeness=event['completeness'],field_values=event['field_values'],fields=dict(fields),id=event['id'],protocol=event['protocol'],response_to=event['response_to'],status=event['status'])
    if not fallback:
        if event.get('tcp_flow_id'):projected['tcp_flow_id']=event['tcp_flow_id']
        if event.get('error')or event.get('expert')or publication:projected.update(error=event.get('error',''),expert=event.get('expert',''),summary=event['summary'])
    return projected


def projection_forms(event):
    # packet_observation.go emits optional diagnostic/publication details and
    # native TCP ownership; command.go's legacy fallback emits the base form.
    # These are exact exporter schemas, not permission to change CLI fields:
    # the subsequent complete-row comparison still requires their literal
    # presence, type and values to be preserved.
    return sorted({digest(projection(event)),digest(projection(event,publication=True)),digest(projection(event,fallback=True))})


def target_row(row,event,record):
    """Require the complete current refusal row, not merely an allowed label."""
    pptp.refusal_row(row,event,row['number'],record);claim=pptp.profile(record);domain=pptp.record_domain(record)
    expected=dict(physical_source=claim['outer']['source'],physical_destination=claim['outer']['destination'],message_transport='network',message_domain=domain,capture_domain=domain,pdus=[projection(event)],pdu_scope='observed_at_packet',protocol_stack=['Ethernet','IPv4'],visibility='unselected-network-profile',protocol_status='limited',protocol_scope='outer network prefix; inner identity unconfirmed',transport='network',layers=['Ethernet','IPv4'],protocol_evidence='encapsulation profile unselected; child headers not admitted',number=row['number'],time=row['time'],source='',destination='',protocol='IPv4',length=len(record['wire']),captured_length=len(record['wire']),info=event['summary'])
    assert canonical(row)==canonical(expected),'every PPTP-corrected CLI field must derive from its exact native parent observation'


def sdk(before,after,targets,source_proof):
    available=collections.defaultdict(list);old={};new={};old_target={};new_target={};before_counts=collections.Counter();after_counts=collections.Counter();before_IP=collections.Counter();after_IP=collections.Counter();retained_IP=collections.Counter()
    for event in rows(before):
        pdu=number(event['id']);assert pdu>0 and pdu not in old;before_counts[event['protocol']]+=1
        old[pdu]=(digest(event),projection_forms(event),event['protocol'])
        if event['protocol']=='ip':before_IP[event.get('expert','')]+=1
        n=target(event,targets)
        if n is not None:assert n not in old_target;old_target[n]=pdu
        else:available[key(event)].append(pdu)
    new_to_old={};old_to_new={};paired=[]
    for event in rows(after):
        pdu=number(event['id']);assert pdu>0 and pdu not in new;after_counts[event['protocol']]+=1
        new[pdu]=(projection_forms(event),event['protocol'])
        if event['protocol']=='ip':after_IP[event.get('expert','')]+=1
        n=target(event,targets)
        if n is not None:assert n not in new_target;new_target[n]=pdu;continue
        candidates=available.get(key(event));assert candidates,('unpaired literal SDK source/protocol/bytes/flow/domain',pdu,event['protocol'])
        prior=candidates.pop(0);assert prior not in old_to_new;new_to_old[pdu]=prior;old_to_new[prior]=pdu;paired.append((prior,pdu))
    assert not any(available.values()),'no non-adjudicated SDK observation can disappear'
    assert set(new_target)==set(targets)and len(old_target)==171 and len(new_target)==271
    for item in source_proof['explicit_domain_and_diagnostic_migrations']:
        n=item['frame'];a=old_target[n];b=new_target[n];assert(a,b)==(item['old_PDU'],item['new_PDU']);new_to_old[b]=a;old_to_new[a]=b
    assert len(old_to_new)==len(old)and len(new_to_old)==len(old),'one-to-one generated SDK aliases, except the 100 source-proved new diagnostics'
    exact=collections.Counter();renumbered=collections.Counter();witness=None
    for event in rows(after):
        if target(event,targets)is not None:continue
        prior=new_to_old[event['id']];translated=translate(event,new_to_old)
        assert digest(translated)==old[prior][0],('non-generated SDK value changed',event['protocol'],prior,event['id'])
        if event['protocol']=='ip':retained_IP[event.get('expert','')]+=1
        if digest(event)==old[prior][0]:exact[event['protocol']]+=1
        else:renumbered[event['protocol']]+=1
        if witness is None and event['protocol']!='ip'and prior!=event['id']and event['source_bytes'].get('PacketRefs'):witness=event
    assert before_counts['ip']+100==after_counts['ip']and{p:n for p,n in before_counts.items()if p!='ip'}=={p:n for p,n in after_counts.items()if p!='ip'}
    expected_IP=before_IP.copy();expected_IP['UnsupportedOrMalformedNetworkLayer']-=171
    assert expected_IP==retained_IP and after_IP==retained_IP+collections.Counter(PPTPHeaderAdmission=271),'only independently proved PPTP parent diagnostics alter IP expert counts'
    mapping=sorted(old_to_new.items());proof=dict(before_SDK_SHA256=sha(before),after_SDK_SHA256=sha(after),before_records=sum(before_counts.values()),after_records=sum(after_counts.values()),before_protocol_counts=dict(before_counts),after_protocol_counts=dict(after_counts),before_IP_expert_counts=dict(before_IP),after_IP_expert_counts=dict(after_IP),strict_paired_IP_expert_counts=dict(retained_IP),paired_non_PPTP_records=len(paired),paired_non_IP_records=sum(n for p,n in exact.items()if p!='ip')+sum(n for p,n in renumbered.items()if p!='ip'),literal_complete_event_counts=dict(exact),generated_reference_only_counts=dict(renumbered),SDK_alias_bijection_pairs=len(mapping),SDK_alias_bijection_SHA256=digest(mapping),source_proved_old_parent_corrections=171,source_proved_new_parent_observations=100,all_other_values_types_bytes_domains_and_connection_IDs_literal=True)
    return proof,old_to_new,old,new,witness


def continuity(capture,before,after,cli):
    proof=parents.audit(capture,after,cli,before);assert proof['old_parent_count']==proof['source_bound_parent_refusals']==proof['retained_old_parent_count']==10385
    assert not proof['replaced_old_parents']and not proof['authorized_removed_non_IP_observations']and proof['observed_legacy_generic_markers']==0
    return{k:v for k,v in proof.items()if k not in['scope','observations']}|dict(source_bound_parent_observations_SHA256=digest(proof['observations']))


class IterableBytes(io.RawIOBase):
    def __init__(self,pieces):super().__init__();self.pieces=iter(pieces);self.pending=b''
    def readable(self):return True
    def readinto(self,buffer):
        while not self.pending:
            try:self.pending=next(self.pieces)
            except StopIteration:return 0
        count=min(len(buffer),len(self.pending));buffer[:count]=self.pending[:count];self.pending=self.pending[count:];return count


class ProjectedJSONL:
    """Replayable intermediate; its logical byte hash is recorded in the proof."""
    def __init__(self,path,transform):self.path=path;self.transform=transform
    def open(self,mode='r',encoding=None):
        pieces=((canonical(self.transform(row))+'\n').encode()for row in rows(self.path));binary=io.BufferedReader(IterableBytes(pieces),buffer_size=1<<20)
        return binary if'b'in mode else io.TextIOWrapper(binary,encoding=encoding or'utf8')


def projector(ids,old,new,corrected):
    def change(row):
        row=copy.deepcopy(row)
        if row.get('record_type')!='protocol_publication'and row.get('number')in corrected:return copy.deepcopy(corrected[row['number']])
        for name in['pdus','published_pdus']:
            for pdu in row.get(name)or[]:
                prior=number(pdu['id']);assert prior in old and digest(pdu)in old[prior][1],('CLI PDU must equal a literal native exporter schema',row.get('number'),prior,pdu.get('protocol'))
                actual=generated(prior,ids);protocol=pdu['protocol'];pdu['id']=actual;source_aliases(pdu['byte_source'],ids);typed_aliases(pdu['field_values'],protocol,ids)
                for field in['pdu.id',*(['pdu.response_to']if protocol in['http','quic','http3']else[])]:
                    if field in pdu['fields']:pdu['fields'][field]=[generated(n,ids)for n in pdu['fields'][field]]
                if protocol in['http','quic','http3']:pdu['response_to']=generated(pdu['response_to'],ids)
                assert digest(pdu)in new[actual][0],('translated CLI projection must equal an actual source-paired native exporter schema',row.get('number'),prior,actual,protocol)
        return row
    return change


def inventory(before,after,targets):
    fields=collections.Counter();changes=[];count=0
    for ordinal,(a,b)in enumerate(itertools.zip_longest(rows(before),rows(after)),1):
        assert a is not None and b is not None,'row/publication sequence must remain complete';count+=1
        if canonical(a)==canonical(b):continue
        changed=sorted(k for k in set(a)|set(b)if(k in a)!=(k in b)or canonical(a.get(k))!=canonical(b.get(k)));fields.update(changed)
        changes.append(dict(ordinal=ordinal,frame=b.get('number'),record_type=b.get('record_type','packet'),PPTP_adjudicated_frame=b.get('number')in targets,changed_fields=changed,before_logical_row_SHA256=digest(a),after_logical_row_SHA256=digest(b)))
    return dict(exported_rows=count,different_rows=len(changes),changed_root_field_counts=dict(fields),all_changed_rows=changes)


def negative_controls(witness,old,ids):
    assert witness is not None;inverse={v:k for k,v in ids.items()};prior=inverse[witness['id']];expected=old[prior][0]
    assert digest(translate(witness,inverse))==expected
    def reject(name,mutation):
        value=copy.deepcopy(witness);mutation(value)
        try:assert digest(translate(value,inverse))==expected
        except(AssertionError,KeyError):return dict(case=name,rejected=True)
        raise AssertionError('negative control accepted: '+name)
    maximum=max(inverse)+1
    def wire_length(event):
        field=next(f for f in event['field_values']['values']if f['name']=='pdu.length');assert type(field['value'])is int and field['value']!=event['id'];field['value']=event['id']
        if'exact_value'in field:field['exact_value']=str(field['value'])
    controls=[reject('unpaired generated PDU source',lambda e:e.__setitem__('id',maximum)),reject('native TCP connection ID is not a generated PDU alias',lambda e:e.__setitem__('tcp_flow_id',e.get('tcp_flow_id',0)+1)),reject('flow namespace is not normalized',lambda e:e.__setitem__('flow',e['flow']+1)),reject('source endpoint stays literal',lambda e:e.__setitem__('source','203.0.113.99:7')),reject('capture encapsulation stays literal',lambda e:e['domain'].__setitem__('Encapsulation',e['domain']['Encapsulation']+'/forged')),reject('wire message length integer is not a generated alias',wire_length),reject('typed generated exact value must agree',lambda e:next(f for f in e['field_values']['values']if f['name']=='pdu.id').__setitem__('exact_value','-1')),reject('source contributor frame stays literal',lambda e:e['source_bytes']['PacketRefs'][0].__setitem__('Number',e['source_bytes']['PacketRefs'][0]['Number']+1))]
    return dict(positive_actual_source_pair=dict(old_PDU=prior,new_PDU=witness['id'],protocol=witness['protocol']),meaningful_mutations=len(controls),controls=controls)


def audit(capture,before_sdk,after_sdk,before_cli,after_cli,inventory_path,oracle,columns,observer=None,stage_dir=None):
    completed=[]
    def stage(name,value):
        if stage_dir is not None:
            destination=stage_dir/(name+'.json')
            with destination.open('x',encoding='utf8')as f:json.dump(dict(auditor_SHA256=sha(Path(__file__)),stage=name,result=value),f,indent=2);f.write('\n')
            completed.append(dict(stage=name,path=str(destination),SHA256=sha(destination)))
    source=pptp.audit_public(capture,after_sdk,after_cli,inventory_path,before_sdk,oracle,columns);targets=pptp.direct_v1_records(capture)
    stage('01-raw-PPTP-source',source)
    print(json.dumps(dict(stage='raw-PPTP-source',refused_sources=source['direct_source_bound_precise_refusals'])),flush=True)
    strict,ids,old,new,witness=sdk(before_sdk,after_sdk,targets,source)
    stage('02-strict-complete-SDK',strict)
    negatives=negative_controls(witness,old,ids);stage('03-actual-source-negative-controls',negatives)
    print(json.dumps(dict(stage='strict-complete-SDK',paired_non_IP=strict['paired_non_IP_records'],paired_all_non_PPTP=strict['paired_non_PPTP_records'],IP_experts=strict['strict_paired_IP_expert_counts'])),flush=True)
    retained=continuity(capture,before_sdk,after_sdk,after_cli)
    stage('04-raw-original-parent-continuity',retained)
    print(json.dumps(dict(stage='raw-original-parent-continuity',retained=retained['source_bound_parent_refusals'])),flush=True)
    native={}
    for e in rows(after_sdk):
        n=target(e,targets)
        if n is not None:native[n]=e
    corrected={};physical_before={}
    for row in rows(after_cli):
        if row.get('record_type')!='protocol_publication'and row.get('number')in targets:
            n=row['number'];target_row(row,native[n],targets[n]);assert n not in corrected;corrected[n]=row
    for row in rows(before_cli):
        if row.get('record_type')!='protocol_publication'and row.get('number')in targets:
            n=row['number'];assert n not in physical_before;physical_before[n]=row
    assert set(corrected)==set(physical_before)==set(targets)
    target_changes=[]
    for n,row in corrected.items():
        prior=physical_before[n]
        for key in['number','time','length','captured_length']:assert canonical(prior[key])==canonical(row[key]),'PPTP corrections retain literal physical source facts'
        for key in['Section','Interface']:assert prior['capture_domain'][key]==row['capture_domain'][key]
        target_changes.append(dict(frame=n,source_wire_SHA256=pptp.digest(targets[n]['wire']),old_capture_domain=prior['capture_domain'],new_capture_domain=row['capture_domain'],before_CLI_row_SHA256=digest(prior),after_CLI_row_SHA256=digest(row),changed_fields=sorted(k for k in set(prior)|set(row)if canonical(prior.get(k))!=canonical(row.get(k)))))
    transform=projector(ids,old,new,corrected);intermediate=ProjectedJSONL(before_cli,transform)
    differences=inventory(intermediate,after_cli,targets)
    cli=dict(stage='SDK-backed generated references and source-bound PPTP corrections, then closed-TCP projection',virtual_intermediate_recipe='canonical original JSONL with actual SDK alias bijection and complete independently recomputed 271 parent refusal rows',virtual_intermediate_SHA256=sha(intermediate),generated_alias_bijection_SHA256=strict['SDK_alias_bijection_SHA256'],source_bound_PPTP_CLI_corrections=target_changes,post_alias_and_PPTP_difference_inventory=differences,all_remaining_CLI_changes_independently_certified=False)
    try:
        cli['closed_TCP_projection']=closed.audit(capture,intermediate,after_cli,observer);cli['all_remaining_CLI_changes_independently_certified']=True
    except AssertionError as error:cli['closed_TCP_projection_failure']=str(error)
    stage('05-complete-CLI-chain',cli)
    return dict(scope=__doc__,auditor_SHA256=sha(Path(__file__)),capture_SHA256=sha(capture),before_CLI_SHA256=sha(before_cli),after_CLI_SHA256=sha(after_cli),strict_SDK=strict,source_bound_PPTP=source,retained_original_parents=retained,CLI=cli,negative_controls=negatives,completed_stage_artifacts=completed,all_SDK_records_and_CLI_projections_certified=cli['all_remaining_CLI_changes_independently_certified'],full_protocol_or_negotiated_PPTP_call_parity_claim=False)


if __name__=='__main__':
    parser=argparse.ArgumentParser(description=__doc__)
    for name in['capture','before-sdk','after-sdk','before-cli','after-cli','inventory','oracle','columns','out']:parser.add_argument('--'+name,type=Path,required=True)
    parser.add_argument('--observer',type=Path);parser.add_argument('--stage-dir',type=Path);args=parser.parse_args()
    for value in vars(args).values():
        if value is not None:assert not any((v/'.git').exists()for v in[value.resolve(),*value.resolve().parents])
    if args.stage_dir:args.stage_dir.mkdir(exist_ok=False)
    result=audit(args.capture,args.before_sdk,args.after_sdk,args.before_cli,args.after_cli,args.inventory,args.oracle,args.columns,args.observer,args.stage_dir)
    with args.out.open('x',encoding='utf8')as f:json.dump(result,f,indent=2);f.write('\n')
    print(json.dumps(dict(strict_non_IP=result['strict_SDK']['paired_non_IP_records'],retained_parents=result['retained_original_parents']['source_bound_parent_refusals'],remaining_CLI_rows=result['CLI']['post_alias_and_PPTP_difference_inventory']['different_rows'],all_stages_certified=result['all_SDK_records_and_CLI_projections_certified'])),flush=True)
