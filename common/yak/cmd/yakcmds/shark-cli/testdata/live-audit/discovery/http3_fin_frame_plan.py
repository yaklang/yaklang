"""Declare clean-FIN truncated-frame controls for independent raw UDP/QUIC peers.
These peers deliberately do not enforce HTTP endpoint semantics. Captures and
full peer receipts stay outside Git. Usage: http3_fin_frame_plan.py OUTPUT_JSON
"""
import json,sys
from pathlib import Path
out=Path(sys.argv[1]);assert not any((p/'.git').exists()for p in[out.resolve(),*out.resolve().parents])
cases=[]
def case(name,direction=None,**wire):
 p={'name':name,'request_headers':[[':method','POST'],[':scheme','https'],[':path','/'],[':authority','field-audit.invalid']],'request_data':None,'reply_headers':[[':status','200']],'reply_data':None,'request_expect':'frame-error'if direction=='request'else'base','reply_expect':'frame-error'if direction=='reply'else'base'};p.update(wire);cases.append(p)
case('valid-before',request_data='abc',reply_data='def')
for direction in['request','reply']:
 for name,suffix in[('truncated-type','40'),('truncated-length','0040'),('truncated-payload','000361')]:case(direction+'-'+name,direction,**{direction+'_append_hex':suffix})
 case(direction+'-first-HEADERS-truncated',direction,**{direction+'_wire_prefix_bytes':4})
 case(direction+'-large-DATA-truncated',direction,**{direction+'_data':'x'*18000,direction+'_trim_bytes':1})
case('valid-after',request_data='abc',reply_data='def')
out.parent.mkdir(parents=True,exist_ok=True);out.write_text(json.dumps({'scope':__doc__,'cases':cases},indent=2)+'\n',encoding='utf8');print('Declared',len(cases),'framing/FIN scenarios')
