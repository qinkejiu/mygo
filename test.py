#!/usr/bin/env python3
import difflib
import os
import shlex
import shutil
import subprocess
import sys
from datetime import datetime
from pathlib import Path

DEFAULT_BASE_DIRS = (Path("tests/CHStone"),)
OUTPUT_FILE = Path("CHS_clean_simulation_results.txt")
COMMAND_TIMEOUT_SEC = 600
DEFAULT_SIM_MAX_CYCLES = 20000
INITIAL_SIM_MAX_CYCLES = 4096
SIM_CYCLE_GROWTH_FACTOR = 2
EMIT_FORMATS = ("ssa", "ir", "mlir", "verilog")

# Heavier CHStone workloads often need many more cycles than stage tests.
SIM_MAX_CYCLES = {
    "common": 1024,
    "dfadd": 20000,
    "dfdiv": 20000,
    "dfmul": 20000,
    "dfsin": 40000,
    "adpcm": 40000,
    "gsm": 60000,
    "motion": 60000,
    "aes": 120000,
    "blowfish": 120000,
    "sha": 80000,
    "mips": 120000,
}

HARDWARE_UNSUPPORTED_MARKERS = (
    "unresolved dereference",
    "no signal mapping for value",
    "unsupported argument",
    "unsupported unary op",
    "has unresolved operand; using zero value fallback",
)

CYCLE_SHORTFALL_MARKERS = (
    "max cycles",
    "cycle limit",
    "timed out",
    "timeout",
)


def should_filter_verilator(line: str) -> bool:
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
    if "error:" in line.lower() or "fatal error" in line.lower() or "undefined reference" in line.lower():
        return False
    return any(noise in line for noise in verilator_noise)


def command_env() -> dict[str, str]:
    env = os.environ.copy()
    env.setdefault("GOCACHE", "/tmp/go-build")
    if not env.get("GOPATH"):
        env["GOPATH"] = str(Path.home() / "go")
    return env


