"""Exact compact CLI PDU schemas; physical row/flow/frame facts are literal."""
import copy


def remap_cli_row_pdu_references(row, mapping):
    out = copy.deepcopy(row)

    def one(value, path):
        assert type(value) is int and value >= 0, path
        if value == 0:
            return 0
        assert value in mapping, (path, 'unmapped generated PDU', value)
        result = mapping[value]
        assert type(result) is int and result > 0, path
        return result

    def source(value, path):
        if value is None:
            return
        assert isinstance(value, dict), path
        if 'ParentPDU' in value:
            value['ParentPDU'] = one(value['ParentPDU'], path + '/ParentPDU')
        if value.get('ParentPDUs') is not None:
            assert isinstance(value['ParentPDUs'], list), path
            value['ParentPDUs'] = [one(v, path + '/ParentPDUs') for v in value['ParentPDUs']]

    for group in ['pdus', 'published_pdus']:
        for index, pdu in enumerate(out.get(group) or []):
            path = group + '/' + str(index)
            if pdu['id'] == 0:
                # This is an existing observation-budget marker, not a native
                # PDU. Preserve it completely; never allocate or map its zero.
                assert type(pdu['id']) is int and pdu['protocol'] == 'message'
                assert pdu['status'] == pdu['completeness'] == 'limited'
                assert pdu['summary'] == 'Packet observation field snapshot budget exceeded'
                continue
            pdu['id'] = one(pdu['id'], path + '/id')
            if 'response_to' in pdu:
                pdu['response_to'] = one(pdu['response_to'], path + '/response_to')
            source(pdu.get('byte_source'), path + '/byte_source')
            snapshot = pdu.get('field_values')
            if snapshot is not None:
                snapshot['pdu'] = one(snapshot['pdu'], path + '/field_values/pdu')
                source(snapshot.get('source'), path + '/field_values/source')
                for value in snapshot.get('values') or []:
                    name = value['name']
                    if name not in ['pdu.id', 'pdu.response_to'] and not (
                        pdu['protocol'] == 'ntp' and name == 'ntp.control.request_pdu'):
                        continue
                    old = value['value']
                    value['value'] = one(old, path + '/' + name)
                    if 'exact_value' in value:
                        assert type(value['exact_value']) is str and value['exact_value'] == str(old)
                        value['exact_value'] = str(value['value'])
            fields = pdu.get('fields') or {}
            for name in ['pdu.id', 'pdu.response_to'] + (['ntp.control.request_pdu'] if pdu['protocol'] == 'ntp' else []):
                if name in fields:
                    assert isinstance(fields[name], list)
                    fields[name] = [one(value, path + '/fields/' + name) for value in fields[name]]
    return out
