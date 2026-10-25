"""Original NetFlow v9 UDP reference vs native fields and independent wire proofs.
Retains unknown templates, raw reference counts and unsupported versions/carriers.
No future packet supplies schemas to an earlier native data observation.
"""
from pathlib import Path
from decimal import Decimal
import base64,calendar,collections,datetime,hashlib,ipaddress,json,re,sys

oracle, events, output = map(Path,sys.argv[1:])
sha=lambda b:hashlib.sha256(b).hexdigest()
load=lambda p:json.loads(p.read_text(encoding='utf-8-sig'))
u=lambda b:int.from_bytes(b,'big')

def epoch(v):
    try:return Decimal(v)
    except Exception:
        m=re.fullmatch(r'(.*T\d\d:\d\d:\d\d)(?:\.(\d+))?(Z|[+-]\d\d:\d\d)',v)
        assert m,v
        dt=datetime.datetime.fromisoformat(m[1]+m[3].replace('Z','+00:00'))
        seconds=calendar.timegm(dt.astimezone(datetime.timezone.utc).utctimetuple())
        return Decimal(seconds)+Decimal('0.'+(m[2] or '0'))

def spans(v):
    if isinstance(v,list) and len(v)>=3 and isinstance(v[0],str): return [v]
    if isinstance(v,list): return [s for x in v for s in spans(x)]
    return []

def flatten(node, values, ranges):
    if isinstance(node,dict):
        for k,v in node.items():
            if k.endswith('_raw'):
                if k.startswith('cflow.'): ranges[k[:-4]].extend(spans(v))
            elif k.startswith('cflow.') and not isinstance(v,dict):
                if isinstance(v,list) and all(not isinstance(x,(dict,list)) for x in v): values[k].extend(v)
                elif not isinstance(v,list): values[k].append(v)
                else: flatten(v,values,ranges)
            else: flatten(v,values,ranges)
    elif isinstance(node,list):
        for v in node: flatten(v,values,ranges)

def canonical(v):
    if isinstance(v,str) and (':' in v or re.fullmatch(r'\d+\.\d+\.\d+\.\d+',v)):
        try:
            a=ipaddress.ip_address(v);return (a.version,a.packed.hex())
        except ValueError: pass
    if isinstance(v,str): return int(v,16 if v.startswith('0x') else 10)
    return v

def reference_records(node):
    if isinstance(node,dict):
        return sum((1 if re.match(r'^(?:Options? )?Flow\s+\d+\b',k) else 0)+reference_records(v) for k,v in node.items())
    if isinstance(node,list):return sum(map(reference_records,node))
    return 0

def parse(w):
    assert len(w)>=20 and u(w[:2])==9
    header=[u(w[:2]),u(w[2:4]),u(w[4:8]),u(w[8:12]),u(w[12:16]),u(w[16:20])]
    pos=20;sets=[];definitions={}
    while pos<len(w):
        assert len(w)-pos>=4
        ident,size=u(w[pos:pos+2]),u(w[pos+2:pos+4]);assert 4<=size<=len(w)-pos
        lo,end=pos+4,pos+size;ts=[]
        if ident in (0,1):
            at=lo
            while end-at>=4:
                start=at;tid,n=u(w[at:at+2]),u(w[at+2:at+4]);at+=4
                scope=0
                if ident==1:
                    opts=u(w[at:at+2]);at+=2;assert n%4==0 and opts%4==0
                    scope,n=n//4,(n+opts)//4
                assert tid>=256 and n>0 and 4*n<=end-at
                fs=[]
                for i in range(n):
                    typ,width=u(w[at:at+2]),u(w[at+2:at+4]);at+=4
                    assert width or i<scope and typ==1
                    fs.append((typ,width,i<scope))
                definition={'id':tid,'fields':fs,'start':start,'length':at-start,'sha':sha(w[start:at])}
                definitions[tid]=definition;ts.append(definition)
            assert end-at<=3
        sets.append({'id':ident,'start':pos,'size':size,'lo':lo,'end':end,'templates':ts})
        pos=end
    assert pos==len(w)
    return header,sets,definitions

original=load(oracle);ref={};versions=collections.Counter()
for row in original:
    ls=row['_source']['layers'];number=int(ls['frame']['frame.number'])
    values,ranges=collections.defaultdict(list),collections.defaultdict(list)
    flatten(ls.get('cflow'),values,ranges)
    versions.update(map(str,values.get('cflow.version',[])))
    candidates=spans(ls.get('cflow_raw'))
    ref[number]={'layers':ls,'values':values,'ranges':ranges,'raw':candidates}
