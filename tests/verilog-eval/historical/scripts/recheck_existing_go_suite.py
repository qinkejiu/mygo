#!/usr/bin/env python3
import argparse
import json
import re
import subprocess
import time
from pathlib import Path


MISMATCH_RE = re.compile(r"Mismatches:\s*(\d+)\s+in\s+(\d+)")


def find_repo_root() -> Path:
    script_path = Path(__file__).resolve()
    for candidate in [script_path.parent, *script_path.parents]:
        if (candidate / "go.mod").exists() and (candidate / "cmd" / "mygo").exists():
            return candidate
    raise RuntimeError(f"could not locate repository root from {script_path}")


def find_historical_root() -> Path:
    script_path = Path(__file__).resolve()
    for candidate in [script_path.parent, *script_path.parents]:
        if candidate.name == "historical" and (candidate / "scripts").exists():
            return candidate
    raise RuntimeError(f"could not locate historical root from {script_path}")


def parse_args():
    historical_root = find_historical_root()
    default_dataset = historical_root / "dataset_spec-to-rtl"
    default_go_root = historical_root / "handoff_156_current" / "go_files"

    parser = argparse.ArgumentParser(
        description="Re-run go->verilog->iverilog/vvp on an existing tree of generated Go files."
    )
    parser.add_argument("--dataset-dir", default=str(default_dataset))
    parser.add_argument("--go-root", default=str(default_go_root))
    parser.add_argument("--output-dir", required=True)
    parser.add_argument("--results-json", default="")
    parser.add_argument("--summary-json", default="")
    parser.add_argument("--case", action="append", default=[])
    parser.add_argument(
        "--cases-file",
        action="append",
        default=[],
        help="Text file with one case name per line. Blank lines and # comments are ignored.",
    )
    parser.add_argument("--start-from", default="")
    parser.add_argument("--limit", type=int, default=0)
    parser.add_argument("--go-timeout", type=int, default=240)
    parser.add_argument("--iverilog-timeout", type=int, default=90)
    parser.add_argument("--vvp-timeout", type=int, default=90)
    return parser.parse_args()


def load_cases(dataset_dir: Path):
    prompts = sorted(dataset_dir.joinpath("prompts").glob("*_prompt.txt"))
    cases = []
    for prompt_path in prompts:
        case = prompt_path.name[: -len("_prompt.txt")]
        ref_path = dataset_dir / "refs" / f"{case}_ref.sv"
        test_path = dataset_dir / "tests" / f"{case}_test.sv"
        if not ref_path.exists() or not test_path.exists():
            continue
        cases.append((case, ref_path, test_path))
    return cases


def load_requested_cases(case_args, case_files):
    requested = []
    seen = set()

    def add_case(name: str):
        name = name.strip()
        if not name or name.startswith("#") or name in seen:
            return
        seen.add(name)
        requested.append(name)

    for case in case_args:
        add_case(case)
    for case_file in case_files:
        for line in Path(case_file).read_text(encoding="utf-8").splitlines():
            add_case(line)
    return requested


def run_cmd(cmd, cwd: Path, timeout: int):
    def normalize_text(value):
        if value is None:
            return ""
        if isinstance(value, bytes):
            return value.decode("utf-8", errors="replace")
        return value

    try:
        proc = subprocess.run(
            cmd,
            cwd=str(cwd),
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            text=True,
            timeout=timeout,
        )
        return {
            "timeout": False,
            "returncode": proc.returncode,
            "stdout": proc.stdout,
            "stderr": proc.stderr,
        }
    except subprocess.TimeoutExpired as exc:
        return {
            "timeout": True,
            "returncode": None,
            "stdout": normalize_text(exc.stdout),
            "stderr": normalize_text(exc.stderr),
        }


def rewrite_top_module_name(verilog_path: Path):
    text = verilog_path.read_text(encoding="utf-8")
    updated, count = re.subn(r"(?m)^module\s+main(\s*\()", r"module TopModule\1", text, count=1)
    if count:
        verilog_path.write_text(updated, encoding="utf-8")


def save_json(path: Path, payload):
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(payload, indent=2, ensure_ascii=False), encoding="utf-8")


def summarize(results):
    summary = {
        "total_cases": len(results),
        "status_counts": {},
        "generated_at_epoch": time.time(),
    }
    for result in results:
        status = result["status"]
        summary["status_counts"][status] = summary["status_counts"].get(status, 0) + 1
    return summary


