"""Compare literal reference wire fields across an attested prefix observation.

Compare every available positive-length raw descriptor and its scalar value in
each unchanged original packet. Generated FT_FRAMENUM references use only the
attested record mapping and are checked separately, even when the reference
attaches them to a positive-length byte range. Other generated fields without
raw bytes are outside this check. No stream/connection IDs or wire integers are
normalized. Native correctness is separately source/graph audited.
"""
import argparse,collections,hashlib,json
from pathlib import Path


def sha(p):return hashlib.sha256(p.read_bytes()).hexdigest()
def canonical(v):return json.dumps(v,sort_keys=True,separators=(',',':'))


def frames(path):
    result={}
    for packet in json.loads(path.read_text('utf8')):
        layers=packet['_source']['layers'];literal=layers['frame']['frame.number'];assert type(literal)is str
        n=int(literal);assert n>0 and n not in result;result[n]=layers
    return result


def wire_fields(layers,frame_fields,translate_ref=None):
    result=collections.Counter();references=collections.Counter()
    def frame_value(value):
        if isinstance(value,list):return[frame_value(v)for v in value]
        assert type(value)is str and value.isascii()and value.isdecimal()and str(int(value))==value
        n=int(value)
        return str(translate_ref(n))if translate_ref is not None and n else value
    def walk(node):
        if isinstance(node,list):
            for value in node:walk(value)
        elif isinstance(node,dict):
            for name,value in node.items():
                if name.endswith('_raw')and name!='frame_raw':
                    # Some actual field abbreviations end in _raw, notably
                    # tcp.seq_raw. Its scalar absolute sequence is paired with
                    # the separate tcp.seq_raw_raw descriptor, never discarded
                    # or mistaken for descriptor metadata.
                    if not isinstance(value,list):continue
                    descriptors=[value]if value and type(value[0])is str else value
                    associated=node.get(name[:-4]);scalar=not isinstance(associated,dict)and(not isinstance(associated,list)or all(not isinstance(v,(dict,list))for v in associated))
                    for index,descriptor in enumerate(descriptors):
                        assert len(descriptor)==6 and type(descriptor[0])is str and all(type(v)is int for v in descriptor[1:])
                        observed=associated[index]if scalar and isinstance(associated,list)and len(associated)==len(descriptors)else associated if scalar else'<protocol subtree>'
                        if name[:-4]in frame_fields:
                            references[canonical([name[:-4],descriptor,frame_value(observed)])]+=1;continue
                        if descriptor[2]<=0:continue # Other generated or unavailable fields have no wire witness.
                        result[canonical([name[:-4],descriptor,observed])]+=1
                elif not name.endswith('_raw')and name!='frame':walk(value)
    walk(layers);return result,references


def audit(before,after,manifest,wire_proof,before_receipt,after_receipt,reference_schema):
    doc=json.loads(manifest.read_text('utf8'));proof=json.loads(wire_proof.read_text('utf8'));old_receipt=json.loads(before_receipt.read_text('utf8'));new_receipt=json.loads(after_receipt.read_text('utf8'))
    assert proof['manifest_SHA256']==sha(manifest)and proof['source_capture_SHA256']==old_receipt['source_capture_SHA256']
    assert proof['capture_SHA256']==new_receipt['capture_SHA256']and sha(before)==old_receipt['reference_SHA256']and sha(after)==new_receipt['export_SHA256']
    schema=json.loads(reference_schema.read_text('utf8'));assert schema['binary_SHA256']==old_receipt['binary_SHA256']==new_receipt['binary_SHA256']
    frame_fields=set(schema['frame_reference_fields']);assert frame_fields and len(frame_fields)==len(schema['frame_reference_fields'])
    assert doc['case_scope']=='all-original-records-plus-one-declared-prefix'and len(proof['inserted_prefixes'])==1
    original=frames(before);current=frames(after);assert set(current)==set(range(1,len(doc['ledger'])+1))
    ledger={v['number']:v for v in doc['ledger']}
    def translate(n):
        assert n in ledger and ledger[n]['change']['kind']=='exact','generated references cannot attach an incomplete-header observation to an original protocol message'
        return ledger[n]['source_frame']
    seen=set();counts=collections.Counter();checks=reference_checks=0
    for entry in doc['ledger']:
        if entry['change']['kind']!='exact':continue
        n=entry['source_frame'];assert n in original and n not in seen;seen.add(n);a=original[n];b=current[entry['number']]
        assert a['frame_raw']==b['frame_raw'],'each original captured frame remains literal in the reference'
        left,old_refs=wire_fields(a,frame_fields);right,new_refs=wire_fields(b,frame_fields,translate)
        assert old_refs==new_refs,('attested generated frame references changed',n)
        reference_checks+=sum(old_refs.values())
        if left!=right:
            delta=(left-right)+(right-left);names=collections.Counter()
            for k,v in delta.items():names[json.loads(k)[0]]+=v
            raise AssertionError(('reference wire fields changed',n,dict(names)))
        checks+=sum(left.values())
        for k,v in left.items():counts[json.loads(k)[0]]+=v
    assert seen==set(original)
    return dict(scope=__doc__,baseline_reference_SHA256=sha(before),reference_SHA256=sha(after),manifest_SHA256=sha(manifest),wire_proof_SHA256=sha(wire_proof),auditor_SHA256=sha(Path(__file__)),reference_schema_SHA256=sha(reference_schema),original_reference_frames=len(seen),literal_raw_field_occurrences=checks,attested_generated_frame_references=reference_checks,field_counts=dict(counts),all_available_raw_descriptors_and_scalar_values_literal=True,generated_analysis_or_all_protocol_parity_claim=False)


if __name__=='__main__':
    ap=argparse.ArgumentParser(description=__doc__)
    for n in['before','after','manifest','wire-proof','before-receipt','after-receipt','reference-schema','out']:ap.add_argument('--'+n,type=Path,required=True)
    a=ap.parse_args()
    for p in vars(a).values():assert not any((v/'.git').exists()for v in[p.resolve(),*p.resolve().parents])
    proof=audit(a.before,a.after,a.manifest,a.wire_proof,a.before_receipt,a.after_receipt,a.reference_schema)
    with a.out.open('x',encoding='utf8')as f:json.dump(proof,f,indent=2);f.write('\n')
    print(json.dumps(dict(original_frames=proof['original_reference_frames'],literal_wire_fields=proof['literal_raw_field_occurrences'])),flush=True)
