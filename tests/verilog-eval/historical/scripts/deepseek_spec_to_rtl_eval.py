#!/usr/bin/env python3
import argparse
import json
import os
import re
import subprocess
import sys
import time
import urllib.error
import urllib.request
from pathlib import Path


MYGO_DSL_SUMMARY = """\
MyGO Go DSL constraints:

1. The file must start with: package main
2. The hardware entry must be: func TopModule(...)
3. Inputs are ordinary TopModule parameters.
4. Outputs must be declared as package-level globals whose names start with out_.
   Example:
       var out_zero bool
       var out_q uint8
       var out_data [4]bool
5. TopModule must assign to those out_* globals directly.
6. Do NOT use return values to represent hardware outputs.
7. Do NOT use pointer parameters like *bool, *uint8, etc.
8. Do NOT use structs, interfaces, maps, select, recursion, or dynamic goroutines.
9. Keep types simple: bool, uint8, uint16, uint32, int where needed, and fixed-size arrays.
10. Prefer straightforward combinational/sequential logic in TopModule only.
11. If a clocked design is requested, write logic using ordinary Go conditionals on clk/reset signals.
12. Include func main() {} at the end only as an empty stub.

Vector modeling rules:
13. Prefer unsigned scalar integers for packed vectors up to 32 bits:
    - 1 bit: bool
    - 2..8 bits: uint8
    - 9..16 bits: uint16
    - 17..32 bits: uint32
14. For packed vector logic, use shifts, masks, bitwise ops, and integer comparisons.
15. Avoid [N]bool for packed vectors <= 32 bits.
16. Only use [N]bool arrays for very wide vectors like 100 bits or 256 bits when necessary.
17. When using [N]bool arrays, index 0 should represent the least-significant bit unless the prompt clearly implies otherwise.
18. For split outputs like out_hi/out_lo, use separate out_* globals with scalar unsigned integer types.
19. For adders, muxes, equality, bit-reverse, byte-reverse, and popcount on widths <= 32 bits, prefer scalar integer expressions over per-bit loops.

Good pattern example:
    package main
    var out_zero bool
    func TopModule() {
        out_zero = false
    }
    func main() {}

Bad patterns:
    func TopModule() bool { ... }              // no return-value outputs
    func TopModule(out *bool) { ... }          // no pointer outputs
    type Ports struct { ... }                  // no struct port bundles

Useful examples:
    package main
    var out_out uint32
    func TopModule(in uint32) {
        out_out = ((in & 0x000000FF) << 24) |
            ((in & 0x0000FF00) << 8) |
            ((in & 0x00FF0000) >> 8) |
            ((in & 0xFF000000) >> 24)
    }
    func main() {}

    package main
    var out_hi uint8
    var out_lo uint8
    func TopModule(in uint16) {
        out_hi = uint8((in >> 8) & 0xFF)
        out_lo = uint8(in & 0xFF)
    }
    func main() {}

    package main
    var out_z bool
    func TopModule(A uint8, B uint8) {
        out_z = (A & 0x3) == (B & 0x3)
    }
    func main() {}

    package main
    var out_out uint8
    func TopModule(in uint8) {
        count := uint8(0)
        if (in & 0x1) != 0 { count++ }
        if (in & 0x2) != 0 { count++ }
        if (in & 0x4) != 0 { count++ }
        out_out = count & 0x3
    }
    func main() {}
"""


def parse_args():
    p = argparse.ArgumentParser()
    p.add_argument("--dataset-dir", required=True)
    p.add_argument("--output-dir", required=True)
    p.add_argument("--results-json", required=True)
    p.add_argument("--model", default="deepseek-chat")
    p.add_argument("--api-base", default="https://api.deepseek.com")
    p.add_argument("--limit", type=int, default=0)
    p.add_argument("--start-from", default="")
    p.add_argument("--sleep-seconds", type=float, default=0.0)
    p.add_argument("--llm-timeout", type=int, default=120)
    p.add_argument("--go-timeout", type=int, default=180)
    p.add_argument("--iverilog-timeout", type=int, default=60)
    p.add_argument("--vvp-timeout", type=int, default=60)
    p.add_argument("--repair-attempts", type=int, default=1)
    return p.parse_args()


