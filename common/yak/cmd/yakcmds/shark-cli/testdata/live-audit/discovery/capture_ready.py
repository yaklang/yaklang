"""Confirm Npcap has captured a marker before starting application traffic."""
import queue, secrets, socket, subprocess, threading, time

def start_capture(tshark, output, capture_filter, duration):
    marker = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
    marker.bind(('127.0.0.1', 0))
    port = marker.getsockname()[1]
    cap = subprocess.Popen([
        str(tshark), '-n', '-i', r'\Device\NPF_Loopback',
        '-f', '(' + capture_filter + f') or udp port {port}',
        '-F', 'pcap', '-w', str(output), '-a', f'duration:{duration}',
        '-l', '-P', '-T', 'fields', '-e', 'frame.number',
    ], stdout=subprocess.PIPE, stderr=subprocess.PIPE, creationflags=0x08000000)
    ready = queue.Queue()
    threading.Thread(target=lambda: ready.put(cap.stdout.readline()), daemon=True).start()
    deadline = time.monotonic() + 10
    payload = b'capture-ready:' + secrets.token_hex(12).encode()
    try:
        while time.monotonic() < deadline:
            marker.sendto(payload, ('127.0.0.1', port))
            try:
                line = ready.get(timeout=.1)
                assert line.strip().isdigit(), ('capture stopped before readiness', line)
                return cap, port
            except queue.Empty:
                assert cap.poll() is None, 'capture exited during startup'
        raise TimeoutError('Npcap did not capture readiness marker')
    except Exception:
        cap.terminate()
        cap.communicate(timeout=5)
        raise
    finally:
        marker.close()
