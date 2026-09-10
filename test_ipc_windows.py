#!/usr/bin/env python3
"""
Yak IPC 测试脚本 — Windows 版
用法: python test_ipc_windows.py <yak_path>
示例: python test_ipc_windows.py C:\yakit-projects\yak-engine\yak.exe
"""

import subprocess, time, json, re, os, sys, socket, threading

def get_yak():
    if len(sys.argv) < 2:
        for p in [r"C:\yakit-projects\yak-engine\yak.exe", "yak.exe", "yak"]:
            if os.path.exists(p):
                return p
        print("用法: python test_ipc_windows.py <yak_path>")
        sys.exit(1)
    return sys.argv[1]

YAK = get_yak()
results = []

def read_event(proc, timeout=10):
    """阻塞读取 yak grpc ready/failed 事件"""
    lines = []
    def reader():
        for line in proc.stdout:
            lines.append(line.rstrip())
            if 'yak grpc ready' in line or 'yak grpc failed' in line:
                break
    t = threading.Thread(target=reader, daemon=True)
    t.start()
    t.join(timeout=timeout)
    for line in lines:
        if 'yak grpc ready' in line:
            return json.loads(line.replace('yak grpc ready ', ''))
        if 'yak grpc failed' in line:
            return json.loads(line.replace('yak grpc failed ', ''))
    return None

def read_event_after_kill(proc, timeout=3):
    """kill 后读取已缓冲的 stdout"""
    proc.kill()
    proc.wait()
    try:
        stdout = proc.stdout.read()
    except Exception:
        stdout = ""
    for line in stdout.split('\n'):
        if 'yak grpc ready' in line:
            return json.loads(line.replace('yak grpc ready ', ''))
        if 'yak grpc failed' in line:
            return json.loads(line.replace('yak grpc failed ', ''))
    return None

def extract_json(out):
    """从 check-secret 输出中提取 <json-xxx>...</json-xxx>"""
    m = re.search(r'<json-[a-f0-9]+>\n(.*?)\n</json-', out, re.DOTALL)
    return json.loads(m.group(1)) if m else None

def run_check_secret(args, env=None, wait=5):
    """运行 check-secret 并返回 JSON 结果，用 utf-8 编码避免 GBK 解码错误"""
    proc = subprocess.Popen([YAK, 'check-secret-local-grpc'] + args,
        stdout=subprocess.PIPE, stderr=subprocess.PIPE, env=env,
        encoding='utf-8', errors='replace')
    time.sleep(wait)
    proc.kill()
    proc.wait()
    return extract_json(proc.stdout.read())

def spawn(args, env=None):
    """启动子进程，用 utf-8 编码"""
    return subprocess.Popen(args,
        stdout=subprocess.PIPE, stderr=subprocess.PIPE, env=env,
        encoding='utf-8', errors='replace')

def record(name, ok, detail=""):
    results.append((name, ok))
    print(f"  {'PASS' if ok else 'FAIL'}: {name}  {detail}")

# ============================================================
print("=" * 60)
print(f"Yak IPC 测试 — Windows")
print(f"yak: {YAK}")
print("=" * 60)

# ============================================================
# TCP 回归测试
# ============================================================
print("\n--- TCP 回归测试 ---")

# T1: TCP 端口占用
print("\n[T1] yak grpc tcp 端口占用")
s = socket.socket(); s.bind(('127.0.0.1', 19031)); s.listen(1)
proc = spawn([YAK, 'grpc', '--local-password', 't', '--port', '19031'])
time.sleep(3)
d = read_event_after_kill(proc, 3)
s.close()
if d:
    ok = d.get('reasonCode') == 'tcp_bind_in_use'
    record("T1 tcp port in use", ok, f"reasonCode={d.get('reasonCode')}")
else:
    record("T1 tcp port in use", False, "no failed event")

# T2: TCP 数据库错误
print("\n[T2] check-secret tcp 数据库错误")
env_bad = dict(os.environ)
env_bad['YAK_DEFAULT_PROJECT_DATABASE_NAME'] = r'C:\nonexistent\bad.db'
d = run_check_secret(['--port', '19032'], env=env_bad, wait=3)
if d:
    ok = d['ok'] == False and d.get('reasonCode') == 'database_error'
    record("T2 tcp database error", ok, f"reasonCode={d.get('reasonCode')}")