native=[]
for line in events.open(encoding='utf-8-sig'):
    if line.startswith('{'):
        e=json.loads(line)
        if e['protocol']=='netflow-v9':
            e['_wire']=base64.b64decode(e['raw'] or '',validate=True)
            e['_hash']=sha(e['_wire']);native.append(e)

byhash=collections.defaultdict(list)
for e in native: byhash[e['_hash']].append(e)
key=lambda e:(json.dumps(e['domain'],sort_keys=True),e['source'],e['destination'],u(e['_wire'][16:20]))
refs=lambda e:[(r['Number'],json.dumps(r['Domain'],sort_keys=True)) for r in e['source_bytes']['PacketRefs'] or []]
claimedrefs=lambda row:[(r['Packet'],json.dumps({'Section':r['Section'],'Interface':r['Interface'],'Encapsulation':r['Encapsulation']},sort_keys=True)) for r in row['Template Packet References']]
completed=lambda e:(e['source_bytes'].get('CompletedIn') or {}).get('Number') or max(n for n,_ in refs(e))
timelines=collections.defaultdict(list)
for e in native:
    if len(e['_wire'])>=20 and u(e['_wire'][:2])==9:timelines[key(e)].append(e)
for group in timelines.values():group.sort(key=completed)
checks=collections.Counter();diff=[];known_refusals=[];matched=set();extra=[];unprojected=collections.Counter();opaque=collections.Counter();additional=collections.Counter()
# Declare the selected contract independently of what the current executable
# happens to emit. A globally missing registered alias must fail this audit.
registered=set('''cflow.version cflow.count cflow.unix_secs cflow.sequence cflow.source_id
cflow.flowset_id cflow.flowset_length cflow.template_id cflow.template_field_count
cflow.template_field_type cflow.template_field_length cflow.scope_field_type
cflow.option_scope_length cflow.option_length cflow.octets cflow.packets cflow.protocol
cflow.tos cflow.tcpflags cflow.srcport cflow.srcaddr cflow.srcmask cflow.inputint
cflow.dstport cflow.dstaddr cflow.dstmask cflow.outputint cflow.nexthop cflow.srcas
cflow.dstas cflow.bgpnexthop cflow.mulpackets cflow.muloctets cflow.post_octets
cflow.post_packets cflow.srcaddrv6 cflow.dstaddrv6 cflow.icmp_type_code_ipv4
cflow.direction cflow.flow_id cflow.firewall_event'''.split())
data_aliases=set('''cflow.octets cflow.packets cflow.protocol cflow.tos cflow.tcpflags
cflow.srcport cflow.srcaddr cflow.srcmask cflow.inputint cflow.dstport cflow.dstaddr
cflow.dstmask cflow.outputint cflow.nexthop cflow.srcas cflow.dstas cflow.bgpnexthop
cflow.mulpackets cflow.muloctets cflow.post_octets cflow.post_packets cflow.srcaddrv6
cflow.dstaddrv6 cflow.icmp_type_code_ipv4 cflow.direction cflow.flow_id cflow.firewall_event'''.split())

def equal(label,a,b,frame):
    checks[label]+=1
    if a!=b: diff.append({'frame':frame,'check':label,'native':a,'wire_or_reference':b})

