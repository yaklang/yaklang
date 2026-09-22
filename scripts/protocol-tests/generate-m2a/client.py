# Generate loopback traffic against real Redis and Kafka services.
import socket, struct, gzip, json, time, argparse
from pathlib import Path
args = argparse.ArgumentParser()
args.add_argument('--output', required=True)
opts = args.parse_args()
out = []

def conn(port):
    return socket.create_connection(('127.0.0.1', port), timeout=5)

def resp(f):
    line = f.readline()
    p = line[:1]
    v = line[1:-2]
    if p in (b'+', b'-', b':', b',', b'#', b'_'):
        return {'type': p.decode(), 'value': v.decode()}
    if p in (b'$', b'!', b'='):
        n = int(v)
        return None if n < 0 else {'type': p.decode(), 'value': f.read(n + 2)[:-2].decode(errors='replace')}
    n = int(v)
    n = n * 2 if p in (b'%', b'|') else n
    return {'type': p.decode(), 'items': [resp(f) for _ in range(n)]}

def command(s, f, *args):
    w = b'*' + str(len(args)).encode() + b'\r\n' + b''.join((b'$' + str(len(a.encode())).encode() + b'\r\n' + a.encode() + b'\r\n' for a in args))
    s.sendall(w)
    r = resp(f)
    out.append({'protocol': 'redis', 'command': args[0], 'result': r})
    return r
s = conn(16379)
f = s.makefile('rb')
for cmd in [('HELLO', '3'), ('AUTH', 'm2-test-only'), ('SET', 'm2:key', 'value'), ('GET', 'm2:key'), ('MULTI',), ('SET', 'm2:txn', 'yes'), ('GET', 'm2:txn'), ('EXEC',), ('MULTI',), ('DISCARD',)]:
    command(s, f, *cmd)
pub = conn(16379)
pf = pub.makefile('rb')
command(pub, pf, 'HELLO', '3')
command(pub, pf, 'SUBSCRIBE', 'm2:channel')
command(s, f, 'PUBLISH', 'm2:channel', 'native-message')
out.append({'protocol': 'redis', 'push': resp(pf)})
command(pub, pf, 'UNSUBSCRIBE', 'm2:channel')
pub.close()
command(s, f, 'CLIENT', 'TRACKING', 'ON', 'BCAST')
other = conn(16379)
of = other.makefile('rb')
command(other, of, 'SET', 'm2:invalidate', '1')
out.append({'protocol': 'redis', 'push': resp(f)})
other.close()
s.close()
I = lambda x: struct.pack('>i', x)
H = lambda x: struct.pack('>h', x)
L = lambda x: struct.pack('>q', x)
S = lambda s: H(len(s.encode())) + s.encode()

def vi(x):
    x = x << 1 ^ x >> 63
    b = b''
    while x > 127:
        b += bytes([x & 127 | 128])
        x >>= 7
    return b + bytes([x])

def crc32c(b):
    c = 4294967295
    for x in b:
        c ^= x
        for _ in range(8):
            c = c >> 1 ^ (2197175160 if c & 1 else 0)
    return c ^ 4294967295

def batch(value, compress):
    v = value.encode()
    rec = b'\x00' + vi(0) + vi(0) + vi(-1) + vi(len(v)) + v + vi(0)
    records = vi(len(rec)) + rec
    if compress:
        records = gzip.compress(records, mtime=0)
    tail = H(compress) + I(0) + L(0) + L(0) + L(-1) + H(-1) + I(-1) + I(1) + records
    body = I(-1) + b'\x02' + struct.pack('>I', crc32c(tail)) + tail
    return L(0) + I(len(body)) + body

def readn(s, n):
    b = b''
    while len(b) < n:
        x = s.recv(n - len(b))
        if not x:
            raise RuntimeError('EOF')
        b += x
    return b
s = conn(19092)
corr = 0

def exchange(api, v, body, expected=True):
    global corr
    corr += 1
    req = H(api) + H(v) + I(corr) + S('m2-native-client') + body
    s.sendall(I(len(req)) + req)
    rsp = b''
    if expected:
        n = struct.unpack('>i', readn(s, 4))[0]
        rsp = readn(s, n)
        assert rsp[:4] == I(corr)
    out.append({'protocol': 'kafka', 'api': api, 'version': v, 'correlation': corr, 'request_hex': (I(len(req)) + req).hex(), 'response_hex': (I(len(rsp)) + rsp).hex() if expected else None})
    return rsp[4:]
for v in range(3):
    assert exchange(18, v, b'')[:2] == H(0)
for v in range(9):
    exchange(3, v, I(1) + S('m2-native') + (b'\x01' if v >= 4 else b'') + (b'\x00\x00' if v >= 8 else b''))
for v in range(3, 8):
    for comp in (0, 1):
        b = batch('m2-v%d-gzip%d' % (v, comp), comp)
        body = H(-1) + H(1) + I(5000) + I(1) + S('m2-native') + I(1) + I(0) + I(len(b)) + b
        response = exchange(0, v, body)
        assert response[4 + 2 + len('m2-native') + 4 + 4:][:2] == H(0), response.hex()
for v in range(4, 12):
    body = I(-1) + I(500) + I(1) + I(1048576) + b'\x00' + (I(0) + I(-1) if v >= 7 else b'') + I(1) + S('m2-native') + I(1) + I(0) + (I(-1) if v >= 9 else b'') + L(0) + (L(-1) if v >= 5 else b'') + I(1048576) + (I(0) if v >= 7 else b'') + (S('') if v >= 11 else b'')
    exchange(1, v, body)
b = batch('m2-noack', 0)
exchange(0, 3, H(-1) + H(0) + I(5000) + I(1) + S('m2-native') + I(1) + I(0) + I(len(b)) + b, False)
exchange(18, 2, b'')
s.close()
Path(opts.output).write_text(json.dumps(out, indent=2) + '\n')
print('native interactions', len(out))
