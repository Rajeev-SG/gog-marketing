"""Exercise exact-source release gates without publishing or calling providers."""
import importlib.util
from pathlib import Path
import unittest
from unittest.mock import patch

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
