"""Benchmark the installed PostHog CLI and the engines of this checkout on Linux."""

import argparse
import json
import os
import re
import shutil
import subprocess
import sys
import time
from pathlib import Path


def process_tree_rss(pid):
    """Sample current RSS across the tree, including children spawned by any thread."""
    pending = [pid]
    seen = set()
    resident_pages = 0
    while pending:
        current = pending.pop()
        if current in seen:
            continue
        seen.add(current)
        process = Path("/proc") / str(current)
        try:
            resident_pages += int((process / "statm").read_text().split()[1])
            for children in (process / "task").glob("*/children"):
                try:
                    pending.extend(map(int, children.read_text().split()))
                except FileNotFoundError:
                    pass
        except (FileNotFoundError, ProcessLookupError):
            pass
    return resident_pages * os.sysconf("SC_PAGE_SIZE")


def measure(project, command, log, env):
    with log.open("x") as output:
        start = time.monotonic()
        process = subprocess.Popen(command, cwd=project, stdout=output, stderr=subprocess.STDOUT, env=env)
        peak = 0
        while process.poll() is None:
            peak = max(peak, process_tree_rss(process.pid))
            time.sleep(0.1)
        elapsed = time.monotonic() - start
    output = log.read_text(errors="replace")
    logics = re.search(r"(\d+) logics? found!", output)
    stale = 'Run "kea-typegen write" to save' in output
    completed = bool(logics) and (process.returncode == 0 or (process.returncode == 1 and stale))
    return {
        "seconds": round(elapsed, 3),
        "peak_rss_mib": round(peak / 2**20, 1),
        "exit_code": process.returncode,
        "completed": completed,
        "logics": int(logics[1]) if logics else None,
        "inline_files_with_changes": output.count("Will not write inline logic types:") if completed else None,
        "log": str(log),
    }


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("project", type=Path, help="PostHog checkout with its frontend dependencies installed")
    parser.add_argument("output", type=Path, help="New directory for per-run logs and JSON results")
    parser.add_argument("--runs", type=int, default=3)
    parser.add_argument("--node", default="node", help="Node executable for each engine")
    parser.add_argument("--engines", nargs="+", choices=["posthog", "classic", "native", "go", "go-js-plugins"],
                        default=["posthog", "classic", "native"], help="Engines to compare in rotating order")
    parser.add_argument("--native-cli", type=Path, help="Use a separate release build for the native baseline")
    args = parser.parse_args()
    if not sys.platform.startswith("linux"):
        parser.error("sampling process-tree RSS requires Linux /proc")
    if args.runs < 1:
        parser.error("--runs must be positive")
    node = shutil.which(args.node)
    if not node:
        parser.error(f"cannot find Node executable: {args.node}")
    project = args.project.resolve()
    branch_cli = Path(__file__).resolve().parents[1] / "dist/src/cli/typegen.js"
    native_cli = args.native_cli.resolve() if args.native_cli else branch_cli
    installed_cli = project / "frontend/node_modules/kea-typegen/dist/src/cli/typegen.js"
    commands = {
        "posthog": [str(installed_cli), "check"],
        "go": [str(branch_cli), "check", "--compiler", "go"],
        "go-js-plugins": [str(branch_cli), "check", "--compiler", "go"],
        "classic": [str(branch_cli), "check", "--compiler", "classic"],
        "native": [str(native_cli), "check", "--compiler", "native"],
    }
    required = [project / ".kearc", *(Path(commands[name][0]) for name in args.engines)]
    for file in required:
        if not file.is_file():
            parser.error(f"missing {file}")
    directory = args.output.resolve()
    directory.mkdir(parents=True)
    env = dict(os.environ, NODE_OPTIONS="--max-old-space-size=16384")
    results = []
    failed = set()
    engines = list(dict.fromkeys(args.engines))
    for index in range(args.runs):
        offset = index % len(engines)
        for engine in engines[offset:] + engines[:offset]:
            if engine in failed:
                continue
            command = [os.path.abspath(node), *commands[engine], "--verbose"]
            name = f"{index + 1}-{engine}"
            print(f"Running {name}", flush=True)
            result = {
                "engine": engine,
                "run": index + 1,
                "command": command,
                **measure(project, command, directory / (name + ".log"),
                          dict(env, KEA_TYPEGEN_JS_PLUGINS="1" if engine == "go-js-plugins" else "0")),
            }
            print(json.dumps(result), flush=True)
            results.append(result)
            (directory / "results.json").write_text(json.dumps(results, indent=2) + "\n")
            if not result["completed"]:
                failed.add(engine)
    return int(bool(failed))


if __name__ == "__main__":
    raise SystemExit(main())
