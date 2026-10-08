import importlib.util
import json
import pathlib
import unittest

spec = importlib.util.spec_from_file_location("normalizer", pathlib.Path(__file__).with_name("normalize-report.py"))
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)


def report(*ids):
    return {"report_type": "irify-full", "RiskNums": len(ids), "Risks": {key: {"hash": key, "severity": "high"} for key in ids}, "Rules": [{"name": "rule"}], "File": [{"path": "main.go"}]}


class NormalizeReportTests(unittest.TestCase):
    def test_single_snapshot_is_unchanged(self):
        original = report("a")
        self.assertEqual(original, module.normalize(json.dumps(original)))

    def test_cumulative_and_disjoint_findings_are_preserved(self):
        text = "\n".join(json.dumps(report(*ids)) for ids in [("a",), ("a", "b"), ("c",)])
        result = module.normalize(text)
        self.assertEqual(3, result["RiskNums"])
        self.assertEqual({"a", "b", "c"}, set(result["Risks"]))
        self.assertEqual(1, len(result["Rules"]))
        self.assertTrue(all(item["severity"] == "high" for item in result["Risks"].values()))

    def test_corrupt_or_conflicting_reports_fail_closed(self):
        modified = report("a")
        modified["Risks"]["a"]["severity"] = "low"
        bad_count = report("a")
        bad_count["RiskNums"] = 0
        for text in ["", "[]", "{}", json.dumps(bad_count), json.dumps(report()) + "oops", json.dumps(report("a")) + json.dumps(modified)]:
            with self.subTest(text=text), self.assertRaises(ValueError):
                module.normalize(text)


if __name__ == "__main__":
    unittest.main()
