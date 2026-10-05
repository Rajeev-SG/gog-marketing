#!/usr/bin/env python3
"""Refuse release unless the exact source SHA passed all Oracle CI jobs."""
import argparse
import json
import subprocess

REQUIRED = {"test", "minimum-go", "postgres", "worker", "hosted-state", "hosted-app"}
LABELS = {"self-hosted", "linux", "arm64", "oracle", "gog-marketing"}


def valid_jobs(jobs):
    by_name = {job.get("name"): job for job in jobs}
    return all(
        name in by_name
        and by_name[name].get("status") == "completed"
        and by_name[name].get("conclusion") == "success"
        and LABELS <= {str(label).lower() for label in by_name[name].get("labels", [])}
        for name in REQUIRED
    )


def gh_json(args):
    result = subprocess.run(["gh", "api", *args], capture_output=True, text=True, check=True, timeout=30)
    return json.loads(result.stdout)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--repo", required=True)
    parser.add_argument("--sha", required=True)
    args = parser.parse_args()
    runs = gh_json([f"repos/{args.repo}/actions/workflows/ci.yml/runs", "--method", "GET", "-f", f"head_sha={args.sha}", "-f", "per_page=100"])
    for run in runs.get("workflow_runs", []):
        if run.get("head_sha") != args.sha or run.get("status") != "completed" or run.get("conclusion") != "success":
            continue
        jobs = gh_json([f"repos/{args.repo}/actions/runs/{run['id']}/jobs?filter=latest&per_page=100"])
        if valid_jobs(jobs.get("jobs", [])):
            print(f"Exact source {args.sha}: all six self-hosted CI jobs passed")
            return 0
    print(f"Release denied: exact source {args.sha} lacks green six-job Oracle CI")
    return 1


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except (OSError, ValueError, subprocess.SubprocessError) as error:
        print(f"Release denied: CI verification unavailable ({type(error).__name__})")
        raise SystemExit(1)