for e in native:
    w=e['_wire'];numbers=[n for n,_ in refs(e)]
    candidates=[(n,s) for n in numbers if n in ref for s in ref[n]['raw'] if s[0]==w.hex()]
    if not candidates:
        extra.append({'id':e['id'],'status':e['status'],'frames':numbers});continue
    number,physical=candidates[-1];matched.add(number);r=ref[number];fields=e['session'] or {}
    equal('current PDU source SHA256',e['source_bytes']['SHA256'],sha(w),number)
    equal('captured UDP transport',e['transport'],'udp',number)
    ls=r['layers'];family='ipv6' if 'ipv6' in ls else 'ip'
    for native_key,addr,port in [('source','src','srcport'),('destination','dst','dstport')]:
        host=ls[family][family+'.'+addr];endpoint=('['+host+']') if ':' in host else host
        equal('captured endpoint distinct from reported record',e[native_key],endpoint+':'+ls['udp']['udp.'+port],number)
    try: h,wire_sets,defs=parse(w)
    except (AssertionError,IndexError,ValueError):
        if e['status']=='decoded':diff.append({'frame':number,'check':'native decoded non-selected structural wire'})
        continue
    names=['Version','Header Record Count','System Uptime Milliseconds','Unix Seconds','Sequence Number','Source ID']
    for name,v,(lo,hi) in zip(names,h,[(0,2),(2,4),(4,8),(8,12),(12,16),(16,20)]):
        equal('header value',fields.get(name),v,number)
        equal('header exact range',fields.get('Wire Field Ranges',{}).get(name),[lo,hi],number)
    equal('reported flows do not allocate captured flow',e['flow'],0,number)
    sets=fields.get('FlowSets') or []
    equal('FlowSet count',len(sets),len(wire_sets),number)
    data_count=0;template_count=sum(len(s['templates']) for s in wire_sets)
    for row,ws in zip(sets,wire_sets):
        equal('FlowSet ID',row['FlowSet ID'],ws['id'],number)
        equal('FlowSet length',row['FlowSet Length'],ws['size'],number)
        equal('template definition count',len(row.get('Templates') or []),len(ws['templates']),number)
        for native_def,wd in zip(row.get('Templates') or [],ws['templates']):
            equal('template ID',native_def['Template ID'],wd['id'],number)
            equal('template specifier count',len(native_def['Fields']),len(wd['fields']),number)
            for f,spec in zip(native_def['Fields'],wd['fields']):
                equal('template scoped field association',[f['Field Type'],f['Field Length'],f['Scope']],list(spec),number)
        if 'Records' not in row:continue
        src_candidates=[s for s in byhash[row['Template PDU SHA256']] if key(s)==key(e) and refs(s)==claimedrefs(row)]
        equal('template source capture/domain/exporter proof exists',bool(src_candidates),True,number)
        if not src_candidates:continue
        source=src_candidates[-1];sh,ss,sd=parse(source['_wire'])
        schema=sd.get(ws['id'])
        equal('template ID present in claimed source',schema is not None,True,number)
        if schema is None:continue
        equal('template definition hash',row['Template Definition SHA256'],schema['sha'],number)
        equal('template actual source range',[row['Template Definition Offset'],row['Template Definition Length']],[schema['start'],schema['length']],number)
        equal('template sequence is source message value',row['Template Sequence Number'],sh[4],number)
        source_numbers=[n for n,_ in refs(source) if n in ref]
        if source_numbers:
            t0=epoch(ref[source_numbers[-1]]['layers']['frame']['frame.time_epoch']);t1=epoch(r['layers']['frame']['frame.time_epoch'])
            equal('capture-time template lease',Decimal(0)<=t1-t0<Decimal(300),True,number)
        if source is not e:
            previous=source
            for middle in timelines[key(e)]:
                if not completed(source)<completed(middle)<=completed(e):continue
                mh=u(middle['_wire'][12:16]);ph=u(previous['_wire'][12:16])
                if mh==ph and middle['_hash']==previous['_hash']:continue
                equal('consecutive schema-use sequence proof',mh,(ph+1)&0xffffffff,number)
                du=(u(middle['_wire'][4:8])-u(previous['_wire'][4:8]))&0xffffffff
                equal('forward or wrapped uptime proof',du<0x80000000,True,number)
                equal('intervening observation accepted complete count',(middle.get('session') or {}).get('Record Count Verified'),True,number)
                if middle is not e:
                    try:
                        _,_,md=parse(middle['_wire'])
                        equal('no hidden observed template replacement',ws['id'] not in md,True,number)
                    except (AssertionError,IndexError,ValueError):
                        diff.append({'frame':number,'check':'intervening malformed export context'})
                previous=middle
        at=ws['lo'];record_size=sum(width for _,width,_ in schema['fields'])
        equal('data record count metadata',row['Record Count'],len(row['Records']),number)
        for index,record in enumerate(row['Records']):
            equal('record order/index',record['Index'],index+1,number)
            equal('record field count',len(record['Fields']),len(schema['fields']),number)
            for f,(typ,width,scope) in zip(record['Fields'],schema['fields']):
                equal('record/schema association',[f['Field Type'],f['Field Length'],f['Scope']],[typ,width,scope],number)
                raw=w[at:at+width]
                equal('record literal bounds',len(raw),width,number)
                if width: equal('record exact current-PDU range',f['Wire Field Ranges'].get('Value'),[at,at+width],number)
                else: equal('empty scope has no physical range',f['Wire Field Ranges'],{},number)
                kind=f['Value Kind'];value=f['Value']
                if kind=='unsigned':equal('unsigned primitive value',value,u(raw),number)
                elif kind in ('IPv4','IPv6'):
                    equal('address primitive bytes',ipaddress.ip_address(value).packed.hex(),raw.hex(),number)
                    equal('address width retained',ipaddress.ip_address(value).version,4 if kind=='IPv4' else 6,number)
                elif kind=='opaque':
                    equal('opaque bytes retained',base64.b64decode(value or '').hex(),raw.hex(),number);opaque[f'{typ}/{width}/scope={scope}']+=1
                elif kind=='scope':equal('derived System scope has zero octets',[scope,typ,width],[True,1,0],number)
                else:diff.append({'frame':number,'check':'unknown native value kind','kind':kind})
                at+=width
            data_count+=1
        equal('record/padding boundary',at<=ws['end'] and ws['end']-at<=3,True,number)
        equal('literal padding',base64.b64decode(row.get('Padding Bytes') or '').hex(),w[at:ws['end']].hex(),number)
    if fields.get('Record Count Verified'):
        equal('complete header record count',h[1],template_count+data_count,number)
        equal('data record total',fields['Data Record Count'],data_count,number)
    else:
        known_refusals.append({'frame':number,'status':e['status'],'declared_records':h[1],'templates':template_count,'native_data_records':data_count,'reference_data_records':reference_records(r['layers'].get('cflow')),'reason':e.get('error')})
    ns=collections.defaultdict(list)
    for v in e['field_values']['values']:
        if v['name'] in registered:ns[v['name']].append(v)
    for name in sorted(registered):
        want=r['values'].get(name) or [];got=ns.get(name) or []
        if not want and not got:continue
        if name in data_aliases and not fields.get('Record Count Verified') and not got:continue
        if not want:
            additional[name]+=len(got);continue
        if not got and name not in registered:continue
        checks['typed alias occurrence/value/range']+=max(len(got),len(want),1)
        a=collections.Counter(canonical(v['value']) for v in got);b=collections.Counter(canonical(v) for v in want)
        same=a==b
        rr=r['ranges'].get(name) or []
        if len(rr)==len(want)==len(got):
            expected=collections.Counter((canonical(v),s[1],s[2]) for v,s in zip(want,rr))
            actual=collections.Counter((canonical(v['value']),physical[1]+v['pdu_range']['offset'],v['pdu_range']['length']) for v in got if 'pdu_range' in v)
            same=same and expected==actual
        if not same:diff.append({'frame':number,'check':'typed alias occurrence/value/range','field':name,'reference':want,'native':[v['value'] for v in got]})
    for name,values in r['values'].items():
        if name not in registered:unprojected[name]+=len(values)

