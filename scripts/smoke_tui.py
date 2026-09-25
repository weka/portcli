#!/usr/bin/env python3
"""Smoke-test the TUI in a real pseudo-terminal.

A tcell SimulationScreen tolerates mistakes a real terminal does not — a
double Init, for one, which renders almost nothing and swallows every key
including Ctrl-C. Only a real pty catches that, so this exists alongside the
Go tests rather than instead of them.

Usage: scripts/smoke_tui.py [path-to-binary] [keys-to-send]
Needs working Port credentials. Exits non-zero on failure.
"""
import fcntl
import os
import pty
import re
import select
import struct
import sys
import termios
import time

BINARY = sys.argv[1] if len(sys.argv) > 1 else "./portcli"
KEYS = sys.argv[2] if len(sys.argv) > 2 else "q"
EXPECTED = ["Base URL", "Auth", "Resource", "Refresh", "IDENTIFIER"]


def main() -> int:
    pid, fd = pty.fork()
    if pid == 0:
        os.environ["TERM"] = "xterm-256color"
        os.execv(BINARY, [BINARY, "tui", "--view", "blueprints"])

    fcntl.ioctl(fd, termios.TIOCSWINSZ, struct.pack("HHHH", 30, 120, 0, 0))
    buf = bytearray()

    def drain(seconds: float) -> None:
        end = time.time() + seconds
        while time.time() < end:
            ready, _, _ = select.select([fd], [], [], 0.2)
            if fd in ready:
                try:
                    chunk = os.read(fd, 65536)
                except OSError:
                    return
                if not chunk:
                    return
                buf.extend(chunk)

    drain(6)
    raw = bytes(buf)
    text = re.sub(rb"\x1b\][^\x07\x1b]*(\x07|\x1b\\)", b"", raw)
    text = re.sub(rb"\x1b\[[0-9;?]*[ -/]*[@-~]", b"", text).decode("utf-8", "replace")

    failures = []
    if len(raw) < 2000:
        failures.append(f"only {len(raw)} bytes drawn — the screen is essentially blank")
    for marker in EXPECTED:
        if marker not in text:
            failures.append(f"missing {marker!r} on screen")

    # Keep draining while waiting to exit: a full pty buffer blocks the app's
    # writes and would otherwise look identical to a hang.
    for key in KEYS:
        os.write(fd, key.encode())
        time.sleep(0.3)
    status, end = None, time.time() + 8
    while time.time() < end:
        drain(0.3)
        waited, st = os.waitpid(pid, os.WNOHANG)
        if waited:
            status = st
            break
    # Read to EOF after the child is gone: the sequence that restores the
    # terminal is written on the way out, so reaping before draining loses
    # exactly the bytes this check is looking for.
    if status is not None:
        while True:
            try:
                ready, _, _ = select.select([fd], [], [], 0.3)
                if fd not in ready:
                    break
                chunk = os.read(fd, 65536)
            except OSError:
                break
            if not chunk:
                break
            buf.extend(chunk)
        raw = bytes(buf)

    if status is None:
        failures.append(f"did not exit after {KEYS!r} — keys are not reaching the app")
        os.kill(pid, 9)
    elif os.waitstatus_to_exitcode(status) not in (0, 130):
        failures.append(f"exited with status {status}")

    if b"?1049l" not in raw:
        failures.append("never left the alternate screen — the terminal is left broken")

    for f in failures:
        print("FAIL:", f)
    if failures:
        return 1
    print(f"ok: {len(raw)} bytes drawn, all markers present, exited cleanly on {KEYS!r}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
