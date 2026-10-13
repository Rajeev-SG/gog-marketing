"""Exercise exact-source release gates without publishing or calling providers."""
import importlib.util
from pathlib import Path
import os
import re
import subprocess
import tempfile
import unittest
from unittest.mock import patch

REPO_ROOT = Path(__file__).resolve().parent.parent
WORKFLOW_PATH = REPO_ROOT / ".github/workflows/ci.yml"
GO_JOB_NAMES = ("minimum-go", "postgres", "test")


def _job_lines(workflow_text, job_name):
    job_key = re.compile(r"^  " + re.escape(job_name) + r":$")
    lines = workflow_text.splitlines()
    for index, line in enumerate(lines):
        if not job_key.match(line):
            continue
        body = []
        cursor = index + 1
        while cursor < len(lines) and (not lines[cursor].strip() or lines[cursor].startswith("    ")):
            body.append(lines[cursor])
            cursor += 1
        return body
    raise AssertionError(f"{WORKFLOW_PATH} has no job {job_name!r}")


def _run_block(lines, step_name):
    step_header = "      - name: " + step_name
    for index, line in enumerate(lines):
        if line != step_header:
            continue
        cursor = index + 1
        while cursor < len(lines) and not lines[cursor].startswith("      - "):
            if lines[cursor].startswith("        run: |"):
                body = []
                cursor += 1
                while cursor < len(lines) and (not lines[cursor].strip() or lines[cursor].startswith("          ")):
                    body.append(lines[cursor])
                    cursor += 1
                indents = [len(part) - len(part.lstrip()) for part in body if part.strip()]
                cut = min(indents)
                return [part[cut:] if part.strip() else part for part in body]
            cursor += 1
        raise AssertionError(f"step {step_name!r} has no run block")
    raise AssertionError(f"job has no step {step_name!r}")


def _setup_go_cache_setting(lines):
    for index, line in enumerate(lines):
        if "uses: actions/setup-go@" not in line:
            continue
        cursor = index + 1
        while cursor < len(lines) and not lines[cursor].startswith("      - "):
            entry = lines[cursor].strip()
            if entry.startswith("cache:"):
                return entry.removeprefix("cache:").strip()
            cursor += 1
        raise AssertionError("setup-go step has no cache setting")
    raise AssertionError("job has no setup-go step")


def _execute_temp_step(job_name, runner_temp, github_env_path, run_id, attempt):
    snippet = "\n".join(
        _run_block(_job_lines(WORKFLOW_PATH.read_text(encoding="utf-8"), job_name), "Use disk-backed temporary storage")
    )
    script = runner_temp / ("temp-step-" + str(job_name) + ".sh")
    script.write_text(snippet, encoding="utf-8")
    env = os.environ.copy()
    env.pop("GOCACHE", None)
    env.update({
        "RUNNER_TEMP": str(runner_temp),
        "GITHUB_ENV": str(github_env_path),
        "GITHUB_RUN_ID": run_id,
        "GITHUB_RUN_ATTEMPT": attempt,
        "GITHUB_JOB": job_name,
    })
    subprocess.run(["bash", "-e", str(script)], check=True, env=env)


def _parse_env_file(path):
    values = {}
    for line in path.read_text(encoding="utf-8").splitlines():
        if "=" in line:
            key, _, value = line.partition("=")
            values[key] = value
    return values


def _go_env_gocache(values):
    env = os.environ.copy()
    env.pop("GOCACHE", None)
    env.update(values)
    result = subprocess.run(["go", "env", "GOCACHE"], check=True, capture_output=True, text=True, env=env)
    return result.stdout.strip()

spec = importlib.util.spec_from_file_location("release_ci", Path(__file__).with_name("check-release-ci.py"))
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)


def jobs():
    return [{"name": name, "status": "completed", "conclusion": "success", "labels": list(module.LABELS)} for name in module.REQUIRED]


class ReleaseCITests(unittest.TestCase):
    def test_all_required_checks_and_labels(self):
        self.assertTrue(module.valid_jobs(jobs()))

    def test_missing_failed_skipped_and_hosted_rejected(self):
        self.assertFalse(module.valid_jobs(jobs()[:-1]))
        for bad in ["failure", "skipped", "cancelled"]:
            rows = jobs()
            rows[0]["conclusion"] = bad
            self.assertFalse(module.valid_jobs(rows))
        rows = jobs()
        rows[0]["labels"] = ["ubuntu-latest"]
        self.assertFalse(module.valid_jobs(rows))

    def test_wrong_sha_never_queries_jobs(self):
        with patch("sys.argv", ["check-release-ci", "--repo", "owner/repo", "--sha", "wanted"]), patch.object(module, "gh_json", return_value={"workflow_runs": [{"id": 1, "head_sha": "other", "status": "completed", "conclusion": "success"}]}) as api:
            self.assertEqual(module.main(), 1)
            self.assertEqual(api.call_count, 1)

    def test_exact_success_and_pending_run(self):
        for status, conclusion, expected in [("completed", "success", 0), ("queued", None, 1), ("completed", "failure", 1)]:
            with patch("sys.argv", ["check-release-ci", "--repo", "owner/repo", "--sha", "wanted"]), patch.object(module, "gh_json", side_effect=[{"workflow_runs": [{"id": 1, "head_sha": "wanted", "status": status, "conclusion": conclusion}]}, {"jobs": jobs()}]):
                self.assertEqual(module.main(), expected)


class GoJobCacheIsolationTests(unittest.TestCase):
    """Regression for run 38410947948: Go jobs must not share one GOCACHE."""

    def _resolved_caches(self):
        run_id = "run-424242"
        attempt = "attempt-7"
        caches = {}
        with tempfile.TemporaryDirectory() as root:
            runner_temp = Path(root)
            github_env = runner_temp / "github_env"
            github_env.touch()
            for job_name in GO_JOB_NAMES:
                _execute_temp_step(job_name, runner_temp, github_env, run_id, attempt)
                caches[job_name] = _go_env_gocache(_parse_env_file(github_env))
            for job_name, cache in caches.items():
                with self.subTest(job=job_name):
                    self.assertIn("GOCACHE", _parse_env_file(github_env))
                    self.assertTrue(cache.startswith(str(runner_temp) + "/"), f"{job_name}: {cache}")
                    for token in (job_name, "424242", "7"):
                        self.assertIn(token, Path(cache).name, f"{job_name}: {cache}")
        return [caches[job_name] for job_name in GO_JOB_NAMES]

    def test_go_jobs_resolve_distinct_gocaches_under_runner_temp(self):
        caches = self._resolved_caches()
        self.assertEqual(len(set(caches)), len(GO_JOB_NAMES), "Go jobs share a GOCACHE")

    def test_go_jobs_disable_setup_go_cache(self):
        workflow_text = WORKFLOW_PATH.read_text(encoding="utf-8")
        for job_name in GO_JOB_NAMES:
            with self.subTest(job=job_name):
                self.assertEqual(_setup_go_cache_setting(_job_lines(workflow_text, job_name)), "false")
