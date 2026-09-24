"""Live Redis 7.2.5 oracle; capture loopback tcp port 19679 around this script."""
import socket
import time

def wire(*args):
    return ('*%d\r\n' % len(args) + ''.join('$%d\r\n%s\r\n' % (len(x), x) for x in args)).encode()

def run(commands, expected):
    with socket.create_connection(('127.0.0.1', 19679), timeout=3) as s:
        s.sendall(b''.join(wire(*c) for c in commands))
        out = b''
        while len(out) < len(expected):
            chunk = s.recv(4096)
            assert chunk
            out += chunk
        assert out == expected, (out, expected)
        s.settimeout(.1)
        try:
            extra = s.recv(4096)
            raise AssertionError(('unexpected reply', extra))
        except socket.timeout:
            pass
        print(commands, repr(out), flush=True)

run([('CLIENT','REPLY','OFF'), ('PING',), ('CLIENT','REPLY','ON'), ('GET','missing')], b'+OK\r\n$-1\r\n')
run([('CLIENT','REPLY','SKIP'), ('PING',), ('GET','missing')], b'$-1\r\n')
run([('CLIENT','REPLY','SKIP'), ('CLIENT','REPLY','SKIP'), ('PING',), ('PING',)], b'+PONG\r\n')
run([('CLIENT','REPLY','SKIP'), ('CLIENT','REPLY','ON'), ('PING',)], b'+OK\r\n+PONG\r\n')
run([('PING',), ('CLIENT','REPLY','OFF'), ('PING',), ('CLIENT','REPLY','ON')], b'+PONG\r\n+OK\r\n')
# Restricted user proves CLIENT REPLY can be denied before changing state.
run([('ACL','SETUSER','no-reply','on','nopass','~*','+@all','-client|reply')], b'+OK\r\n')
run([('AUTH','no-reply',''), ('CLIENT','REPLY','OFF'), ('PING',)], b"+OK\r\n-NOPERM User no-reply has no permissions to run the 'client|reply' command\r\n+PONG\r\n")
time.sleep(.1)
