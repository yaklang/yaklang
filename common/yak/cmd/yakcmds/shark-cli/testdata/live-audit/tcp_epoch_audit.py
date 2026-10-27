"""Partition selected original TCP tuples with fully observed two-way handshakes.

This opt-in certificate requires an opening SYN and matching opposite SYN-ACK
in every epoch. A changed SYN begins a candidate epoch even when FIN/RST was
not captured. Each candidate must then have its own matching two-way handshake.
It does not certify arbitrary midstream, simultaneous-open or stale-packet cases.
Frame bytes, capture domains and endpoint identities are never rewritten.
"""
import json


def connection(packet):
    return json.dumps([packet['domain'],sorted([packet['source'],packet['destination']])],sort_keys=True,separators=(',',':'))


def partition(physical):
    current={};epochs=[];by_frame={}
    for frame,packet in sorted(physical.items()):
        key=connection(packet);state=current.get(key)
        if packet['syn']:
            assert not packet['fin'] and not packet['reset'] and not packet['payload'],'selected SYN control has no payload/close flags'
        if packet['syn'] and not packet['ack']:
            opening=(packet['source'],packet['sequence'])
            if state is None or state['opening']!=opening:
                if state is not None:
                    assert state['SYN_ACK_frame'] is not None,'previous epoch also has a captured two-way handshake'
                state=dict(epoch=len(epochs)+1,connection=key,opening=opening,SYN_frame=frame,SYN_ACK_frame=None,server_ISN=None,frames=[])
                current[key]=state;epochs.append(state)
        assert state is not None,'selected epoch starts with an actual captured opening SYN'
        if packet['syn'] and packet['ack']:
            assert packet['source']!=state['opening'][0] and packet['ack_sequence']==(state['opening'][1]+1)&0xffffffff,'opposite SYN-ACK acknowledges this opening SYN exactly'
            assert state['server_ISN'] in [None,packet['sequence']],'same epoch has one server ISN (or exact SYN-ACK retransmissions)'
            state['server_ISN']=packet['sequence']
            if state['SYN_ACK_frame'] is None:state['SYN_ACK_frame']=frame
        state['frames'].append(frame);by_frame[frame]=state['epoch']
    for state in epochs:
        assert state['SYN_ACK_frame'] is not None,'each selected epoch has its captured opposite SYN-ACK'
    return by_frame,epochs


def bind_events(native,physical,by_frame):
    event_epochs={};flow_epochs={};epoch_flows={}
    for event in native:
        if not event.get('raw'):continue
        refs=(event.get('source_bytes') or {}).get('PacketRefs') or []
        assert refs,'a selected wire PDU has original contributor frames'
        epochs={by_frame[ref['Number']] for ref in refs}
        assert len(epochs)==1,'one native PDU cannot merge bytes from different TCP epochs'
        epoch=epochs.pop();event_epochs[event['id']]=epoch
        for ref in refs:
            assert connection(event)==connection(physical[ref['Number']]),'contributor has the same original tuple and domain'
        flow=(connection(event),event['flow'])
        assert flow not in flow_epochs or flow_epochs[flow]==epoch,'one native flow cannot conflate original TCP epochs'
        assert epoch not in epoch_flows or epoch_flows[epoch]==flow,'one original epoch cannot acquire multiple native flows'
        flow_epochs[flow]=epoch;epoch_flows[epoch]=flow
    for event in native:
        if not event.get('raw'):
            flow=(connection(event),event['flow'])
            assert flow in flow_epochs,'empty closure diagnosis belongs to an observed native flow'
            event_epochs[event['id']]=flow_epochs[flow]
    return event_epochs,flow_epochs
