#!/usr/bin/env python3
"""Push NetLynx (UDP/TCP 514) as remote syslog on switches/routers.

Vendors: Ubiquiti EdgeSwitch, SNR, Eltex MES, MikroTik RouterOS.
Uses PuTTY plink (legacy SSH) when available; falls back to OpenSSH + sshpass.

Usage:
  python3 docs/configure-device-syslog.py --target 10.0.0.1 --inventory devices.tsv

Inventory TSV: host\\tvendor\\tuser\\tpassword\\tenable\\tport
  vendor: edgeswitch|snr|eltex|mikrotik
"""
from __future__ import annotations

import argparse
import os
import shutil
import subprocess
import sys
import time
from dataclasses import dataclass


@dataclass
class Dev:
    host: str
    vendor: str
    user: str
    password: str
    enable: str = ""
    port: int = 22


def which_plink() -> str | None:
    return shutil.which("plink") or (r"C:\Windows\plink.exe" if os.path.isfile(r"C:\Windows\plink.exe") else None)


def plink_session(d: Dev, lines: list[str], settle: float = 0.45) -> str:
    """Interactive CLI over plink (handles ssh-rsa / old KEX)."""
    plink = which_plink()
    if not plink:
        raise RuntimeError("plink not found")

    # Accept host key once (non-batch).
    pre = subprocess.run(
        [
            plink,
            "-ssh",
            "-P",
            str(d.port),
            "-pw",
            d.password,
            f"{d.user}@{d.host}",
            "exit",
        ],
        input="y\n",
        capture_output=True,
        text=True,
        timeout=25,
    )
    _ = pre

    script = "\n".join(lines) + "\n"
    # Delay between lines via plink -m is instant; use a small expect-like by feeding slowly via stdin.
    proc = subprocess.Popen(
        [
            plink,
            "-ssh",
            "-batch",
            "-P",
            str(d.port),
            "-pw",
            d.password,
            f"{d.user}@{d.host}",
        ],
        stdin=subprocess.PIPE,
        stdout=subprocess.PIPE,
        stderr=subprocess.STDOUT,
        text=True,
    )
    assert proc.stdin and proc.stdout
    out_chunks: list[str] = []
    try:
        for line in lines:
            proc.stdin.write(line + "\n")
            proc.stdin.flush()
            time.sleep(settle)
            # non-blocking-ish drain
            if proc.poll() is not None:
                break
        # confirm write memory
        time.sleep(0.8)
        proc.stdin.write("y\n")
        proc.stdin.flush()
        time.sleep(1.0)
        proc.stdin.write("exit\n")
        proc.stdin.flush()
        try:
            proc.stdin.close()
        except Exception:
            pass
        try:
            rest, _ = proc.communicate(timeout=20)
            out_chunks.append(rest or "")
        except subprocess.TimeoutExpired:
            proc.kill()
            rest, _ = proc.communicate()
            out_chunks.append(rest or "")
    finally:
        if proc.poll() is None:
            proc.kill()
    return "".join(out_chunks)


def plink_exec(d: Dev, remote_cmd: str) -> str:
    plink = which_plink()
    if not plink:
        raise RuntimeError("plink not found")
    subprocess.run(
        [plink, "-ssh", "-P", str(d.port), "-pw", d.password, f"{d.user}@{d.host}", "exit"],
        input="y\n",
        capture_output=True,
        text=True,
        timeout=25,
    )
    r = subprocess.run(
        [
            plink,
            "-ssh",
            "-batch",
            "-P",
            str(d.port),
            "-pw",
            d.password,
            f"{d.user}@{d.host}",
            remote_cmd,
        ],
        capture_output=True,
        text=True,
        timeout=60,
    )
    return (r.stdout or "") + (r.stderr or "")


def configure_edgeswitch(d: Dev, target: str) -> str:
    # EdgeSwitch FastPath: logging host "<ip>" ipv4 <port> <severity>
    en = d.enable or d.password
    lines = [
        "enable",
        en,
        "configure",
        "logging syslog",
        f'logging host "{target}" ipv4 514 debug',
        "logging buffered 7",
        "logging console 6",
        "logging traps 7",
        "exit",
        "write memory",
        "y",
        "show logging hosts",
        "exit",
    ]
    return plink_session(d, lines, settle=0.55)