result={'scope':__doc__,'oracle_sha256':sha(oracle.read_bytes()),'events_sha256':sha(events.read_bytes()),'original_reference_frames':len(original),'original_reference_versions':dict(versions),
        'original_reference_data_records':sum(reference_records(row['_source']['layers'].get('cflow')) for row in original),
        'reference_data_records_by_version':{v:sum(reference_records(row['_source']['layers'].get('cflow')) for row in original if str(row['_source']['layers']['cflow'].get('cflow.version'))==v) for v in versions},
        'native_events':len(native),'native_statuses':dict(collections.Counter(e['status'] for e in native)),
        'matched_original_frames':len(matched),'checks':dict(checks),'checks_total':sum(checks.values()),'selected_differences':diff,'contextual_refusals':known_refusals,
        'opaque_field_occurrences':dict(opaque),'additional_native':extra,'additional_native_alias_fields':dict(additional),'reference_fields_not_projected':dict(unprojected),
        'unselected_reference_frames':[n for n in ref if n not in matched]}
output.write_bytes((json.dumps(result,indent=2)+'\n').encode())
print(json.dumps({k:v for k,v in result.items() if k not in ['scope','selected_differences','reference_fields_not_projected','contextual_refusals','unselected_reference_frames']}))
print('selected_differences',len(diff),'contextual_refusals',len(known_refusals))
raise SystemExit(bool(diff or extra))
