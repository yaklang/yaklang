"""Exact old public-SDK reference paths, independently inventoried and sourced.

This helper neither discovers IDs from their integer values nor establishes
inserted-ESP correctness. A caller must build one injective old/new event-ID
mapping from the unchanged non-ESP event order and separately source-prove new
ESP rows. Everything outside these explicit paths stays fully literal.
"""
import copy


def remap_known_pdu_references(event, mapping):
    out = copy.deepcopy(event)

    def one(value, path):
        assert type(value) is int and value >= 0, (path, 'not a nonnegative integer')
        if value == 0:
            return 0
        assert value in mapping, (path, 'dangling old generated PDU reference', value)
        result = mapping[value]
        assert type(result) is int and result > 0, (path, 'invalid mapped PDU ID')
        return result

    def byte_source(value, path):
        if value is None:
            return
        assert isinstance(value, dict), path
        if 'ParentPDU' in value:
            value['ParentPDU'] = one(value['ParentPDU'], path + '/ParentPDU')
        if value.get('ParentPDUs') is not None:
            assert isinstance(value['ParentPDUs'], list), path
            value['ParentPDUs'] = [one(v, path + '/ParentPDUs') for v in value['ParentPDUs']]
        # PacketRefs[].Number and CompletedIn.Number are physical frame IDs.

    out['id'] = one(out['id'], 'id')
    assert out['id'] > 0
    if 'response_to' in out:
        out['response_to'] = one(out['response_to'], 'response_to')
    byte_source(out.get('source_bytes'), 'source_bytes')
    snapshot = out.get('field_values')
    if snapshot is not None:
        snapshot['pdu'] = one(snapshot['pdu'], 'field_values/pdu')
        byte_source(snapshot.get('source'), 'field_values/source')
        for index, item in enumerate(snapshot.get('values') or []):
            name = item['name']
            allowed = name in ['pdu.id', 'pdu.response_to'] or (
                out['protocol'] == 'ntp' and name == 'ntp.control.request_pdu')
            if not allowed:
                continue
            old = item['value']
            mapped = one(old, 'field_values/values/' + str(index) + '/' + name)
            if 'exact_value' in item:
                assert type(item['exact_value']) is str and item['exact_value'] == str(old)
                item['exact_value'] = str(mapped)
            item['value'] = mapped

    for root in ['fields', 'session']:
        values = out.get(root)
        if not isinstance(values, dict):
            continue
        if out['protocol'] == 'rdpudp' and 'Original Carrier PDU' in values:
            values['Original Carrier PDU'] = one(values['Original Carrier PDU'], root + '/Original Carrier PDU')
        if out['protocol'] != 'ntp':
            continue
        if 'Request PDU ID' in values:
            values['Request PDU ID'] = one(values['Request PDU ID'], root + '/Request PDU ID')
        reassembled = values.get('Reassembled Byte Source')
        if not isinstance(reassembled, dict):
            continue
        if reassembled.get('Parent PDUs') is not None:
            assert isinstance(reassembled['Parent PDUs'], list)
            reassembled['Parent PDUs'] = [one(v, root + '/Reassembled Byte Source/Parent PDUs')
                                         for v in reassembled['Parent PDUs']]
        for span in reassembled.get('Ranges') or []:
            if 'Parent PDU' in span:
                span['Parent PDU'] = one(span['Parent PDU'], root + '/Reassembled Byte Source/Ranges/*/Parent PDU')
    return out