else:
    record("T2 tcp database error", False, "no JSON")

# T3: TCP 成功
print("\n[T3] check-secret tcp 成功")
d = run_check_secret(['--port', '19033'], wait=5)
if d:
    record("T3 tcp success", d['ok'] == True, f"ok={d['ok']}")
else:
    record("T3 tcp success", False, "no JSON")

# ============================================================
# IPC (npipe) 测试
# ============================================================
print("\n--- IPC (npipe) 测试 ---")

PIPE_NAME = "yakit-test-ipc"

# T4: npipe ready 事件
print("\n[T4] yak grpc npipe ready 事件")
proc = spawn([YAK, 'grpc', '--local-password', 't', '--transport', 'npipe', '--socket-path', PIPE_NAME])
d = read_event(proc, 10)
if d:
    ok = d.get('transport') == 'npipe' and 'instanceId' in d
    record("T4 npipe ready", ok, f"transport={d.get('transport')}, address={d.get('address')}")
else:
    d = read_event_after_kill(proc, 3)
    if d:
        ok = d.get('transport') == 'npipe' and 'instanceId' in d
        record("T4 npipe ready", ok, f"transport={d.get('transport')} (after kill)")
    else:
        record("T4 npipe ready", False, "no event")
proc.kill(); proc.wait()

# T5: check-secret npipe 服务器模式
print("\n[T5] check-secret npipe 服务器模式成功")
d = run_check_secret(['--transport', 'npipe', '--socket-path', PIPE_NAME], wait=5)
if d:
    ok = d['ok'] == True and d.get('transport') == 'npipe'
    record("T5 npipe server mode", ok, f"ok={d['ok']}, transport={d.get('transport')}")
else:
    record("T5 npipe server mode", False, "no JSON")

# T6: check-secret npipe 客户端模式
print("\n[T6] check-secret npipe 客户端模式连接")
server = spawn([YAK, 'grpc', '--local-password', 'cpass', '--transport', 'npipe', '--socket-path', PIPE_NAME])
time.sleep(2)
client = spawn([YAK, 'check-secret-local-grpc', '--transport', 'npipe', '--socket-path', PIPE_NAME, '--client-password', 'cpass'])
time.sleep(4); client.kill(); client.wait()
d = extract_json(client.stdout.read())
server.kill(); server.wait()
if d:
    record("T6 npipe client mode", d['ok'] == True, f"ok={d['ok']}, transport={d.get('transport')}")
else:
    record("T6 npipe client mode", False, "no JSON")

# T7: npipe 客户端模式密码错误
print("\n[T7] check-secret npipe 客户端模式密码错误")
server = spawn([YAK, 'grpc', '--local-password', 'rightpass', '--transport', 'npipe', '--socket-path', PIPE_NAME])
time.sleep(2)
client = spawn([YAK, 'check-secret-local-grpc', '--transport', 'npipe', '--socket-path', PIPE_NAME, '--client-password', 'wrongpass'])
time.sleep(5); client.kill(); client.wait()
d = extract_json(client.stdout.read())
server.kill(); server.wait()
if d:
    ok = d['ok'] == False and d.get('reasonCode') == 'version_rpc_failed'
    record("T7 npipe wrong password", ok, f"ok={d['ok']}, reasonCode={d.get('reasonCode')}")
else:
    record("T7 npipe wrong password", False, "no JSON")

# T8: npipe 缺少 socket-path
print("\n[T8] yak grpc npipe 缺少 socket-path")
proc = spawn([YAK, 'grpc', '--local-password', 't', '--transport', 'npipe'])
time.sleep(3)
d = read_event_after_kill(proc, 3)
if d:
    ok = d.get('reasonCode') == 'init_failed'
    record("T8 npipe missing path", ok, f"reasonCode={d.get('reasonCode')}")
else:
    record("T8 npipe missing path", False, "no event")

# ============================================================
# 汇总
# ============================================================
print("\n" + "=" * 60)
print("验证结果汇总")
print("=" * 60)
passed = sum(1 for _, ok in results if ok)
total = len(results)
for name, ok in results:
    print(f"  {'PASS' if ok else 'FAIL'}: {name}")
print(f"\n  {passed}/{total} passed")
if passed == total:
    print("\n  全部通过")
else:
    print(f"\n  {total - passed} 项失败")
