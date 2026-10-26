"""Write declared HTTP3 body/FIN controls for the independent raw QUIC peers.
The output manifest is an external experiment input, never a repository capture.
Usage: http3_body_plan.py OUTPUT_JSON
"""
import json,sys
from pathlib import Path
out=Path(sys.argv[1]);assert not any((p/'.git').exists() for p in [out.resolve(),*out.resolve().parents])
cases=[]
def case(name,method='POST',ql=None,qdata='',status='200',rl=None,rdata='',qexpect='base',rexpect='base',interim=None,trailers=None):
 q=[[':method',method],[':scheme','https'],[':path','/'],[':authority','field-audit.invalid']]
 if method=='CONNECT':q=[[':method','CONNECT'],[':authority','example.com:443']]
 if ql is not None:
  for value in (ql if isinstance(ql,list) else [ql]):q.append(['content-length',value])
 r=[[':status',status]]
 if rl is not None:r.append(['content-length',rl])
 p={'name':name,'request_headers':q,'request_data':qdata,'request_expect':qexpect,'reply_headers':r,'reply_data':rdata,'reply_expect':rexpect}
 if interim:p['reply_interim_headers']=interim
 if trailers is not None:p['request_trailers']=trailers
 cases.append(p)
case('request-exact',ql='4',qdata='data')
case('request-short-at-FIN',ql='5',qdata='data',qexpect='malformed')
case('request-overrun',ql='3',qdata='data',qexpect='malformed')
case('request-zero-with-data',ql='0',qdata='x',qexpect='malformed')
case('request-huge-decimal',ql='1844674407370955161600000000',qdata='',qexpect='malformed')
case('request-equivalent-lengths',ql=['0004, 4','4'],qdata='data')
case('request-conflicting-lengths',ql=['4','5'],qdata='data',qexpect='malformed')
case('request-nondecimal-length',ql='+4',qdata='data',qexpect='malformed')
case('response-exact',rl='4',rdata='data')
case('response-short-at-FIN',rl='5',rdata='data',rexpect='malformed')
case('response-overrun',rl='3',rdata='data',rexpect='malformed')
case('response-zero-with-data',rl='0',rdata='x',rexpect='malformed')
case('HEAD-representation-length',method='HEAD',rl='17',rdata=None)
case('HEAD-with-content',method='HEAD',rl='1',rdata='x',rexpect='malformed')
case('304-representation-length',status='304',rl='17',rdata=None)
case('304-with-content',status='304',rl='1',rdata='x',rexpect='malformed')
case('204-empty',status='204',rdata=None)
case('204-forbidden-length-zero',status='204',rl='0',rdata=None,rexpect='malformed')
case('204-with-content',status='204',rdata='x',rexpect='malformed')
case('103-FIN-without-final',status='103',rdata=None,rexpect='malformed')
case('103-forbidden-length',status='103',rl='0',rdata=None,rexpect='malformed')
case('103-then-final',rl='4',rdata='data',interim=[[[':status','103']]])
case('CONNECT-success-forbidden-length',method='CONNECT',rl='0',rdata=None,rexpect='malformed')
case('CONNECT-tunnel',method='CONNECT',rdata='opaque-tunnel')
case('CONNECT-rejection-body',method='CONNECT',status='403',rl='4',rdata='data')
case('length-in-trailers',ql='4',qdata='data',trailers=[['content-length','4']],qexpect='malformed')
case('large-body',ql='18000',qdata='x'*18000,rl='18000',rdata='y'*18000)
case('valid-after-errors',ql='4',qdata='data',rl='4',rdata='data')
out.parent.mkdir(parents=True,exist_ok=True);out.write_text(json.dumps({'scope':__doc__,'cases':cases},indent=2)+'\n',encoding='utf8');print('Declared',len(cases),'body/FIN scenarios')