def configure_snr(d: Dev, target: str) -> str:
    # SNR / H3C-style: numbered loghost slots (1..8). Use slot 2 by default
    # (slot 1 often already points at an older collector).
    src = d.host
    lines = [
        "enable",
        d.enable or d.password,
        "configure terminal",
        f"info-center loghost 2 config {target} facility local0 source-ip {src}",
        "info-center loghost 2 output-enable",
        "info-center loghost 2 match level debugging",
        "info-center loghost 2 record-cmd",
        "end",
        "write",
        "y",
        "show running-config | include info-center",
        "exit",
    ]
    return plink_session(d, lines, settle=0.55)


def configure_eltex(d: Dev, target: str) -> str:
    # MES: "logging host A.B.C.D" (NOT "logging host ipv4 …" — that triggers DNS lookup).
    lines = [
        "configure",
        "logging on",
        f"logging host {target}",
        f"logging host {target} port 514",
        "logging buffered debugging",
        "exit",
        "write",
        "y",
        "show running-config | include logging",
        "exit",
    ]
    return plink_session(d, lines, settle=0.55)


def configure_mikrotik(d: Dev, target: str) -> str:
    chunks: list[str] = []
    # Idempotent-ish: remove old action/rules named netlynx then recreate.
    # RouterOS 6 uses bsd-syslog=yes; ROS 7 prefers remote-log-format=syslog.
    cmds = [
        ":do { /system logging remove [find where action=netlynx] } on-error={}",
        ":do { /system logging action remove [find where name=netlynx] } on-error={}",
        (
            f"/system logging action add name=netlynx target=remote remote={target} "
            f"remote-port=514 remote-log-format=syslog remote-protocol=udp syslog-facility=local0"
        ),
    ]
    for top in ("critical", "error", "warning", "info", "interface", "system", "bridge", "stp", "dhcp", "firewall"):
        cmds.append(f"/system logging add action=netlynx topics={top}")
    cmds.append("/system logging action print where name=netlynx")
    for cmd in cmds:
        chunks.append(plink_exec(d, cmd))
    return "\n".join(chunks)


HANDLERS = {
    "edgeswitch": configure_edgeswitch,
    "ubiquiti": configure_edgeswitch,
    "snr": configure_snr,
    "eltex": configure_eltex,
    "mikrotik": configure_mikrotik,
    "routeros": configure_mikrotik,
}


def load_inventory(path: str) -> list[Dev]:
    out: list[Dev] = []
    with open(path, encoding="utf-8") as f:
        for line_no, line in enumerate(f, 1):
            line = line.strip()
            if not line or line.startswith("#"):
                continue
            parts = line.split("\t")
            if len(parts) < 4:
                raise SystemExit(f"inventory line {line_no}: need host vendor user password [enable [port]]")
            port = int(parts[5]) if len(parts) > 5 and parts[5] else 22
            out.append(
                Dev(
                    host=parts[0].strip(),
                    vendor=parts[1].strip().lower(),
                    user=parts[2].strip(),
                    password=parts[3],
                    enable=parts[4] if len(parts) > 4 else "",
                    port=port,
                )
            )
    return out


def main() -> int:
    if not which_plink():
        print("ERROR: plink.exe required (PuTTY) for legacy SSH to switches", file=sys.stderr)
        return 2
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument("--target", required=True, help="NetLynx syslog IP")
    ap.add_argument("--inventory", help="TSV inventory file")
    ap.add_argument("--host")
    ap.add_argument("--vendor")
    ap.add_argument("--user")
    ap.add_argument("--password")
    ap.add_argument("--enable-password", default="")
    ap.add_argument("--port", type=int, default=22)
    ap.add_argument("--dry-run", action="store_true")
    args = ap.parse_args()

    if args.inventory:
        devices = load_inventory(args.inventory)
    elif args.host and args.vendor and args.user and args.password is not None:
        devices = [Dev(args.host, args.vendor.lower(), args.user, args.password, args.enable_password, args.port)]
    else:
        ap.error("provide --inventory or --host/--vendor/--user/--password")

    rc = 0
    for d in devices:
        fn = HANDLERS.get(d.vendor)
        if not fn:
            print(f"SKIP {d.host}: unknown vendor {d.vendor!r}", flush=True)
            rc = 1
            continue
        print(f"==> {d.host} ({d.vendor}) -> {args.target}", flush=True)
        if args.dry_run:
            continue
        try:
            text = fn(d, args.target)
            ok_hints = ("logging host", "loghost", "netlynx", "Logging", "info-center", "remote", "trap")
            hit = any(h.lower() in text.lower() for h in ok_hints)
            print(("OK" if hit else "DONE") + f" {d.host} ({len(text)} bytes)", flush=True)
        except Exception as e:
            print(f"FAIL {d.host}: {e}", flush=True)
            rc = 1
    return rc


if __name__ == "__main__":
    sys.exit(main())
