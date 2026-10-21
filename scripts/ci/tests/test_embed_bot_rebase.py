import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import textwrap
import unittest


WORKFLOW = Path(__file__).resolve().parents[3] / ".github/workflows/update-embed-fs.yml"
HASH = "common/consts/aitool-hash.go"
RESOURCE = "common/coreplugin/base-yak-plugin/fixture.yak"


class EmbedBotRebaseTest(unittest.TestCase):
    def setUp(self):
        self.bash = shutil.which("bash")
        if not self.bash:
            self.skipTest("bash is required to execute the workflow step")
        tmp = tempfile.TemporaryDirectory(prefix="test_embed_bot_")
        self.root = Path(tmp.name).resolve()
        self.assertTrue(self.root.is_relative_to(Path(tempfile.gettempdir()).resolve()))
        self.addCleanup(tmp.cleanup)
        self.env = dict(os.environ, GIT_TERMINAL_PROMPT="0", GIT_EDITOR="true")
        for key in ("GIT_DIR", "GIT_WORK_TREE", "GIT_INDEX_FILE"):
            self.env.pop(key, None)
        self.git("init", "--quiet", "--initial-branch=feature")
        for key, value in (("user.name", "CI fixture"), ("user.email", "ci@example.invalid"),
                           ("commit.gpgsign", "false"), ("core.autocrlf", "false")):
            self.git("config", key, value)
        self.write(HASH, "base hash\n")
        self.write(RESOURCE, "base resource\n")
        self.write("application.go", "base application\n")
        self.commit("base")
        self.git("update-ref", "refs/remotes/origin/main", "HEAD")

    def git(self, *args):
        result = subprocess.run(["git", *args], cwd=self.root, env=self.env,
                                text=True, capture_output=True, timeout=20)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        return result.stdout.strip()

    def write(self, name, content):
        path = self.root / name
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(content, encoding="utf-8")

    def commit(self, title):
        self.git("add", ".")
        self.git("commit", "--quiet", "-m", title)

    def bot_commit(self, application_conflict=False):
        self.write(HASH, "bot hash\n")
        if application_conflict:
            self.write("application.go", "bot application\n")
        self.commit("chore: auto-update embed files [bot commit]")

    def human_commit(self, number, resource_change=True):
        self.write(HASH, f"human hash {number}\n")
        self.write("application.go", f"human application {number}\n")
        if resource_change:
            self.write(RESOURCE, f"human resource {number}\n")
        self.commit(f"human change {number}")

    def run_cleanup(self):
        # Execute the production workflow body, not a copy of its rebase loop.
        step = WORKFLOW.read_text(encoding="utf-8").split(
            "      - name: Check bot commit and resource changes\n", 1)[1]
        step = step.split("\n      - name:", 1)[0]
        script = textwrap.dedent(step.split("        run: |\n", 1)[1])
        env_file = self.root / ".git/github-env"
        result = subprocess.run([self.bash, "-e", "-c", script], cwd=self.root,
                                env=dict(self.env, GITHUB_ENV=env_file.as_posix()),
                                text=True, capture_output=True, timeout=30)
        self.assertFalse((self.root / ".git/rebase-merge").exists(), result.stdout + result.stderr)
        self.assertFalse((self.root / ".git/rebase-apply").exists(), result.stdout + result.stderr)
        self.assertEqual(self.git("status", "--porcelain"), "")
        return result, env_file.read_text() if env_file.exists() else ""

    def test_consecutive_generated_conflicts_preserve_human_changes(self):
        self.bot_commit()
        self.human_commit(1)
        self.human_commit(2)
        result, env = self.run_cleanup()
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertNotIn("SKIP_CI", env)
        self.assertEqual(self.git("branch", "--show-current"), "feature")
        self.assertEqual((self.root / HASH).read_text(), "base hash\n")
        self.assertEqual((self.root / RESOURCE).read_text(), "human resource 2\n")
        self.assertEqual((self.root / "application.go").read_text(), "human application 2\n")
        self.assertEqual(self.git("log", "--format=%s", "origin/main..HEAD").splitlines(),
                         ["human change 2", "human change 1"])

    def test_product_cleanup_never_discards_application_conflicts(self):
        self.bot_commit(application_conflict=True)
        self.human_commit(1)
        before = self.git("rev-parse", "HEAD")
        result, _ = self.run_cleanup()
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("unexpected rebase conflict in application.go", result.stdout)
        self.assertEqual(self.git("rev-parse", "HEAD"), before)
        self.assertEqual((self.root / "application.go").read_text(), "human application 1\n")

    def test_unchanged_resources_keep_existing_bot_commit(self):
        self.bot_commit()
        self.human_commit(1, resource_change=False)
        before = self.git("rev-parse", "HEAD")
        result, env = self.run_cleanup()
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertIn("SKIP_CI=true", env)
        self.assertEqual(self.git("rev-parse", "HEAD"), before)


if __name__ == "__main__":
    unittest.main()
