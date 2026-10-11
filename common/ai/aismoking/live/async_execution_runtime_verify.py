"""Verify real aim artifacts, independently of the model's final claims.

Usage: python async_execution_runtime_verify.py OUTPUT_DIR
Uses only the standard library; preserves separate control and cleanup verdicts.
"""
import json
import re
import shutil
import sys
from pathlib import Path


def load(path, default=None):
    return json.loads(path.read_text(encoding="utf-8-sig")) if path.exists() else default


def verify(case):
    result = load(case / "result.json", {})
    observations = load(case / "observations.json", [])
    final = load(case / "final-executions.json", [])
    snapshots = load(case / "snapshots.json", [])
    events = load(case / "events.json", [])
    requests = load(case / "requests.json", [])
    responses = load(case / "responses.json", [])
    timeline = (case / "timeline.txt").read_text(encoding="utf-8") if (case / "timeline.txt").exists() else ""
    feedbacks = []
    for line in timeline.splitlines():
        try:
            entry = json.loads(line)
            if isinstance(entry, dict) and entry.get("action") == "manage_async_execution_runtime" and entry.get("status") == "completed":
                feedbacks.append(json.loads(entry["detail"]))
        except (ValueError, KeyError, TypeError):
            pass
    # Text actions feed their result into the next prompt, while native actions
    # also archive it as FUNCTION_CALL_ACTION_RESULT in Timeline.
    for request in requests:
        prompt = request.get("prompt", "")
        try:
            messages = json.loads(prompt)
            if isinstance(messages, list):
                prompt = "\n".join(m.get("content", "") for m in messages if isinstance(m, dict) and isinstance(m.get("content"), str))
        except ValueError:
            pass
        for match in re.finditer(r"<\|FEEDBACK_[^|]+\|>\s*(.*?)\s*<\|FEEDBACK_END_[^|]+\|>", prompt, re.S):
            try:
                feedback = json.loads(match[1])
                if isinstance(feedback, dict) and "op" in feedback:
                    feedbacks.append(feedback)
            except ValueError:
                pass
    actions = []
    seen = set()
    action_dir = case / "actions"
    action_dir.mkdir(exist_ok=True)
    for event in events:
        if event["type"] != "filesystem_pin_filename":
            continue
        pin = json.loads(event["content"])
        path = Path(pin["path"])
        if "action_calls" not in str(path) or path in seen or not path.exists():
            continue
        seen.add(path)
        text = path.read_text(encoding="utf-8")
        name = re.search(r"- Name: (\S+)", text)
        params = re.search(r"```json\s*(.*?)\s*```", text, re.S)
        if name and params:
            actions.append({"name": name[1], "params": json.loads(params[1]), "record": path.name})
            shutil.copyfile(path, action_dir / path.name)
    checks = []

    def check(name, passed):
        checks.append({"check": name, "pass": bool(passed)})

    check("runtime_returned_without_error", result.get("runtime_error") == "<nil>")
    check("real_tool_observations_available", bool(observations))
    check("unique_execution_ids", len({row["execute_id"] for row in final}) == len(final))
    check("all_executions_terminal", bool(final) and all(row["state"] in {"completed", "failed", "cancelled", "timed_out"} for row in final))
    check("inspector_and_join_succeeded", bool(observations) and all(row["wait_error"] == "<nil>" and row["inspect_error"] == "<nil>" for row in observations))
    main_prompts = [r["prompt"] for r in requests if "prompt" in r and r.get("caller", "").startswith("react-loop")]
    if main_prompts:
        check("auto_context_yaml_visible_to_model", any(re.search(r"<\|ASYNC_EXECUTIONS_.*?executions:\s+.*?execute_id:", p, re.S) for p in main_prompts))
    if responses:
        check("provider_response_parse_clean", all(not r.get("parse_errors") for r in responses))
    if result.get("native") and responses:
        check("native_management_action_called", any(c["name"] == "manage_async_execution_runtime" for r in responses for c in r.get("tool_calls", [])))
    by_label = {}
    for row in observations:
        execution = row["execution"]
        stdout = execution.get("observation", {}).get("stdout", "")
        match = re.search(r"MONITOR_START label=(\S+)", stdout)
        if match:
            label = match[1]
            by_label[label] = execution
            others = set(re.findall(r"MONITOR_(?:START|SAMPLE|CLEANED) label=(\S+)", stdout))
            check(f"stdout_isolated:{label}", others == {label})
        else:
            # Accepted calls can still be waiting at the original invocation
            # gate when cancelled. They have a receipt but never entered Yak.
            label = execution.get("metadata", {}).get("attributes", {}).get("identifier")
            if label:
                by_label[label] = execution
    ops = [a["params"].get("op", "list") for a in actions if a["name"] == "manage_async_execution_runtime"]
    groups = [a["params"]["directly_call_tool_params_group"] for a in actions if "directly_call_tool_params_group" in a["params"]]

    def state(label, expected):
        check(f"state:{label}={expected}", by_label.get(label, {}).get("state") == expected)

    def completed(label):
        state(label, "completed")
        result = by_label.get(label, {}).get("observation", {}).get("result", {})
        # The original ToolCaller inspector exposes ToolResult, whose data is
        # the production rendered result rather than the raw Yak RESULT object.
        data = result.get("data", "")
        raw_result = None
        if isinstance(data, str) and data.startswith("RESULT:\n"):
            try:
                raw_result = json.loads(data.split("\n\nOBSERVATIONS:", 1)[0][len("RESULT:\n"):])
            except ValueError:
                pass
        check(f"structured_result:{label}", result.get("success") is True and raw_result is not None and raw_result.get("label") == label and raw_result.get("samples") == 2)

    if case.name == "single":
        state("single_hold", "cancelled")
        completed("single_complete")
        check("sync_default_did_not_register_async", "sync_default" not in by_label and (case / "sync_default.cleaned").exists())
        check("all_single_management_ops", set(["list", "inspect", "wait", "stop"]).issubset(ops))
        check("zero_timeout_wait_requested", any(a["name"] == "manage_async_execution_runtime" and a["params"].get("op") == "wait" and a["params"].get("timeout_seconds") == 0 for a in actions))
        check("wait_timeout_did_not_cancel", any(f.get("op") == "wait" and f.get("wait_timed_out") is True and f.get("execution", {}).get("state") == "running" for f in feedbacks))
        check("unknown_id_inspected", any(a["params"].get("execute_id") == "exec-does-not-exist" for a in actions))
    elif case.name == "mixed":
        state("mixed_a", "cancelled")
        state("mixed_b", "cancelled")
        completed("after_stop_all")
        check("mixed_group_flags", any(len(g) == 3 and [x.get("async", False) for x in g] == [True, False, True] for g in groups))
        check("sync_batch_member_executed", (case / "mixed_sync.cleaned").exists() and "mixed_sync" not in by_label)
        a_id = by_label.get("mixed_a", {}).get("execute_id")
        b_id = by_label.get("mixed_b", {}).get("execute_id")
        isolated = False
        for sample in snapshots:
            states = {e["execute_id"]: e["state"] for e in sample["executions"]}
            if states.get(a_id) == "cancelled" and states.get(b_id) == "running":
                isolated = True
        check("stopping_one_preserved_other", isolated)
        check("stop_all_invoked", "stop_all" in ops)
    elif case.name == "capacity_failure":
        caps = [e for label, e in by_label.items() if label.startswith("cap_")]
        check("four_accepted_at_capacity", len(caps) == 4 and len(final) == 5)
        check("five_async_members_submitted", any(len(g) == 5 and all(x.get("async") is True for x in g) for g in groups))
        accepted = {label for label in by_label if label.startswith("cap_")}
        started = {path.stem for path in case.glob("cap_*.started")}
        check("rejected_member_did_not_execute", len(accepted) == 4 and started.issubset(accepted))
        check("capacity_rejection_reported", "capacity" in timeline.lower())
        check("accepted_capacity_members_cancelled", len(caps) == 4 and all(e["state"] == "cancelled" for e in caps))
        state("expected_failure", "failed")
        failed = by_label.get("expected_failure", {})
        check("failure_stderr_observed", "EXPECTED_MONITOR_FAILURE_expected_failure" in failed.get("observation", {}).get("stderr", ""))
    elif case.name == "owner_release":
        state("owner_hold", "cancelled")
        check("owner_cleanup_not_forced_by_verifier", "stop" not in ops and "stop_all" not in ops)
        check("owner_task_ended", any(a["name"] == "finish" for a in actions))
        check("running_output_observed", "MONITOR_SAMPLE label=owner_hold" in by_label.get("owner_hold", {}).get("observation", {}).get("stdout", ""))
    cleanup = [{"label": label, "state": row["state"], "callback_started": (case / f"{label}.started").exists(), "defer_ran": (case / f"{label}.cleaned").exists()} for label, row in by_label.items()]
    profiles = [json.loads(e["content"]) for e in events if e["type"] == "prompt_profile"]
    status = {"case": case.name, "control_pass": all(c["pass"] for c in checks), "cleanup_pass": bool(cleanup) and all(not c["callback_started"] or c["defer_ran"] for c in cleanup),
              "checks": checks, "cleanup": cleanup, "management_ops": ops, "action_count": len(actions),
              "main_prompt_tokens": [p.get("prompt_tokens") for p in profiles], "result": result}
    (case / "verification.json").write_text(json.dumps(status, ensure_ascii=False, indent=2), encoding="utf-8")
    return status


def main():
    root = Path(sys.argv[1])
    results = [verify(case) for case in sorted(root.iterdir()) if case.is_dir() and (case / "result.json").exists()]
    (root / "verification.json").write_text(json.dumps(results, ensure_ascii=False, indent=2), encoding="utf-8")
    for result in results:
        failures = [c["check"] for c in result["checks"] if not c["pass"]]
        print(f'{result["case"]}: control={result["control_pass"]} cleanup={result["cleanup_pass"]} failures={failures}')
    # Both independent verdicts matter: do not turn missing cleanup into a pass.
    return 0 if results and all(r["control_pass"] and r["cleanup_pass"] for r in results) else 1


if __name__ == "__main__":
    sys.exit(main())