def load_cases(dataset_dir: Path):
    prompts = sorted(dataset_dir.joinpath("prompts").glob("*_prompt.txt"))
    cases = []
    for prompt_path in prompts:
        case = prompt_path.name[: -len("_prompt.txt")]
        ref_path = dataset_dir / "refs" / f"{case}_ref.sv"
        test_path = dataset_dir / "tests" / f"{case}_test.sv"
        if not ref_path.exists() or not test_path.exists():
            continue
        cases.append((case, prompt_path, ref_path, test_path))
    return cases


def strip_code_fence(text: str) -> str:
    m = re.search(r"```(?:go)?\s*(.*?)```", text, flags=re.S)
    if m:
        return m.group(1).strip() + "\n"
    return text.strip() + "\n"


def ensure_main_wrapper(go_src: str) -> str:
    if re.search(r"\bfunc\s+main\s*\(", go_src):
        return go_src
    src = go_src.rstrip() + "\n\nfunc main() {}\n"
    return src


def call_deepseek(api_base: str, api_key: str, model: str, prompt_text: str, ref_sv: str, timeout: int) -> str:
    user_prompt = (
        "Generate one complete Go source file for the MyGO compiler.\n"
        "Return Go code only.\n\n"
        "Follow this DSL summary exactly:\n"
        f"{MYGO_DSL_SUMMARY}\n\n"
        "Original task prompt:\n"
        f"{prompt_text}\n\n"
        "Reference Verilog:\n"
        f"{ref_sv}\n"
    )
    body = {
        "model": model,
        "messages": [
            {
                "role": "system",
                "content": (
                    "Generate only valid Go source code for MyGO. "
                    "Follow the provided DSL exactly. "
                    "Outputs must use out_* package globals. "
                    "Do not use pointer outputs or return-value outputs."
                ),
            },
            {"role": "user", "content": user_prompt},
        ],
        "temperature": 0.2,
    }
    req = urllib.request.Request(
        api_base.rstrip("/") + "/chat/completions",
        data=json.dumps(body).encode("utf-8"),
        headers={
            "Content-Type": "application/json",
            "Authorization": f"Bearer {api_key}",
        },
        method="POST",
    )
    try:
        with urllib.request.urlopen(req, timeout=timeout) as resp:
            payload = json.loads(resp.read().decode("utf-8"))
    except urllib.error.HTTPError as e:
        detail = e.read().decode("utf-8", errors="replace")
        raise RuntimeError(f"HTTP {e.code}: {detail}") from e
    except urllib.error.URLError as e:
        raise RuntimeError(f"network error: {e}") from e
    try:
        content = payload["choices"][0]["message"]["content"]
    except Exception as e:
        raise RuntimeError(f"unexpected response: {payload}") from e
    return strip_code_fence(content)


def call_deepseek_repair(api_base: str, api_key: str, model: str, prompt_text: str, ref_sv: str, broken_go: str, feedback: str, timeout: int) -> str:
    user_prompt = (
        "Repair the following Go code so it compiles under the MyGO DSL.\n"
        "Return Go code only.\n\n"
        "Follow this DSL summary exactly:\n"
        f"{MYGO_DSL_SUMMARY}\n\n"
        "Repair requirements:\n"
        "- Return exactly one complete Go file.\n"
        "- Keep exactly one func TopModule(...).\n"
        "- Keep package main.\n"
        "- Keep outputs only as package-level out_* globals.\n"
        "- Remove or inline any local variable that is declared but not used.\n"
        "- Do not use ?: syntax, pointers for outputs, or return-value outputs.\n"
        "- If a temporary is only used once, inline it instead of leaving dead locals.\n\n"
        "Original task prompt:\n"
        f"{prompt_text}\n\n"
        "Reference Verilog:\n"
        f"{ref_sv}\n\n"
        "Broken Go code:\n"
        f"{broken_go}\n\n"
        "Compiler or elaboration feedback:\n"
        f"{feedback}\n"
    )
    body = {
        "model": model,
        "messages": [
            {
                "role": "system",
                "content": (
                    "Repair invalid Go code for MyGO. "
                    "Do not explain. "
                    "Return one complete corrected Go file only. "
                    "Avoid duplicate functions, illegal Go syntax, unused locals, and invalid bool/integer conversions. "
                    "If the feedback says a variable is declared and not used, remove it or inline it."
                ),
            },
            {"role": "user", "content": user_prompt},
        ],
        "temperature": 0.1,
    }
    req = urllib.request.Request(
        api_base.rstrip("/") + "/chat/completions",
        data=json.dumps(body).encode("utf-8"),
        headers={
            "Content-Type": "application/json",
            "Authorization": f"Bearer {api_key}",
        },
        method="POST",
    )
    try:
        with urllib.request.urlopen(req, timeout=timeout) as resp:
            payload = json.loads(resp.read().decode("utf-8"))
    except urllib.error.HTTPError as e:
        detail = e.read().decode("utf-8", errors="replace")
        raise RuntimeError(f"HTTP {e.code}: {detail}") from e
    except urllib.error.URLError as e:
        raise RuntimeError(f"network error: {e}") from e
    try:
        content = payload["choices"][0]["message"]["content"]
    except Exception as e:
        raise RuntimeError(f"unexpected response: {payload}") from e
    return strip_code_fence(content)


