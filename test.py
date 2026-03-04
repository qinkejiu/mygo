#!/usr/bin/env python3
import difflib
import os
import shlex
import subprocess
import sys
from datetime import datetime
from pathlib import Path

BASE_DIR = Path("tests/CHStone")
OUTPUT_FILE = Path("CHS_clean_simulation_results.txt")
COMMAND_TIMEOUT_SEC = 600
DEFAULT_SIM_MAX_CYCLES = 20000

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
    env["GOCACHE"] = "/tmp/go-build"
    env["GOPATH"] = "/tmp/go"
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


def normalize_output(text: str) -> str:
    lines = [line.rstrip() for line in text.splitlines()]
    return "\n".join(lines).strip()


def write_output_section(output_fh, title: str, body: str) -> None:
    output_fh.write(f"\n[{title}]\n")
    if body.strip():
        output_fh.write(body)
        if not body.endswith("\n"):
            output_fh.write("\n")
    else:
        output_fh.write("(no output)\n")


def main():
    if not BASE_DIR.exists():
        print(f"❌ Error: '{BASE_DIR}' not found", file=sys.stderr)
        sys.exit(1)

    folders = sorted(d.name for d in BASE_DIR.iterdir() if d.is_dir() and (d / "main.go").exists())
    if not folders:
        print(f"⚠️  No valid folders with main.go found in {BASE_DIR}", file=sys.stderr)
        sys.exit(1)

    print(f"🔍 Found {len(folders)} folders with main.go")
    print(f"📝 Output will be saved to: {OUTPUT_FILE.resolve()}\n")

    hw_success = 0
    sw_success = 0
    match_success = 0

    with open(OUTPUT_FILE, "w", encoding="utf-8") as fh:
        fh.write("CHSTONE SIMULATION RESULTS (behavioral check enabled)\n")
        fh.write("Hardware pass requires mygo sim stdout to match software stdout exactly.\n")
        fh.write(f"Generated: {datetime.now().strftime('%Y-%m-%d %H:%M:%S')}\n")
        fh.write(f"{'='*70}\n\n")

        for idx, folder in enumerate(folders, 1):
            main_go = f"tests/CHStone/{folder}/main.go"
            print(f"[{idx:2d}/{len(folders)}] {folder:20s}", end=" ", flush=True)

            sw_cmd = f"go run {shlex.quote(main_go)}"
            sw_ok, sw_stdout, sw_stderr, sw_code = run_command(sw_cmd, folder, "software simulation", fh)
            if sw_ok:
                sw_success += 1

            max_cycles = SIM_MAX_CYCLES.get(folder, DEFAULT_SIM_MAX_CYCLES)
            hw_cmd = f"go run ./cmd/mygo sim --sim-max-cycles {max_cycles} {shlex.quote(main_go)}"
            hw_ok, hw_stdout, hw_stderr, hw_code = run_command(hw_cmd, folder, "hardware simulation", fh)
            if hw_ok:
                lowered = hw_stderr.lower()
                if any(marker in lowered for marker in HARDWARE_UNSUPPORTED_MARKERS):
                    hw_ok = False
                    fh.write("[CHECK]\n")
                    fh.write("Hardware run exited 0 but hit unsupported lowering markers; counting as FAIL.\n")

            sw_norm = normalize_output(sw_stdout)
            hw_norm = normalize_output(hw_stdout)
            outputs_match = sw_ok and hw_ok and sw_norm == hw_norm
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
            fh.write(f"software_return_code={sw_code}\n")
            fh.write(f"hardware_return_code={hw_code}\n")
            fh.write(f"software_ok={sw_ok}\n")
            fh.write(f"hardware_ok={hw_ok}\n")
            fh.write(f"output_match={outputs_match}\n")
            write_output_section(fh, "SOFTWARE_STDOUT", sw_stdout)
            write_output_section(fh, "HARDWARE_STDOUT", hw_stdout)
            write_output_section(fh, "SOFTWARE_STDERR(filtered)", sw_stderr)
            write_output_section(fh, "HARDWARE_STDERR(filtered)", hw_stderr)
            if diff_text:
                write_output_section(fh, "SW_HW_DIFF", diff_text)
            fh.write("\n")

            if hw_ok:
                hw_success += 1
            if outputs_match:
                match_success += 1

            status = "✅" if outputs_match else "⚠️"
            print(
                f"{status} (match:{'✓' if outputs_match else '✗'} "
                f"hw:{'✓' if hw_ok else '✗'} sw:{'✓' if sw_ok else '✗'})"
            )

        fh.write(f"\n{'='*70}\n")
        fh.write("SUMMARY\n")
        fh.write(f"{'='*70}\n")
        fh.write(f"Total folders: {len(folders)}\n")
        fh.write(f"Software simulation success: {sw_success}/{len(folders)}\n")
        fh.write(f"Hardware simulation command success: {hw_success}/{len(folders)}\n")
        fh.write(f"Behavioral match success (HW==SW): {match_success}/{len(folders)}\n")
        fh.write(f"{'='*70}\n")

    print(f"\n✅ Done. Clean results saved to:\n   {OUTPUT_FILE.resolve()}")
    print(
        f"\n📊 Summary: MATCH={match_success}/{len(folders)} | "
        f"HW={hw_success}/{len(folders)} | SW={sw_success}/{len(folders)}"
    )


if __name__ == "__main__":
    main()
