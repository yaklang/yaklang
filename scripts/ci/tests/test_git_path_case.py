from pathlib import Path
import subprocess
import sys
import tempfile
import unittest


CHECKER = Path(__file__).resolve().parents[1] / "check_git_path_case.py"


class GitPathCaseTest(unittest.TestCase):
    def setUp(self):
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        self.root = Path(tmp.name)
        self.git("init", "--bare", "--quiet")
        self.blob = self.git("hash-object", "-w", "--stdin", input=b"rule content\n")

    def git(self, *args, input=None):
        return subprocess.check_output(["git", "-C", str(self.root), *args], input=input).strip().decode()

    def tree(self, paths):
        # Construct Git objects without checking them out, so the regression
        # fixtures also work on macOS/Windows case-insensitive filesystems.
        entries = {}
        for path in paths:
            node = entries
            parts = path.split("/")
            for part in parts[:-1]:
                node = node.setdefault(part, {})
            node[parts[-1]] = self.blob

        def write_tree(node):
            records = []
            for name, value in node.items():
                if isinstance(value, dict):
                    records.append(f"040000 tree {write_tree(value)}\t{name}\0")
                else:
                    records.append(f"100644 blob {value}\t{name}\0")
            return self.git("mktree", "-z", input="".join(records).encode())

        return write_tree(entries)

    def check(self, tree):
        return subprocess.run(
            [sys.executable, str(CHECKER), tree], cwd=self.root, text=True, capture_output=True
        )

    def test_distinct_rules_in_case_colliding_parent_directories(self):
        upper = "common/syntaxflow/sfbuildin/buildin/php/cwe-200-Information-Exposure"
        lower = "common/syntaxflow/sfbuildin/buildin/php/cwe-200-information-exposure"
        result = self.check(self.tree([
            upper + "/php-ftp-information-exposure.sf",
            lower + "/source-php-display-errors.sf",
        ]))
        self.assertEqual(result.returncode, 1, result.stdout + result.stderr)
        self.assertIn(upper, result.stderr)
        self.assertIn(lower, result.stderr)

    def test_file_and_file_directory_collisions(self):
        for paths in (["rules/Rule.sf", "rules/rule.sf"], ["Rules", "rules/one.sf"]):
            with self.subTest(paths=paths):
                result = self.check(self.tree(paths))
                self.assertEqual(result.returncode, 1, result.stdout + result.stderr)
                self.assertIn("case-insensitive path collision", result.stderr)

    def test_merged_directory_and_whitespace_paths_are_valid(self):
        result = self.check(self.tree([
            "rules/cwe-200-information-exposure/php-ftp-information-exposure.sf",
            "rules/cwe-200-information-exposure/source-php-display-errors.sf",
            "MixedCase/one.sf",
            "MixedCase/two.sf",
            "space and\nnewline.sf",
        ]))
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)

    def test_invalid_tree_fails_closed(self):
        self.assertNotEqual(self.check("nonexistent-tree").returncode, 0)


if __name__ == "__main__":
    unittest.main()
