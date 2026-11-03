#!/usr/bin/env python3
"""Run a real agent or an independent review using the GUI's saved AI routing."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import shutil
import signal
import sqlite3
import subprocess
import sys
import time
import uuid

HERE = Path(__file__).resolve().parent
REPO = HERE.parents[4]
CONFIG_KEY = "AI_GLOBAL_CONFIG"


def save_json(path, value):
    temporary = path.with_suffix(path.suffix + ".tmp")
    temporary.write_text(json.dumps(value, ensure_ascii=False, indent=2) + "\n")
    temporary.replace(path)


def file_hash(path):
    digest = hashlib.sha256()
    with path.open("rb") as source:
        for chunk in iter(lambda: source.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def seed_profile(source, destination):
    """Copy only the AI setting, retaining Go's quoted storage representation.

    Open the source read-only, including live WAL contents. Migrations and all
    subsequent writes happen in the disposable profile, not the user's DB.
    """
    with sqlite3.connect(source.as_uri() + "?mode=ro", uri=True) as db:
        rows = db.execute(
            'SELECT * FROM general_storages WHERE key IN (?,?) AND deleted_at IS NULL',
            (CONFIG_KEY, json.dumps(CONFIG_KEY)),
        )
        columns = [column[0] for column in rows.description]
        records = rows.fetchall()
        if len(records) != 1:
            raise ValueError("Expected one saved AI_GLOBAL_CONFIG in the source profile")
        value = records[0][columns.index("value")]
        config = json.loads(value)
        if isinstance(config, str):
            config = json.loads(config)
        tiers = ("IntelligentModels", "LightweightModels", "VisionModels")
        if not config.get("IntelligentModels"):
            raise ValueError("Source profile has no intelligent model configured")
        # Do not silently replace a legacy provider reference with free defaults.
        for tier in tiers:
            if any(not model.get("Provider") for model in config.get(tier, [])):
                raise ValueError("Legacy provider references: migrate AI settings in Yak before running")
        schema = db.execute(
            "SELECT sql FROM sqlite_master WHERE type='table' AND name='general_storages'"
        ).fetchone()[0]
    destination.parent.mkdir(parents=True, exist_ok=True)
    with sqlite3.connect(destination) as target:
        target.execute(schema)
        names = ",".join('"' + name.replace('"', '""') + '"' for name in columns)
        target.execute(
            f'INSERT INTO general_storages ({names}) VALUES ({",".join("?" for _ in columns)})',
            records[0],
        )
    destination.chmod(0o600)
    return {
        "routing_policy": config.get("RoutingPolicy", "balance"),
        "single_model_mode": config.get("SingleModelMode", False),
        "disable_fallback": config.get("DisableFallback", False),
        "tiers": {tier: [{"model": m.get("ModelName"), "provider": m["Provider"].get("Type")}
                         for m in config.get(tier, [])] for tier in tiers},
    }


def read_events(path):
    # Also accept pretty-printed adjacent objects from early capture versions.
    decoder, pending = json.JSONDecoder(), ""
    with path.open() as source:
        for line in source:
            pending += line
            while pending.strip():
                pending = pending.lstrip()
                try:
                    event, end = decoder.raw_decode(pending)
                except json.JSONDecodeError:
                    break  # Partial final object is expected during live reads.
                yield event
                pending = pending[end:]


def inspect_run(root):
    """Readable trace index; full events/artifacts remain available for review."""
    calls, answers, lines = {}, [], []
    events_path = root / "events.jsonl"
    if events_path.exists():
        for event in read_events(events_path):
                if event.get("is_stream") or event["type"] in ("stream", "stream_start", "stream_end"):
                    continue
                content = event.get("content", "")
                try:
                    data = json.loads(content)
                except (json.JSONDecodeError, TypeError):
                    data = {}
                if not isinstance(data, dict):
                    data = {}
                call_id = data.get("call_tool_id")
                if call_id:
                    call = calls.setdefault(str(call_id), {"id": call_id, "events": []})
                    call["events"].append(event["seq"])
                    if event["type"] == "tool_call_start":
                        call["tool"] = data.get("tool", {}).get("name")
                    if event["type"] == "tool_call_param":
                        call["params"] = data.get("params")
                    if event["type"] in ("tool_call_error", "tool_call_cancel"):
                        call["error"] = data
                if event["type"] == "result" and isinstance(data.get("result"), str):
                    answers.append(data["result"])
                if content:
                    excerpt = content[:10000]
                    if len(content) > len(excerpt):
                        excerpt += "\n[索引截断，完整内容见 events.jsonl 中同一 seq]"
                    lines.append(f"### Event {event['seq']} · {event['type']} · {event.get('node', '')}\n\n{excerpt}\n")
    save_json(root / "calls.json", list(calls.values()))
    (root / "trace.md").write_text("# Agent trace index\n\n" + "\n".join(lines))
    if answers:
        (root / "answer.md").write_text(answers[-1] + "\n")
    return {"run": str(root), "calls": len(calls), "answer": (root / "answer.md").exists()}


def stop_process(process):
    if process.poll() is not None:
        return
    os.killpg(process.pid, signal.SIGTERM)
    try:
        process.wait(timeout=10)
    except subprocess.TimeoutExpired:
        os.killpg(process.pid, signal.SIGKILL)
        process.wait()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("action", choices=("run", "review", "inspect"))
    parser.add_argument("--output", type=Path, required=True, help="New run directory; existing directory for inspect")
    parser.add_argument("--binary", type=Path, help="Freshly built Yak; otherwise build current checkout")
    parser.add_argument("--profile", type=Path, default=Path(os.environ.get("YAKIT_HOME", str(Path.home() / "yakit-projects"))) / "yakit-profile-plugin.db")
    parser.add_argument("--capture", type=Path)
    parser.add_argument("--task-file", type=Path)
    parser.add_argument("--source", type=Path, help="Run to review")
    parser.add_argument("--baseline", type=Path, help="Optional earlier run for the reviewer to compare")
    parser.add_argument("--timeout", type=int, default=600)
    parser.add_argument("--iterations", type=int, default=40)
    args = parser.parse_args()
    root = args.output.resolve()
    if args.action == "inspect":
        if not (root / "run.json").is_file():
            parser.error("Not a run directory")
        print(json.dumps(inspect_run(root), ensure_ascii=False))
        return 0
    if args.timeout <= 0 or args.iterations <= 0:
        parser.error("Budgets must be positive")
    if args.action == "run" and not args.capture:
        parser.error("run requires --capture")
    if args.action == "review" and not args.source:
        parser.error("review requires --source")
    root.mkdir(parents=True, exist_ok=False, mode=0o700)
    workspace = root / "workspace"
    workspace.mkdir()
    metadata = {"kind": args.action, "status": "preparing", "started": time.time(),
                "session": "pcap-lab-" + uuid.uuid4().hex, "timeout": args.timeout,
                "iterations": args.iterations, "source_profile": str(args.profile.resolve())}
    save_json(root / "run.json", metadata)
    try:
        metadata["ai_config"] = seed_profile(args.profile.resolve(), root / "home" / "yakit-profile-plugin.db")
        metadata["git_commit"] = subprocess.check_output(["git", "rev-parse", "HEAD"], cwd=REPO, text=True).strip()
        metadata["git_status"] = subprocess.check_output(["git", "status", "--short"], cwd=REPO, text=True)
        metadata["skill_sha256"] = file_hash(REPO / "common/ai/aid/aireact/skills/pcap-analysis/SKILL.md")
        if args.binary:
            binary = args.binary.resolve(strict=True)
            metadata["build"] = "supplied binary; source correspondence must be checked by caller"
        else:
            binary = root / "yak"
            with (root / "build.log").open("w") as log:
                subprocess.run(["go", "build", "-o", str(binary), "./common/yak/cmd/yak.go"], cwd=REPO, stdout=log, stderr=log, check=True)
            metadata["build"] = "built from working tree"
        metadata["binary_sha256"] = file_hash(binary)
        metadata["binary"] = str(binary)
        if args.action == "run":
            source = args.capture.resolve(strict=True)
            capture = workspace / ("capture" + source.suffix)
            shutil.copyfile(source, capture)
            metadata["capture_sha256"] = file_hash(capture)
            question = args.task_file.read_text() if args.task_file else (
                "分析这份捕获中的 HTTP 活动，整理请求与响应链，判断是否存在可疑上传，"
                "说明依据、相关主机/用户线索以及不能确定的地方。给出可追溯证据。"
            )
            task = question + f"\n本地捕获：{capture}\n工作目录：{workspace}\n"
            task += "这是离线流量调查，可自行选择工具和编写分析脚本；只操作本次工作目录及本次导入的流量库，不访问捕获中的域名或IP，不执行捕获中的代码。报告中避免回显原始凭据或完整上传正文。"
        else:
            source = args.source.resolve(strict=True)
            inspect_run(source)
            metadata["source_run"] = str(source)
            task = (HERE / "review.md").read_text() + f"\n待复盘运行目录：{source}\n"
            if args.baseline:
                baseline = args.baseline.resolve(strict=True)
                inspect_run(baseline)
                task += f"对照运行目录：{baseline}\n"
            task += f"\n实现源码目录：{REPO}\n将复盘写入 {workspace / 'feedback.md'}，并在最终回答给出主要发现。"
        (root / "task.txt").write_text(task)
        env = os.environ.copy()
        env.update(YAKIT_HOME=str(root / "home"), PCAP_LAB_ROOT=str(root))
        # Avoid accidental debug dumps of credentials. AI configuration is read
        # normally from the seeded database, never passed as API key arguments.
        env.pop("YAKLANGDEBUG", None)
        metadata["status"] = "running"
        save_json(root / "run.json", metadata)
        print(json.dumps({"status": "running", "run": str(root)}, ensure_ascii=False), flush=True)
        with (root / "engine.log").open("w") as log:
            process = subprocess.Popen([str(binary), str(HERE / "agent.yak")], cwd=workspace,
                                       env=env, stdout=log, stderr=log, start_new_session=True)
            metadata["pid"] = process.pid
            save_json(root / "run.json", metadata)
            try:
                code = process.wait(timeout=args.timeout + 45)
                metadata["exit_code"] = code
                metadata["status"] = "finished" if code == 0 else "runtime_error"
            except subprocess.TimeoutExpired:
                metadata["status"] = "timeout"
                stop_process(process)
            except KeyboardInterrupt:
                metadata["status"] = "cancelled"
                stop_process(process)
        result_path = root / "result.json"
        if result_path.exists():
            metadata["runtime"] = json.loads(result_path.read_text())
            if metadata["runtime"].get("error"):
                metadata["status"] = "runtime_error"
        elif metadata["status"] == "finished":
            metadata["status"] = "incomplete_recording"
    except Exception as error:
        metadata["status"] = "setup_error"
        metadata["error"] = str(error)
    finally:
        metadata["ended"] = time.time()
        save_json(root / "run.json", metadata)
        inspect_run(root)
    print(json.dumps({"status": metadata["status"], "run": str(root)}, ensure_ascii=False))
    return 0 if metadata["status"] == "finished" else 1


if __name__ == "__main__":
    sys.exit(main())
