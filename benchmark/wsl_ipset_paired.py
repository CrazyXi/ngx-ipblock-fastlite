"""Run with unshare -n python3 benchmark/wsl_ipset_paired.py LIST_DIR NEW_JSONL [SIZE].

LIST_DIR/{150000,400000,600000}/ips must match the archived subset hashes.
"""
import hashlib
import json
import os
import re
import socket
import subprocess as sp
import sys
import tempfile
import time
from contextlib import ExitStack
from pathlib import Path


def run(*args, **kw):
    return sp.check_output(args, text=True, **kw)


def probe(addr, src):
    res = sp.run(["curl", "--noproxy", "*", "--interface", src, "-s", "-o", "/dev/null",
                  "-w", "%{http_code}", "--max-time", "0.5", f"http://{addr}:18080/"],
                 text=True, capture_output=True)
    return res.returncode, res.stdout


def bench(seconds):
    raw = run("taskset", "-c", "3,4", "wrk", "-t2", "-c32", f"-d{seconds}s",
              "--latency", "http://127.0.0.1:18080/")
    assert "Socket errors" not in raw and "Non-2xx or 3xx" not in raw
    return raw


def counts():
    rules = run("iptables", "-nvxL", "INPUT")
    return [int(x) for x in re.findall(r"^\s*(\d+)\s+\d+\s+(?:ACCEPT|DROP)\s+", rules, re.M)]


def stop(p):
    p.terminate()
    p.wait(timeout=5)


if os.readlink("/proc/self/ns/net") == os.readlink("/proc/1/ns/net"):
    sys.exit("fresh network namespace required")
source, dest = map(Path, sys.argv[1:3])
only = int(sys.argv[3]) if len(sys.argv) == 4 else None
manifest = Path(__file__).parent / "results/wsl-sources.jsonl"
with tempfile.TemporaryDirectory() as tmp, ExitStack() as stack, dest.open("x") as out:
    run("ip", "link", "set", "lo", "up")
    assert not any(l.startswith("-A") for l in run("iptables", "-S").splitlines())
    for cpu, port, location in [(2, 18081, "return 204;"), (0, 18080,
            'proxy_http_version 1.1; proxy_set_header Connection ""; proxy_pass http://backend;')]:
        d = Path(tmp) / str(port)
        (d / "logs").mkdir(parents=True)
        upstream = "upstream backend { server 127.0.0.1:18081; keepalive 32; }" if cpu == 0 else ""
        (d / "nginx.conf").write_text(f"worker_processes 1; events {{ worker_connections 1024; }} "
            f"http {{ access_log off; {upstream} server {{ listen {port}; location / {{ {location} }} }} }}")
        p = sp.Popen(["taskset", "-c", str(cpu), "/usr/local/openresty/nginx/sbin/nginx", "-p",
                      str(d) + "/", "-c", "nginx.conf", "-g", "daemon off;"])
        stack.callback(stop, p)
    for _ in range(100):
        if probe("127.0.0.1", "127.0.0.1") == (0, "204"):
            break
        time.sleep(0.05)
    for record in map(json.loads, manifest.read_text().splitlines()):
        n = record["size"]
        if only is not None and only != n:
            continue
        ips = (source / str(n) / "ips").read_text().splitlines()
        assert len(ips) == n and "127.0.0.1" not in ips
        assert hashlib.sha256(b"".join(socket.inet_aton(i)[::-1] for i in ips)).hexdigest() == record["payload_sha256"]
        run("ipset", "restore", input="create blocked hash:ip maxelem 600000\n" +
            "".join("add blocked " + ip + "\n" for ip in ips))
        assert f"Number of entries: {n}" in run("ipset", "list", "-terse", "blocked")
        run("ip", "addr", "add", ips[0]+"/32", "dev", "lo")
        for pair in range(4):
            for on in ((False, True) if pair % 2 == 0 else (True, False)):
                # Complementary actions give exact counters and only one set lookup per packet.
                port = ["INPUT", "-p", "tcp", "--dport", "18080"]
                allow = [*port, "-m", "set", "!", "--match-set", "blocked", "src", "-j", "ACCEPT"]
                drop = [*port, "-j", "DROP"]
                if on:
                    run("iptables", "-A", *allow)
                    run("iptables", "-A", *drop)
                assert probe("127.0.0.1", ips[0]) == ((28, "000") if on else (0, "204"))
                assert probe("127.0.0.1", "127.0.0.1") == (0, "204")
                if on:
                    assert all(v > 0 for v in counts()) and len(counts()) == 2
                bench(1)
                before = counts()
                raw = bench(5)
                after = counts()
                delta = [v-u for v,u in zip(after,before)]
                assert (len(delta)==2 and delta[0]>0 and delta[1]==0) if on else before==after==[]
                row = dict(size=n, pair=pair+1, enabled=on, packets_before=before, packets_after=after,
                           rps=float(re.search(r"Requests/sec:\s*([\d.]+)",raw)[1]), raw=raw)
                print(json.dumps(row), file=out, flush=True)
                if on:
                    run("iptables", "-D", *drop)
                    run("iptables", "-D", *allow)
        run("ip", "addr", "del", ips[0]+"/32", "dev", "lo")
        run("ipset", "destroy", "blocked")
        print(n, "done", flush=True)
