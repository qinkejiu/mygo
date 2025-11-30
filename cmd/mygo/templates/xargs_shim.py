#!/usr/bin/env python3
import subprocess
import sys


def main():
    argv = sys.argv[1:]
    data = sys.stdin.read().split()
    if not data:
        return 0
    cmd = argv + data
    try:
        completed = subprocess.run(cmd, check=False)
        return completed.returncode
    except FileNotFoundError as exc:
        sys.stderr.write(f"xargs shim could not find command: {exc}\n")
        return 127


if __name__ == "__main__":
    sys.exit(main())
