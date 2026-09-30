#!/usr/bin/env python3
"""Real socket exchanges using stomp.py 8.2.0, against RabbitMQ 4.1.0.
This client oracle comes from stomp.py callbacks, independently of Yaklang.
"""
import argparse, base64, json, queue, time
import stomp

class Oracle(stomp.ConnectionListener):
    def __init__(self):
        self.events=[]; self.messages=queue.Queue(); self.receipts=queue.Queue()
    def record(self, direction, frame):
        body=frame.body.encode() if isinstance(frame.body,str) else bytes(frame.body or b'')
        self.events.append(dict(direction=direction, command=frame.cmd,headers=dict(frame.headers),body_base64=base64.b64encode(body).decode()))
    def on_send(self,frame): self.record('client',frame)
    def on_connected(self,frame): self.record('server',frame)
    def on_message(self,frame): self.record('server',frame); self.messages.put(frame)
    def on_receipt(self,frame): self.record('server',frame);self.receipts.put(frame.headers['receipt-id'])
    def on_error(self,frame): self.record('server',frame);raise RuntimeError(frame.body)
    def wait_receipt(self,want):
        got=self.receipts.get(timeout=10)
        assert got==want,(got,want)

def main():
    p=argparse.ArgumentParser();p.add_argument('--version',choices=['1.0','1.1','1.2'],required=True);p.add_argument('--out',required=True);p.add_argument('--port',type=int,default=46163);args=p.parse_args()
    assert stomp.__version__=='8.2.0'
    cls={'1.0':stomp.Connection10,'1.1':stomp.Connection11,'1.2':stomp.Connection12}[args.version]
    conn=cls([('127.0.0.1',args.port)],auto_decode=False,**({} if args.version=='1.0' else {'vhost':'/','heartbeats':(0,0)}))
    oracle=Oracle();conn.set_listener('oracle',oracle);conn.connect('fixture','fixture',wait=True)
    destination='/queue/pr5013-'+args.version.replace('.','')
    mode='client' if args.version=='1.0' else 'client-individual'
    conn.subscribe(destination,id='sub-1',ack=mode,headers={'receipt':'subscribed','auto-delete':'true'})
    oracle.wait_receipt('subscribed')
    # Commit publishes a binary body; abort must never deliver its body.
    conn.begin(transaction='aborted');conn.send(destination,b'never-deliver',headers={'transaction':'aborted'});conn.abort(transaction='aborted')
    conn.begin(transaction='committed')
    body=b'fixture\x00body' if args.version=='1.2' else b'fixture-body'
    label='raw-colon-free' if args.version=='1.0' else 'escaped:colon\nnewline'
    conn.send(destination,body,headers={'transaction':'committed','x-variant':label,'content-type':'application/octet-stream'})
    conn.commit(transaction='committed',headers={'receipt':'committed'})
    oracle.wait_receipt('committed');message=oracle.messages.get(timeout=10)
    assert message.body==body,(message.body,body)
    assert message.headers['x-variant']==label,message.headers
    if args.version=='1.1':conn.ack(message.headers['message-id'],'sub-1',receipt='acked')
    else:conn.ack(message.headers.get('ack',message.headers['message-id']),receipt='acked')
    oracle.wait_receipt('acked')
    if args.version!='1.0':
        conn.send(destination,b'nack-and-discard',headers={'receipt':'sent-nack'})
        oracle.wait_receipt('sent-nack');message=oracle.messages.get(timeout=10)
        if args.version=='1.1':conn.nack(message.headers['message-id'],'sub-1',receipt='nacked',requeue='false')
        else:conn.nack(message.headers['ack'],receipt='nacked',requeue='false')
        oracle.wait_receipt('nacked')
    conn.unsubscribe(id='sub-1',headers={'receipt':'unsubscribed'});oracle.wait_receipt('unsubscribed')
    conn.disconnect(receipt='disconnected');time.sleep(.2)
    assert oracle.messages.empty(),'ABORT unexpectedly delivered a message'
    with open(args.out,'w') as f:json.dump(dict(client='stomp.py',client_version=stomp.__version__,stomp_version=args.version,assertions={'body_roundtrip':True,'header_roundtrip':True,'abort_not_delivered':True},events=oracle.events),f,indent=2);f.write('\n')

if __name__=='__main__':main()
