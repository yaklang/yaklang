"""Freeze a ready-before-handshake capture adapter; original peer logic is literal."""
from pathlib import Path
import hashlib,json
repo=Path(r'C:\Users\V\.codex\worktrees\shark-protocol-infrastructure\yaklang')
original=repo/'common/yak/cmd/yakcmds/shark-cli/testdata/live-audit/discovery/quic_epochs.py'
out=Path(__file__).parent/'quic_epochs_ready39.py'
assert not out.exists()
source=original.read_text('utf8').replace('import collections,','import queue,threading\nimport collections,',1)
old=next(line for line in source.splitlines()if line.startswith('capture=subprocess.Popen('))
new='''marker=socket.socket(socket.AF_INET,socket.SOCK_DGRAM);marker.bind(('127.0.0.1',0));marker_port=marker.getsockname()[1]
expression=f'udp port {server_address[1]} or udp port {marker_port}'
capture_args=[sys.argv[2],'-n','-i',r'\\Device\\NPF_Loopback','-f',expression,'-F','pcap','-w',str(out/'live.pcap'),'-a','duration:25','-a','filesize:102400','-l','-P','-T','fields','-e','frame.number']
capture=subprocess.Popen(capture_args,stdout=subprocess.PIPE,stderr=subprocess.PIPE,creationflags=0x08000000)
ready=queue.Queue();threading.Thread(target=lambda:ready.put(capture.stdout.readline()),daemon=True).start()
try:
    deadline=time.monotonic()+10
    while True:
        assert time.monotonic()<deadline and capture.poll()is None,'capture readiness failed'
        marker.sendto(b'quic-epochs-ready:'+secrets.token_hex(8).encode(),('127.0.0.1',marker_port))
        try:
            first=ready.get(timeout=.1);assert first.strip().isdigit(),first
            threading.Thread(target=lambda:capture.stdout.read(),daemon=True).start()
            break
        except queue.Empty:pass
except BaseException:
    capture.terminate();capture.communicate(timeout=5);raise
finally:marker.close()
readiness=dict(marker_port=marker_port,actual_marker_observed=True,capture_arguments=capture_args,maximum_KiB=102400)
'''
assert source.count(old)==1
source=source.replace(old,new)
anchor="time.sleep(1);operations=[];network=collections.Counter();profiles=[]"
assert source.count(anchor)==1;source=source.replace(anchor,"operations=[];network=collections.Counter();profiles=[]")
anchor="dict(aioquic=importlib.metadata.version('aioquic')"
assert source.count(anchor)==1;source=source.replace(anchor,"dict(generator_SHA256=hashlib.sha256(Path(__file__).read_bytes()).hexdigest(),readiness=readiness,aioquic=importlib.metadata.version('aioquic')")
out.write_text(source,encoding='utf8')
print(json.dumps(dict(adapter=str(out),SHA256=hashlib.sha256(out.read_bytes()).hexdigest(),original_SHA256=hashlib.sha256(original.read_bytes()).hexdigest(),peer_protocol_logic_unchanged=True)))