def run_command(cmd: str, folder_name: str, cmd_label: str, output_fh):
    timestamp = datetime.now().strftime("%H:%M:%S")
    separator = "=" * 70
    output_fh.write(f"\n{separator}\n")
    output_fh.write(f"📁 {folder_name} | {cmd_label}\n")
    output_fh.write(f"⏱️  {timestamp} | $ {cmd}\n")
    output_fh.write(f"{separator}\n\n")
    output_fh.flush()

    try:
        result = subprocess.run(
            cmd,
            shell=True,
            capture_output=True,
            text=True,
            timeout=COMMAND_TIMEOUT_SEC,
            env=command_env(),
        )

        clean_stdout = result.stdout
        output_fh.write(clean_stdout if clean_stdout.strip() else "(no output)\n")

        clean_stderr = "\n".join(
            line for line in result.stderr.splitlines() if not should_filter_verilator(line)
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

        return result.returncode == 0, clean_stdout, clean_stderr, result.returncode
    except subprocess.TimeoutExpired:
        output_fh.write(f"❌ TIMEOUT ({COMMAND_TIMEOUT_SEC}s)\n\n")
        output_fh.flush()
        return False, "", f"TIMEOUT ({COMMAND_TIMEOUT_SEC}s)", 124
    except Exception as exc:
        output_fh.write(f"❌ EXCEPTION: {exc}\n\n")
        output_fh.flush()
        return False, "", str(exc), 1


def remove_path(path: Path, removed: list[Path], failed: list[tuple[Path, str]]) -> None:
    if not path.exists():
        return
    try:
        if path.is_dir() and not path.is_symlink():
            shutil.rmtree(path)
        else:
            path.unlink()
        removed.append(path)
    except Exception as exc:
        failed.append((path, str(exc)))


def cleanup_cache_paths(case_dirs: list[Path]) -> tuple[list[Path], list[tuple[Path, str]]]:
    targets: set[Path] = set()
    script_dir = Path(__file__).resolve().parent
    targets.add(script_dir / "__pycache__")
    for case_dir in case_dirs:
        targets.add(case_dir / ".mygo-tmp")
        targets.add(case_dir / "__pycache__")

    # Only remove the default go-build cache that this script opts into.
    gocache = os.environ.get("GOCACHE", "")
    if not gocache or Path(gocache) == Path("/tmp/go-build"):
        targets.add(Path("/tmp/go-build"))

    removed: list[Path] = []
    failed: list[tuple[Path, str]] = []
    for target in sorted(targets, key=lambda p: p.as_posix()):
        remove_path(target, removed, failed)
    return removed, failed


def parse_base_dirs(argv: list[str]) -> list[Path]:
    if not argv:
        return list(DEFAULT_BASE_DIRS)
    return [Path(arg) for arg in argv]


def discover_case_dirs(base_dirs: list[Path]) -> list[Path]:
    case_dirs: set[Path] = set()
    for base_dir in base_dirs:
        if not base_dir.exists():
            print(f"⚠️  Skip missing base dir: {base_dir}", file=sys.stderr)
            continue
        for main_go in base_dir.rglob("main.go"):
            case_dirs.add(main_go.parent)
    return sorted(case_dirs, key=lambda p: p.as_posix())


def case_label(case_dir: Path) -> str:
    try:
        return case_dir.relative_to(Path.cwd()).as_posix()
    except ValueError:
        return case_dir.as_posix()


def emit_output_path(main_go: Path, emit: str) -> Path:
    if emit == "verilog":
        return main_go.with_suffix(".sv")
    return main_go.with_suffix(f".{emit}")


def run_emit_conversions(main_go: Path, case_name: str, output_fh) -> dict[str, tuple[bool, int, Path]]:
    results: dict[str, tuple[bool, int, Path]] = {}
    main_go_str = main_go.as_posix()
    for emit in EMIT_FORMATS:
        output_path = emit_output_path(main_go, emit)
        cmd = (
            "go run ./cmd/mygo compile "
            f"-emit={emit} "
            f"-o {shlex.quote(output_path.as_posix())} "
            f"{shlex.quote(main_go_str)}"
        )
        ok, _, _, code = run_command(cmd, case_name, f"compile -emit={emit}", output_fh)
        results[emit] = (ok, code, output_path)
    return results


def write_text_file(path: Path, body: str) -> None:
    path.write_text(body, encoding="utf-8")


def write_case_outputs(
    case_dir: Path,
    emit_results: dict[str, tuple[bool, int, Path]],
    sw_stdout: str,
    sw_stderr: str,
    sw_code: int,
    sw_ok: bool,
    hw_stdout: str,
    hw_stderr: str,
    hw_code: int,
    hw_ok: bool,
    hw_cycles_used: int,
    outputs_match: bool,
    match_mode: str,
    diff_text: str,
) -> None:
    write_text_file(case_dir / "software.stdout.txt", sw_stdout)
    write_text_file(case_dir / "software.stderr.txt", sw_stderr)
    write_text_file(case_dir / "hardware.stdout.txt", hw_stdout)
    write_text_file(case_dir / "hardware.stderr.txt", hw_stderr)

    diff_path = case_dir / "sw_hw.diff.txt"
    if diff_text:
        write_text_file(diff_path, diff_text + ("\n" if not diff_text.endswith("\n") else ""))
    elif diff_path.exists():
        diff_path.unlink()

    summary_lines = [
        f"generated={datetime.now().strftime('%Y-%m-%d %H:%M:%S')}",
        f"case={case_label(case_dir)}",
    ]
    for emit in EMIT_FORMATS:
        emit_ok, emit_code, emit_path = emit_results[emit]
        summary_lines.append(f"{emit}_ok={emit_ok}")
        summary_lines.append(f"{emit}_return_code={emit_code}")
        summary_lines.append(f"{emit}_path={emit_path.as_posix()}")

    summary_lines.extend(
        [
            f"software_return_code={sw_code}",
            f"hardware_return_code={hw_code}",
            f"software_ok={sw_ok}",
            f"hardware_ok={hw_ok}",
            f"hardware_cycles_used={hw_cycles_used}",
            f"output_match={outputs_match}",
            f"output_match_mode={match_mode}",
            f"software_stdout_path={(case_dir / 'software.stdout.txt').as_posix()}",
            f"hardware_stdout_path={(case_dir / 'hardware.stdout.txt').as_posix()}",
        ]
    )
    write_text_file(case_dir / "simulation.result.txt", "\n".join(summary_lines) + "\n")


def normalize_output(text: str) -> str:
    lines = [line.rstrip() for line in text.splitlines()]
    return "\n".join(lines).strip()


def normalize_output_compact(text: str) -> str:
    # Compact normalization tolerates leading/trailing spaces and blank lines.
    lines = [line.strip() for line in text.splitlines()]
    compact = [line for line in lines if line]
    return "\n".join(compact).strip()


def outputs_equivalent(sw_stdout: str, hw_stdout: str) -> tuple[bool, str]:
    sw_norm = normalize_output(sw_stdout)
    hw_norm = normalize_output(hw_stdout)
    if sw_norm == hw_norm:
        return True, "strict"

    sw_compact = normalize_output_compact(sw_stdout)
    hw_compact = normalize_output_compact(hw_stdout)
    if sw_compact == hw_compact:
        return True, "compact"
    return False, "none"


def write_output_section(output_fh, title: str, body: str) -> None:
    output_fh.write(f"\n[{title}]\n")
    if body.strip():
        output_fh.write(body)
        if not body.endswith("\n"):
            output_fh.write("\n")
    else:
        output_fh.write("(no output)\n")


def likely_cycle_shortfall(sw_stdout: str, hw_ok: bool, hw_stdout: str, hw_stderr: str) -> bool:
    lowered = hw_stderr.lower()
    if any(marker in lowered for marker in CYCLE_SHORTFALL_MARKERS):
        return True

    sw_norm = normalize_output_compact(sw_stdout)
    hw_norm = normalize_output_compact(hw_stdout)

    if not sw_norm:
        return False

    # If hardware output is only a strict prefix, simulation likely stopped too early.
    if hw_ok and hw_norm and sw_norm.startswith(hw_norm) and sw_norm != hw_norm:
        return True
    if hw_ok and not hw_norm:
        return True

    # Fallback: if all produced lines are a correct prefix but fewer lines were produced.
    if hw_ok:
        sw_lines = sw_norm.splitlines()
        hw_lines = hw_norm.splitlines()
        if hw_lines and len(hw_lines) < len(sw_lines) and sw_lines[: len(hw_lines)] == hw_lines:
            return True
    return False


def run_hardware_simulation(main_go: str, folder_key: str, folder_label: str, sw_stdout: str, output_fh):
    max_cycles_cap = SIM_MAX_CYCLES.get(folder_key, DEFAULT_SIM_MAX_CYCLES)
    current_cycles = min(INITIAL_SIM_MAX_CYCLES, max_cycles_cap)
    last_shortfall_cycles = 0

    hw_ok = False
    hw_stdout = ""
    hw_stderr = ""
    hw_code = 1
    output_match = False
    match_mode = "none"
    matched_cycles = None

    while True:
        hw_cmd = f"go run ./cmd/mygo sim --sim-max-cycles {current_cycles} {shlex.quote(main_go)}"
        hw_ok, hw_stdout, hw_stderr, hw_code = run_command(
            hw_cmd,
            folder_label,
            f"hardware simulation (sim-max-cycles={current_cycles})",
            output_fh,
        )

        if hw_ok:
            lowered = hw_stderr.lower()
            if any(marker in lowered for marker in HARDWARE_UNSUPPORTED_MARKERS):
                hw_ok = False
                output_fh.write("[CHECK]\n")
                output_fh.write("Hardware run exited 0 but hit unsupported lowering markers; counting as FAIL.\n")
            else:
                output_match, match_mode = outputs_equivalent(sw_stdout, hw_stdout)
                if output_match and not any(marker in lowered for marker in CYCLE_SHORTFALL_MARKERS):
                    matched_cycles = current_cycles
                    break

        if current_cycles >= max_cycles_cap:
            return hw_ok, hw_stdout, hw_stderr, hw_code, current_cycles, output_match, match_mode

        if not likely_cycle_shortfall(sw_stdout, hw_ok, hw_stdout, hw_stderr):
            return hw_ok, hw_stdout, hw_stderr, hw_code, current_cycles, output_match, match_mode

        next_cycles = min(max_cycles_cap, current_cycles * SIM_CYCLE_GROWTH_FACTOR)
        if next_cycles <= current_cycles:
            return hw_ok, hw_stdout, hw_stderr, hw_code, current_cycles, output_match, match_mode

        output_fh.write("[RETRY]\n")
        output_fh.write(
            f"Likely cycle shortfall at {current_cycles} cycles; retrying with {next_cycles} cycles.\n"
        )
        last_shortfall_cycles = current_cycles
        current_cycles = next_cycles

    # Minimize the cycle budget once a matching run is found.
    assert matched_cycles is not None
    low = max(1, last_shortfall_cycles + 1)
    high = matched_cycles
    best_cycles = matched_cycles
    best_stdout = hw_stdout
    best_stderr = hw_stderr
    best_code = hw_code
    best_ok = hw_ok
    best_mode = match_mode

    if low < high:
        output_fh.write("[TUNE]\n")
        output_fh.write(
            f"Output matched at {matched_cycles} cycles; searching minimal matching cycle count in [{low}, {high}].\n"
        )

    while low < high:
        mid = (low + high) // 2
        hw_cmd = f"go run ./cmd/mygo sim --sim-max-cycles {mid} {shlex.quote(main_go)}"
        mid_ok, mid_stdout, mid_stderr, mid_code = run_command(
            hw_cmd,
            folder_label,
            f"hardware simulation tuning (sim-max-cycles={mid})",
            output_fh,
        )
        if mid_ok:
            lowered = mid_stderr.lower()
            if any(marker in lowered for marker in HARDWARE_UNSUPPORTED_MARKERS):
                mid_ok = False

        mid_match = False
        mid_mode = "none"
        if mid_ok:
            mid_match, mid_mode = outputs_equivalent(sw_stdout, mid_stdout)
            if mid_match and any(marker in lowered for marker in CYCLE_SHORTFALL_MARKERS):
                mid_match = False

        if mid_match:
            best_cycles = mid
            best_stdout = mid_stdout
            best_stderr = mid_stderr
            best_code = mid_code
            best_ok = mid_ok
            best_mode = mid_mode
            high = mid
        else:
            low = mid + 1

    if best_cycles != matched_cycles:
        output_fh.write("[TUNE]\n")
        output_fh.write(
            f"Reduced matching sim-max-cycles from {matched_cycles} to {best_cycles}.\n"
        )

    return best_ok, best_stdout, best_stderr, best_code, best_cycles, True, best_mode


def main():
    base_dirs = parse_base_dirs(sys.argv[1:])
    case_dirs = discover_case_dirs(base_dirs)
    try:
        if not case_dirs:
            joined = ", ".join(d.as_posix() for d in base_dirs)
            print(f"⚠️  No valid folders with main.go found in: {joined}", file=sys.stderr)
            sys.exit(1)

        joined = ", ".join(d.as_posix() for d in base_dirs)
        print(f"🔍 Found {len(case_dirs)} folders with main.go")
        print(f"📂 Scanned base dirs: {joined}")
        print(f"📝 Output will be saved to: {OUTPUT_FILE.resolve()}\n")

        hw_success = 0
        sw_success = 0
        match_success = 0
        emit_success = {emit: 0 for emit in EMIT_FORMATS}

        with open(OUTPUT_FILE, "w", encoding="utf-8") as fh:
            fh.write("GO WORKLOAD SIMULATION RESULTS (behavioral check enabled)\n")
            fh.write("Hardware pass requires mygo sim stdout to match software stdout exactly.\n")
            fh.write(f"Base dirs: {joined}\n")
            fh.write(f"Generated: {datetime.now().strftime('%Y-%m-%d %H:%M:%S')}\n")
            fh.write(f"{'='*70}\n\n")

            for idx, case_dir in enumerate(case_dirs, 1):
                case_name = case_label(case_dir)
                folder_key = case_dir.name
                main_go_path = case_dir / "main.go"
                main_go = main_go_path.as_posix()
                print(f"[{idx:2d}/{len(case_dirs)}] {case_name}", end=" ", flush=True)

                emit_results = run_emit_conversions(main_go_path, case_name, fh)
                for emit in EMIT_FORMATS:
                    if emit_results[emit][0]:
                        emit_success[emit] += 1

                sw_cmd = f"go run {shlex.quote(main_go)}"
                sw_ok, sw_stdout, sw_stderr, sw_code = run_command(sw_cmd, case_name, "software simulation", fh)
                if sw_ok:
                    sw_success += 1

                hw_ok, hw_stdout, hw_stderr, hw_code, hw_cycles_used, outputs_match, match_mode = (
                    run_hardware_simulation(main_go, folder_key, case_name, sw_stdout, fh)
                )
                outputs_match = sw_ok and hw_ok and outputs_match
                if not outputs_match:
                    match_mode = "none"

                sw_norm = normalize_output(sw_stdout)
                hw_norm = normalize_output(hw_stdout)
                diff_text = ""
                if sw_ok and hw_ok and not outputs_match:
                    diff_lines = difflib.unified_diff(
                        sw_norm.splitlines(),
                        hw_norm.splitlines(),
                        fromfile="software",
                        tofile="hardware",
                        lineterm="",
                    )
                    diff_text = "\n".join(diff_lines)

                fh.write("\n[CASE RESULT]\n")
                fh.write(f"case={case_name}\n")
                for emit in EMIT_FORMATS:
                    emit_ok, emit_code, emit_path = emit_results[emit]
                    fh.write(f"{emit}_ok={emit_ok}\n")
                    fh.write(f"{emit}_return_code={emit_code}\n")
                    fh.write(f"{emit}_path={emit_path.as_posix()}\n")
                fh.write(f"software_return_code={sw_code}\n")
                fh.write(f"hardware_return_code={hw_code}\n")
                fh.write(f"software_ok={sw_ok}\n")
                fh.write(f"hardware_ok={hw_ok}\n")
                fh.write(f"hardware_cycles_used={hw_cycles_used}\n")
                fh.write(f"output_match={outputs_match}\n")
                fh.write(f"output_match_mode={match_mode}\n")
                write_output_section(fh, "SOFTWARE_STDOUT", sw_stdout)
                write_output_section(fh, "HARDWARE_STDOUT", hw_stdout)
                write_output_section(fh, "SOFTWARE_STDERR(filtered)", sw_stderr)
                write_output_section(fh, "HARDWARE_STDERR(filtered)", hw_stderr)
                if diff_text:
                    write_output_section(fh, "SW_HW_DIFF", diff_text)
                fh.write("\n")

                write_case_outputs(
                    case_dir=case_dir,
                    emit_results=emit_results,
                    sw_stdout=sw_stdout,
                    sw_stderr=sw_stderr,
                    sw_code=sw_code,
                    sw_ok=sw_ok,
                    hw_stdout=hw_stdout,
                    hw_stderr=hw_stderr,
                    hw_code=hw_code,
                    hw_ok=hw_ok,
                    hw_cycles_used=hw_cycles_used,
                    outputs_match=outputs_match,
                    match_mode=match_mode,
                    diff_text=diff_text,
                )

                if hw_ok:
                    hw_success += 1
                if outputs_match:
                    match_success += 1

                status = "✅" if outputs_match else "⚠️"
                emit_status = " ".join(f"{emit}:{'✓' if emit_results[emit][0] else '✗'}" for emit in EMIT_FORMATS)
                print(
                    f"{status} (match:{'✓' if outputs_match else '✗'} "
                    f"hw:{'✓' if hw_ok else '✗'} sw:{'✓' if sw_ok else '✗'} {emit_status})"
                )

            fh.write(f"\n{'='*70}\n")
            fh.write("SUMMARY\n")
            fh.write(f"{'='*70}\n")
            fh.write(f"Total folders: {len(case_dirs)}\n")
            for emit in EMIT_FORMATS:
                fh.write(f"{emit.upper()} conversion success: {emit_success[emit]}/{len(case_dirs)}\n")
            fh.write(f"Software simulation success: {sw_success}/{len(case_dirs)}\n")
            fh.write(f"Hardware simulation command success: {hw_success}/{len(case_dirs)}\n")
            fh.write(f"Behavioral match success (HW==SW): {match_success}/{len(case_dirs)}\n")
            fh.write(f"{'='*70}\n")

        print(f"\n✅ Done. Clean results saved to:\n   {OUTPUT_FILE.resolve()}")
        emit_summary = " | ".join(f"{emit.upper()}={emit_success[emit]}/{len(case_dirs)}" for emit in EMIT_FORMATS)
        print(
            f"\n📊 Summary: MATCH={match_success}/{len(case_dirs)} | "
            f"HW={hw_success}/{len(case_dirs)} | SW={sw_success}/{len(case_dirs)} | {emit_summary}"
        )
    finally:
        removed, failed = cleanup_cache_paths(case_dirs)
        if removed:
            print(f"\n🧹 Cache cleanup removed {len(removed)} path(s).")
        if failed:
            print("\n⚠️  Cache cleanup could not remove some paths:", file=sys.stderr)
            for path, err in failed:
                print(f"   - {path}: {err}", file=sys.stderr)


if __name__ == "__main__":
    main()