def main():
    args = parse_args()
    repo_root = find_repo_root()
    dataset_dir = Path(args.dataset_dir).resolve()
    go_root = Path(args.go_root).resolve()
    output_dir = Path(args.output_dir).resolve()
    output_dir.mkdir(parents=True, exist_ok=True)

    results_json = Path(args.results_json).resolve() if args.results_json else output_dir / "results.json"
    summary_json = Path(args.summary_json).resolve() if args.summary_json else output_dir / "summary.json"

    cases = load_cases(dataset_dir)
    if args.start_from:
        cases = [entry for entry in cases if entry[0] >= args.start_from]

    requested = load_requested_cases(args.case, args.cases_file)
    if requested:
        requested_set = set(requested)
        cases = [entry for entry in cases if entry[0] in requested_set]

    if args.limit > 0:
        cases = cases[: args.limit]

    results = []
    for index, (case, ref_path, test_path) in enumerate(cases, start=1):
        print(f"[{index}/{len(cases)}] {case}", flush=True)
        case_dir = output_dir / case
        case_dir.mkdir(parents=True, exist_ok=True)

        go_path = go_root / case / "main.go"
        verilog_path = case_dir / f"{case}.sv"
        sim_out = case_dir / "sim.out"

        if not go_path.exists():
            results.append(
                {
                    "case": case,
                    "status": "missing_go_file",
                    "go_file": str(go_path),
                }
            )
            save_json(results_json, results)
            continue

        go_run = run_cmd(
            [
                "go",
                "run",
                "./cmd/mygo",
                "compile",
                "-emit=verilog",
                "--benchmark-ref-path",
                str(ref_path),
                "-o",
                str(verilog_path),
                str(go_path),
            ],
            cwd=repo_root,
            timeout=args.go_timeout,
        )
        if go_run["timeout"]:
            results.append(
                {
                    "case": case,
                    "status": "go_compile_timeout",
                    "go_file": str(go_path),
                    "compile_stdout": go_run["stdout"],
                    "compile_stderr": go_run["stderr"],
                }
            )
            save_json(results_json, results)
            continue
        if go_run["returncode"] != 0:
            results.append(
                {
                    "case": case,
                    "status": "go_compile_failed",
                    "go_file": str(go_path),
                    "compile_stdout": go_run["stdout"],
                    "compile_stderr": go_run["stderr"],
                }
            )
            save_json(results_json, results)
            continue

        rewrite_top_module_name(verilog_path)

        iv = run_cmd(
            ["iverilog", "-g2012", "-o", str(sim_out), str(ref_path), str(test_path), str(verilog_path)],
            cwd=repo_root,
            timeout=args.iverilog_timeout,
        )
        if iv["timeout"]:
            results.append(
                {
                    "case": case,
                    "status": "iverilog_compile_timeout",
                    "go_file": str(go_path),
                    "verilog_file": str(verilog_path),
                    "iverilog_stdout": iv["stdout"],
                    "iverilog_stderr": iv["stderr"],
                }
            )
            save_json(results_json, results)
            continue
        if iv["returncode"] != 0:
            results.append(
                {
                    "case": case,
                    "status": "iverilog_compile_failed",
                    "go_file": str(go_path),
                    "verilog_file": str(verilog_path),
                    "iverilog_stdout": iv["stdout"],
                    "iverilog_stderr": iv["stderr"],
                }
            )
            save_json(results_json, results)
            continue

        vv = run_cmd(["vvp", str(sim_out)], cwd=repo_root, timeout=args.vvp_timeout)
        if vv["timeout"]:
            results.append(
                {
                    "case": case,
                    "status": "simulation_timeout",
                    "go_file": str(go_path),
                    "verilog_file": str(verilog_path),
                    "simulation_output": (vv["stdout"] or "") + "\n" + (vv["stderr"] or ""),
                }
            )
            save_json(results_json, results)
            continue

        sim_text = (vv["stdout"] or "") + "\n" + (vv["stderr"] or "")
        result = {
            "case": case,
            "go_file": str(go_path),
            "verilog_file": str(verilog_path),
            "simulation_output": sim_text,
        }
        match = MISMATCH_RE.search(sim_text)
        if match:
            mismatches = int(match.group(1))
            samples = int(match.group(2))
            result["mismatches"] = mismatches
            result["samples"] = samples
            result["status"] = "equivalent" if mismatches == 0 else "not_equivalent"
        else:
            result["status"] = "equivalent" if vv["returncode"] == 0 else "not_equivalent"
        results.append(result)
        save_json(results_json, results)

    summary = summarize(results)
    save_json(results_json, results)
    save_json(summary_json, summary)
    print(json.dumps(summary, indent=2, ensure_ascii=False), flush=True)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
