#!/usr/bin/env python3
import os
import sys
import subprocess
from pathlib import Path
from datetime import datetime

def find_repo_root() -> Path:
    script_path = Path(__file__).resolve()
    for candidate in [script_path.parent, *script_path.parents]:
        if (candidate / "go.mod").exists() and (candidate / "cmd" / "mygo").exists():
            return candidate
    raise RuntimeError(f"could not locate repository root from {script_path}")


REPO_ROOT = find_repo_root()
SCRIPT_DIR = Path(__file__).resolve().parent
BASE_DIR = SCRIPT_DIR
OUTPUT_FILE = SCRIPT_DIR / "stages_clean_simulation_results.txt"
GO_WORK_DIR = REPO_ROOT / ".mygo-tmp" / "test-go"
GO_CACHE_DIR = GO_WORK_DIR / "go-build"
GO_TMP_DIR = GO_WORK_DIR / "go-tmp"
MYGO_BINARY = GO_WORK_DIR / "mygo-test-bin"
COMMAND_TIMEOUT_SECONDS = 120
BUILD_TIMEOUT_SECONDS = 30

# Keep per-case budgets in sync with handshake/FSM latency so hardware runs
# long enough to produce terminal prints (e.g. finished/router complete).
SIM_MAX_CYCLES = {
    "simple_channel": 32,
    "pipeline1": 64,
    "pipeline2": 80,
    "router_csp": 80,
}
NORMALIZE_LINE_ORDER_CASES = {
    "phi_loop",
    "pipeline1",
    "pipeline2",
    "router_csp",
}

def benchmark_ref_args(main_go: str) -> list[str]:
    main_path = (REPO_ROOT / main_go).resolve()
    case_name = main_path.parent.name
    candidates = [
        REPO_ROOT / "tests" / "verilog-eval" / "reference_verilog" / f"{case_name}_ref.sv",
        REPO_ROOT / "tests" / "verilog-eval" / "historical" / "dataset_spec-to-rtl" / "refs" / f"{case_name}_ref.sv",
    ]
    for candidate in candidates:
        if candidate.exists():
            return ["--benchmark-ref-path", str(candidate)]
    return []

def should_filter_verilator(line: str) -> bool:
    """过滤 Verilator 编译过程日志，保留真正的错误信息"""
    verilator_noise = [
        "make: Entering directory",
        "make: Leaving directory",
        "g++ ",
        " -c -o ",
        " -MMD -I/usr/share/verilator",
        "verilator_includer",
        "Archive ar -rcs",
        "rm Vmain__ALL.verilator_deplist.tmp",
        "echo \"\" >",
        "Vmain__ALL.verilator_deplist.tmp",
        "DVM_COVERAGE=",
        "DVM_TRACE=",
        "Wno-bool-operation",
        "Wno-unused",
        "/usr/share/verilator/include",
        "verilated.cpp",
        "verilated_threads.cpp",
        "obj_dir",
        ".mygo-verilator-",
        ".mygo-tmp",
    ]
    # 保留真正的编译错误（如 error: / fatal error:）
    if "error:" in line.lower() or "fatal error" in line.lower() or "undefined reference" in line.lower():
        return False
    return any(noise in line for noise in verilator_noise)

def prepare_go_env():
    GO_CACHE_DIR.mkdir(parents=True, exist_ok=True)
    GO_TMP_DIR.mkdir(parents=True, exist_ok=True)
    env = os.environ.copy()
    env["GOCACHE"] = str(GO_CACHE_DIR.resolve())
    env["GOTMPDIR"] = str(GO_TMP_DIR.resolve())
    return env

def build_mygo_binary() -> str | None:
    env = prepare_go_env()
    MYGO_BINARY.parent.mkdir(parents=True, exist_ok=True)
    try:
        result = subprocess.run(
            ["go", "build", "-o", str(MYGO_BINARY), "./cmd/mygo"],
            capture_output=True,
            text=True,
            timeout=BUILD_TIMEOUT_SECONDS,
            env=env,
            cwd=REPO_ROOT,
        )
    except subprocess.TimeoutExpired:
        print(f"❌ building mygo binary timed out ({BUILD_TIMEOUT_SECONDS}s)", file=sys.stderr)
        return None
    except Exception as exc:
        print(f"❌ failed to build mygo binary: {exc}", file=sys.stderr)
        return None
    if result.returncode != 0:
        stderr = "\n".join(
            line for line in result.stderr.splitlines()
            if not should_filter_verilator(line)
        ).strip()
        print("❌ failed to build mygo binary", file=sys.stderr)
        if stderr:
            print(stderr, file=sys.stderr)
        return None
    return str(MYGO_BINARY.resolve())


def normalize_stdout(folder_name: str, stdout: str) -> str:
    if folder_name not in NORMALIZE_LINE_ORDER_CASES:
        return stdout
    lines = [line.rstrip() for line in stdout.splitlines() if line.strip()]
    if not lines:
        return stdout
    lines.sort()
    return "\n".join(lines) + "\n"

