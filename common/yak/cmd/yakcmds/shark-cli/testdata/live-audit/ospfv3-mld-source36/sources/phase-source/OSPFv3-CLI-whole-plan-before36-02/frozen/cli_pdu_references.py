"""Only generated PDU references in the existing CLI observation schema.

The caller supplies a separately verified native ledger. Physical numbers,
flow/conversation IDs, ranges and wire transaction integers are never remapped.
"""
import copy
from cli_source_expected import field_map


def remap_row(row,mapping):
    result=copy.deepcopy(row)
    def one(value,path):
        assert type(value)is int and value>=0,path
        if value==0:return 0
        assert value in mapping,(path,'unmapped generated PDU',value)
        found=mapping[value];assert type(found)is int and found>0,path
        return found
    def source(s,path):
        if s is None:return
        assert isinstance(s,dict),path
        if 'ParentPDU'in s:s['ParentPDU']=one(s['ParentPDU'],path+'/ParentPDU')
        if s.get('ParentPDUs')is not None:
            s['ParentPDUs']=[one(v,path+'/ParentPDUs')for v in s['ParentPDUs']]
    for group in('pdus','published_pdus'):
        for index,event in enumerate(result.get(group,[])):
            path=group+'/'+str(index);assert event['id']>0,path
            event['id']=one(event['id'],path+'/id')
            if 'response_to'in event:event['response_to']=one(event['response_to'],path+'/response_to')
            source(event.get('byte_source'),path+'/byte_source')
            snapshot=event['field_values']
            assert event['fields']==field_map(snapshot),path+'/original FieldMap'
            snapshot['pdu']=one(snapshot['pdu'],path+'/field_values/pdu')
            source(snapshot.get('source'),path+'/field_values/source')
            for i,item in enumerate(snapshot.get('values')or[]):
                name=item['name']
                if name not in('pdu.id','pdu.response_to') and not(event['protocol']=='ntp'and name=='ntp.control.request_pdu'):continue
                old=item['value'];mapped=one(old,path+'/field_values/'+str(i)+'/'+name)
                if 'exact_value'in item:
                    assert type(item['exact_value'])is str and item['exact_value']==str(old)
                    item['exact_value']=str(mapped)
                item['value']=mapped
            event['fields']=field_map(snapshot)
    return result