def run_cmd(cmd, cwd: Path, timeout: int = 300):
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
    except subprocess.TimeoutExpired as e:
        return {
            "timeout": True,
            "returncode": None,
            "stdout": e.stdout or "",
            "stderr": e.stderr or "",
        }


def classify_equivalence(vvp_stdout: str, vvp_stderr: str, returncode: int):
    text = (vvp_stdout or "") + "\n" + (vvp_stderr or "")
    m = re.search(r"Mismatches:\s*(\d+)\s+in\s+(\d+)", text)
    if m:
        mismatches = int(m.group(1))
        samples = int(m.group(2))
        return mismatches == 0, mismatches, samples, text
    if returncode == 0:
        return True, 0, 0, text
    return False, None, None, text


def rewrite_top_module_name(verilog_path: Path):
    text = verilog_path.read_text(encoding="utf-8")
    updated, count = re.subn(r"(?m)^module\s+main(\s*\()", r"module TopModule\1", text, count=1)
    if count == 0:
        return False
    verilog_path.write_text(updated, encoding="utf-8")
    return True


def save_results(results_path: Path, results):
    results_path.write_text(json.dumps(results, indent=2, ensure_ascii=False), encoding="utf-8")


def try_compile(repo_root: Path, go_path: Path, verilog_path: Path, timeout: int):
    return run_cmd(
        ["go", "run", "./cmd/mygo", "compile", "-emit=verilog", "-o", str(verilog_path), str(go_path)],
        cwd=repo_root,
        timeout=timeout,
    )