def run_command(cmd: str, folder_name: str, cmd_label: str, output_fh) -> bool:
    timestamp = datetime.now().strftime("%H:%M:%S")
    separator = "=" * 70
    output_fh.write(f"\n{separator}\n")
    output_fh.write(f"📁 {folder_name} | {cmd_label}\n")
    output_fh.write(f"⏱️  {timestamp} | $ {cmd}\n")
    output_fh.write(f"{separator}\n\n")
    output_fh.flush()

    try:
        env = prepare_go_env()
        result = subprocess.run(
            cmd,
            shell=True,
            capture_output=True,
            text=True,
            timeout=COMMAND_TIMEOUT_SECONDS,
            env=env,
            cwd=REPO_ROOT,
        )

        # 清理 STDOUT：保留所有（仿真结果）
        clean_stdout = normalize_stdout(folder_name, result.stdout)
        output_fh.write(clean_stdout)
        if not clean_stdout.strip():
            output_fh.write("(no output)\n")

        # 清理 STDERR：过滤 Verilator 编译日志，仅保留真实错误
        clean_stderr = "\n".join(
            line for line in result.stderr.splitlines()
            if not should_filter_verilator(line)
        ).strip()

        if clean_stderr:
            output_fh.write("\n[ERROR]\n")
            output_fh.write(clean_stderr + "\n")

        end_time = datetime.now().strftime("%H:%M:%S")
        status = "✅" if result.returncode == 0 else f"❌ code={result.returncode}"
        output_fh.write(f"\n{separator}\n")
        output_fh.write(f"{status} | End: {end_time}\n")
        output_fh.write(f"{separator}\n\n")
        output_fh.flush()

        return result.returncode == 0
    except subprocess.TimeoutExpired:
        output_fh.write(f"❌ TIMEOUT ({COMMAND_TIMEOUT_SECONDS}s)\n\n")
        output_fh.flush()
        return False
    except Exception as e:
        output_fh.write(f"❌ EXCEPTION: {e}\n\n")
        output_fh.flush()
        return False

def main():
    if not BASE_DIR.exists():
        print(f"❌ Error: '{BASE_DIR}' not found", file=sys.stderr)
        sys.exit(1)

    folders = sorted([
        d.name for d in BASE_DIR.iterdir()
        if d.is_dir() and (d / "main.go").exists()
    ])

    if not folders:
        print(f"⚠️  No valid folders with main.go found in {BASE_DIR}", file=sys.stderr)
        sys.exit(1)

    print(f"🔍 Found {len(folders)} folders with main.go")
    print(f"📝 Output will be saved to: {OUTPUT_FILE.resolve()}\n")
    mygo_bin = build_mygo_binary()
    if not mygo_bin:
        sys.exit(1)

    success1 = success2 = 0

    with open(OUTPUT_FILE, "w", encoding="utf-8") as f:
        f.write(f"CLEAN SIMULATION RESULTS (Verilator logs filtered)\n")
        f.write(f"Generated: {datetime.now().strftime('%Y-%m-%d %H:%M:%S')}\n")
        f.write(f"{'='*70}\n\n")

        for idx, folder in enumerate(folders, 1):
            main_go = f"tests/stages/{folder}/main.go"
            print(f"[{idx:2d}/{len(folders)}] {folder:20s}", end=" ", flush=True)

            # 命令1: mygo sim（硬件仿真）
            max_cycles = SIM_MAX_CYCLES.get(folder, 64)
            benchmark_args = benchmark_ref_args(main_go)
            extra = " ".join(benchmark_args)
            if extra:
                extra = extra + " "
            cmd1 = f"{mygo_bin} sim {extra}--sim-max-cycles {max_cycles} {main_go}"
            ok1 = run_command(cmd1, folder, "hardware simulation", f)
            if ok1:
                success1 += 1

            # 命令2: direct go run（软件仿真）
            cmd2 = f"go run {main_go}"
            ok2 = run_command(cmd2, folder, "software simulation", f)
            if ok2:
                success2 += 1

            status = "✅" if (ok1 or ok2) else "⚠️"
            print(f"{status} (hw:{'✓' if ok1 else '✗'} sw:{'✓' if ok2 else '✗'})")

        # 汇总
        f.write(f"\n{'='*70}\n")
        f.write("SUMMARY\n")
        f.write(f"{'='*70}\n")
        f.write(f"Total folders: {len(folders)}\n")
        f.write(f"Hardware simulation (mygo sim) success: {success1}/{len(folders)}\n")
        f.write(f"Software simulation (direct run) success: {success2}/{len(folders)}\n")
        f.write(f"{'='*70}\n")

    print(f"\n✅ Done. Clean results saved to:\n   {OUTPUT_FILE.resolve()}")
    print(f"\n📊 Summary: HW={success1}/{len(folders)} | SW={success2}/{len(folders)}")

if __name__ == "__main__":
    main()
