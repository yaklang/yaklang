import importlib.util
import json
from pathlib import Path
import sqlite3
import tempfile
import unittest

spec = importlib.util.spec_from_file_location("pcap_lab", Path(__file__).with_name("run.py"))
lab = importlib.util.module_from_spec(spec)
spec.loader.exec_module(lab)


class LabTests(unittest.TestCase):
    def test_profile_preserves_routing_without_copying_user_data(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            source, target = root / "source.db", root / "private" / "profile.db"
            config = {"SingleModelMode": True, "DisableFallback": True,
                      "IntelligentModels": [{"ModelName": "fixture-model", "Provider": {
                          "Type": "openai", "APIKey": "test-secret", "Proxy": "http://localhost:1234"}}]}
            encoded = json.dumps(json.dumps(config))
            with sqlite3.connect(source) as db:
                db.execute("create table general_storages (id integer primary key, key text, value text, deleted_at text)")
                db.execute("insert into general_storages values (1,?,?,NULL)", (json.dumps(lab.CONFIG_KEY), encoded))
                db.execute("insert into general_storages values (2,'unrelated','user-data',NULL)")
                db.execute("create table private_history (content text)")
            before = lab.file_hash(source)
            summary = lab.seed_profile(source, target)
            self.assertEqual(before, lab.file_hash(source))
            self.assertTrue(summary["single_model_mode"])
            self.assertNotIn("test-secret", json.dumps(summary))
            with sqlite3.connect(target) as db:
                self.assertEqual(db.execute("select value from general_storages").fetchall(), [(encoded,)])
                self.assertEqual(db.execute("select name from sqlite_master where type='table'").fetchall(), [("general_storages",)])

    def test_trace_keeps_recovery_and_handles_interrupted_final_line(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            events = [
                {"type": "tool_call_start", "content": json.dumps({"call_tool_id": "a", "tool": {"name": "exec"}})},
                {"type": "tool_call_param", "content": json.dumps({"call_tool_id": "a", "params": {"command": "example"}})},
                {"type": "tool_call_error", "content": json.dumps({"call_tool_id": "a", "error": "failed"})},
                {"type": "result", "content": json.dumps({"result": "Recovered and answered"})},
            ]
            with (root / "events.jsonl").open("w") as output:
                for seq, event in enumerate(events, 1):
                    output.write(json.dumps(dict(event, seq=seq)) + "\n")
                output.write('{"seq":5,')
            lab.inspect_run(root)
            calls = json.loads((root / "calls.json").read_text())
            self.assertEqual(calls[0]["params"]["command"], "example")
            self.assertEqual(calls[0]["events"], [1, 2, 3])
            self.assertIn("error", calls[0])
            self.assertEqual((root / "answer.md").read_text().strip(), "Recovered and answered")


if __name__ == "__main__":
    unittest.main()