def main():
    args = parse_args()
    api_key = os.environ.get("DEEPSEEK_API_KEY", "").strip()
    if not api_key:
        print("DEEPSEEK_API_KEY is required", file=sys.stderr)
        return 2

    repo_root = Path.cwd()
    dataset_dir = Path(args.dataset_dir).resolve()
    output_dir = Path(args.output_dir).resolve()
    output_dir.mkdir(parents=True, exist_ok=True)
    results_path = Path(args.results_json).resolve()
    results_path.parent.mkdir(parents=True, exist_ok=True)

    cases = load_cases(dataset_dir)
    if args.start_from:
        cases = [c for c in cases if c[0] >= args.start_from]
    if args.limit > 0:
        cases = cases[: args.limit]

    results = []
    for idx, (case, prompt_path, ref_path, test_path) in enumerate(cases, start=1):
        print(f"[{idx}/{len(cases)}] {case}", flush=True)
        case_dir = output_dir / case
        case_dir.mkdir(parents=True, exist_ok=True)
        go_path = case_dir / "main.go"
        verilog_path = case_dir / f"{case}.sv"
        try:
            prompt_text = prompt_path.read_text(encoding="utf-8")
            ref_sv = ref_path.read_text(encoding="utf-8")
            generated_go = call_deepseek(args.api_base, api_key, args.model, prompt_text, ref_sv, args.llm_timeout)
            generated_go = ensure_main_wrapper(generated_go)
            go_path.write_text(generated_go, encoding="utf-8")
        except Exception as e:
            results.append(
                {
                    "case": case,
                    "status": "llm_failed",
                    "error": str(e),
                }
            )
            save_results(results_path, results)
            continue

        go_run = try_compile(repo_root, go_path, verilog_path, args.go_timeout)
        if (not go_run["timeout"]) and go_run["returncode"] != 0 and args.repair_attempts > 0:
            compile_feedback = (go_run["stdout"] or "") + "\n" + (go_run["stderr"] or "")
            current_go = go_path.read_text(encoding="utf-8")
            repaired = False
            for _ in range(args.repair_attempts):
                try:
                    fixed_go = call_deepseek_repair(
                        args.api_base,
                        api_key,
                        args.model,
                        prompt_text,
                        ref_sv,
                        current_go,
                        compile_feedback,
                        args.llm_timeout,
                    )
                    fixed_go = ensure_main_wrapper(fixed_go)
                    go_path.write_text(fixed_go, encoding="utf-8")
                    current_go = fixed_go
                    go_run = try_compile(repo_root, go_path, verilog_path, args.go_timeout)
                    if (not go_run["timeout"]) and go_run["returncode"] == 0:
                        repaired = True
                        break
                    compile_feedback = (go_run["stdout"] or "") + "\n" + (go_run["stderr"] or "")
                except Exception as e:
                    compile_feedback = compile_feedback + f"\nREPAIR_ERROR: {e}\n"
            # leave go_run as final attempt result whether or not repaired
        if go_run["timeout"]:
            results.append(
                {
                    "case": case,
                    "status": "go_compile_timeout",
                    "compile_stdout": go_run["stdout"],
                    "compile_stderr": go_run["stderr"],
                    "go_file": str(go_path),
                }
            )
            save_results(results_path, results)
            continue
        if go_run["returncode"] != 0:
            results.append(
                {
                    "case": case,
                    "status": "go_compile_failed",
                    "compile_stdout": go_run["stdout"],
                    "compile_stderr": go_run["stderr"],
                    "go_file": str(go_path),
                }
            )
            save_results(results_path, results)
            continue

        try:
            rewrite_top_module_name(verilog_path)
        except Exception as e:
            results.append(
                {
                    "case": case,
                    "status": "postprocess_failed",
                    "error": str(e),
                    "go_file": str(go_path),
                    "verilog_file": str(verilog_path),
                }
            )
            save_results(results_path, results)
            continue

        sim_out = case_dir / "sim.out"
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
                    "iverilog_stdout": iv["stdout"],
                    "iverilog_stderr": iv["stderr"],
                    "go_file": str(go_path),
                    "verilog_file": str(verilog_path),
                }
            )
            save_results(results_path, results)
            continue
        if iv["returncode"] != 0:
            results.append(
                {
                    "case": case,
                    "status": "iverilog_compile_failed",
                    "iverilog_stdout": iv["stdout"],
                    "iverilog_stderr": iv["stderr"],
                    "go_file": str(go_path),
                    "verilog_file": str(verilog_path),
                }
            )
            save_results(results_path, results)
            continue

        vv = run_cmd(["vvp", str(sim_out)], cwd=repo_root, timeout=args.vvp_timeout)
        if vv["timeout"]:
            results.append(
                {
                    "case": case,
                    "status": "simulation_timeout",
                    "simulation_output": (vv["stdout"] or "") + "\n" + (vv["stderr"] or ""),
                    "go_file": str(go_path),
                    "verilog_file": str(verilog_path),
                }
            )
            save_results(results_path, results)
            continue
        equivalent, mismatches, samples, sim_text = classify_equivalence(vv["stdout"], vv["stderr"], vv["returncode"])
        result = {
            "case": case,
            "status": "equivalent" if equivalent else "not_equivalent",
            "go_file": str(go_path),
            "verilog_file": str(verilog_path),
        }
        if mismatches is not None:
            result["mismatches"] = mismatches
            result["samples"] = samples
        result["simulation_output"] = sim_text
        results.append(result)

        save_results(results_path, results)
        if args.sleep_seconds > 0:
            time.sleep(args.sleep_seconds)

    save_results(results_path, results)
    total = len(results)
    eq = sum(1 for r in results if r["status"] == "equivalent")
    print(json.dumps({"total": total, "equivalent": eq}, indent=2), flush=True)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
