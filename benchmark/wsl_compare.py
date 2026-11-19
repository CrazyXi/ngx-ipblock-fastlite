"""Run as root inside a fresh WSL network + private mount namespace.

unshare -nm --propagation private python3 benchmark/wsl_compare.py NEW_WORK_DIR SOURCE_DIR
SOURCE_DIR contains the pinned upstream files as 30d.txt, 180d.txt and 365d.txt.
Requires APISIX's Debian package, wrk, iproute2, nftables, iptables and ipset.
"""
import argparse
import hashlib
import ipaddress
import json
import os
import re
import shutil
import struct
import subprocess as sp
import sys
import time
from pathlib import Path

repo = Path(__file__).resolve().parents[1]
sizes = [(150000, 30), (400000, 180), (600000, 365)]
engines = ("plain", "fastlite", "geo", "matcher", "ipset", "nft", "apisix-off", "apisix-on")
parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument("work", type=Path)
parser.add_argument("source", type=Path)
parser.add_argument("--only", action="append", choices=[f"{n}:{key}" for n, _ in sizes for key in engines],
                    help="Repeat a specific size and engine (repeatable)")
args = parser.parse_args()
work = args.work.resolve()
source = args.source.resolve()
if os.readlink("/proc/self/ns/net") == os.readlink("/proc/1/ns/net") or os.readlink("/proc/self/ns/mnt") == os.readlink("/proc/1/ns/mnt"):
    sys.exit("Fresh network and mount namespaces required")
work.mkdir()
os.chdir(work)
nginx = "/usr/local/openresty/nginx/sbin/nginx"
api = Path("/usr/local/apisix")
deps = api / "deps/share/lua/5.1"
os.environ["PATH"] = "/usr/local/openresty/bin:" + os.environ["PATH"]


def run(*args, data=None):
    return sp.check_output(args, input=data, text=True, stderr=sp.STDOUT)


def probe(addr, port=18080):
    p = sp.run(["curl", "--noproxy", "*", "-so", "/dev/null", "--max-time", "0.4",
                "-w", "%{http_code}", f"http://{addr}:{port}/"], capture_output=True, text=True)
    return p.returncode, p.stdout


def server(conf, folder, cpu):
    folder.mkdir(exist_ok=True)
    (folder / "logs").mkdir(exist_ok=True)
    (folder / "nginx.conf").write_text(conf)
    return sp.Popen(["taskset", "-c", cpu, nginx, "-p", str(folder) + "/", "-c", "nginx.conf",
                     "-g", "daemon off;"], stdout=sys.stderr)


def pss(pid):
    children = Path(f"/proc/{pid}/task/{pid}/children").read_text().split()
    return sum(int(re.search(r"^Pss:\s+(\d+)", Path(f"/proc/{p}/smaps_rollup").read_text(), re.M)[1])
               for p in [pid, *children])


def cpu_seconds(pid):
    children = Path(f"/proc/{pid}/task/{pid}/children").read_text().split()
    ticks = 0
    for p in [pid, *children]:
        fields = Path(f"/proc/{p}/stat").read_text().rsplit(")", 1)[1].split()
        ticks += int(fields[11]) + int(fields[12])
    return ticks / os.sysconf("SC_CLK_TCK")


def bench(pid, addr="127.0.0.1", reject=False):
    before = cpu_seconds(pid)
    raw = run("taskset", "-c", "3,4", "wrk", "-t2", "-c32", "-d3s", "--latency", f"http://{addr}:18080/")
    errors = re.search(r"Non-2xx or 3xx responses: (\d+)", raw)
    total = int(re.search(r"(\d+) requests in", raw)[1])
    if not total or int(errors[1] if errors else 0) != (total if reject else 0) or "Socket errors" in raw:
        raise ValueError(raw)
    return {"rps": float(re.search(r"Requests/sec:\s+([\d.]+)", raw)[1]),
            "server_cpu_us_per_request": (cpu_seconds(pid) - before) * 1e6 / total,
            "p99": re.search(r"^\s*99%[ \t]+([\d.]+(?:us|ms|s))\s*$", raw, re.M)[1], "raw": raw}


def gateway(ips, enabled):
    (api / "conf/config.yaml").write_text("""apisix:
  node_listen: 18080
  enable_admin: false
  enable_control: false
  ssl:
    enable: false
nginx_config:
  worker_processes: 1
  enable_cpu_affinity: false
  error_log_level: crit
  http:
    enable_access_log: false
deployment:
  role: data_plane
  role_data_plane:
    config_provider: yaml
plugins:
  - ip-restriction
""")
    route = {"id": "bench", "uri": "/", "upstream": {"type": "roundrobin", "nodes": {"127.0.0.1:18081": 1}}}
    if enabled:
        route["plugins"] = {"ip-restriction": {"blacklist": ips}}
    (api / "conf/apisix.yaml").write_text(json.dumps({"routes": [route]}) + "\n#END\n")
    run("taskset", "-c", "0", "apisix", "init")
    return sp.Popen(["taskset", "-c", "0", nginx, "-p", str(api) + "/", "-c", "conf/nginx.conf",
                     "-g", "daemon off;"], stdout=sys.stderr)


def measure(kind, ips, directory):
    folder = directory / ("case-" + kind)
    folder.mkdir()
    proc = None
    kernel = kind in ("ipset", "nft")
    row = {"size": len(ips), "case": kind}
    begin = time.monotonic()
    try:
        extra = ""
        hook = ""
        if kind == "geo":
            extra = f"geo $blocked {{ default 0; include {directory}/geo; }}"
            hook = "if ($blocked) { return 403; }"
        if kind == "fastlite":
            extra = f"""lua_package_path "{repo}/lua/?.lua;;"; init_by_lua_block {{
              require('ip_blacklist').init({{path='{directory}/fixture.bin', default_site={{enabled=true,block_status=403}}}}) }}"""
            hook = "access_by_lua_block { require('ip_blacklist').access() }"
        if kind == "matcher":
            extra = f"""lua_package_path "{deps}/?.lua;;"; init_by_lua_block {{
              local ips={{}}; for line in io.lines('{directory}/ips') do ips[#ips+1]=line end
              package.loaded.bench_matcher=assert(require('resty.ipmatcher').new(ips)); ips=nil; collectgarbage('collect') }}"""
            hook = "access_by_lua_block { if package.loaded.bench_matcher:match(ngx.var.remote_addr) then return ngx.exit(403) end }"
        if kind == "ipset":
            run("ipset", "restore", data="create blocked hash:ip maxelem 600000\n" +
                "".join("add blocked " + x + "\n" for x in ips))
            run("iptables", "-A", "INPUT", "-p", "tcp", "--dport", "18080", "-m", "set",
                "--match-set", "blocked", "src", "-j", "DROP")
            row["kernel_set"] = run("ipset", "list", "-terse", "blocked")
        if kind == "nft":
            run("nft", "-f", "-", data="add table ip bench\nadd set ip bench blocked { type ipv4_addr; }\n" +
                "".join("add element ip bench blocked { " + ", ".join(ips[i:i+512]) + " }\n"
                        for i in range(0, len(ips), 512)) +
                "add chain ip bench input { type filter hook input priority 0; policy accept; }\n"
                "add rule ip bench input tcp dport 18080 ip saddr @blocked counter drop\n")
        if kind.startswith("apisix"):
            proc = gateway(ips, kind == "apisix-on")
        else:
            proc = server(f"""worker_processes 1; error_log logs/error.log crit; events {{ worker_connections 1024; }}
http {{ access_log off; {extra}
upstream backend {{ server 127.0.0.1:18081; keepalive 32; }}
server {{ listen 18080; location / {{ {hook} proxy_http_version 1.1; proxy_set_header Connection "";
proxy_pass http://backend; }} }} }}""", folder, "0")
        for _ in range(300):
            if proc.poll() is not None:
                raise RuntimeError("server exited")
            if probe("127.0.0.1") == (0, "204"):
                break
            time.sleep(.02)
        else:
            raise RuntimeError("readiness timeout")
        row["activate_s"] = round(time.monotonic() - begin, 3)
        expected = (28, "000") if kernel else (0, "204" if kind in ("plain", "apisix-off") else "403")
        for hit in [ips[0], ips[-1]]:
            actual = probe(hit)
            if actual != expected:
                raise ValueError((hit, actual, expected))
        if kernel:
            row["hit_counter"] = run("nft", "list", "chain", "ip", "bench", "input") if kind == "nft" else run("iptables", "-nvxL", "INPUT")
        if kind == "ipset":
            assert f"Number of entries: {len(ips)}" in row["kernel_set"]
        if kind == "nft":
            loaded = json.loads(run("nft", "-j", "list", "set", "ip", "bench", "blocked"))
            assert sum(len(x["set"]["elem"]) for x in loaded["nftables"] if "set" in x) == len(ips)
        bench(proc.pid)
        row["samples"] = [bench(proc.pid) for _ in range(3)]
        if expected == (0, "403"):
            row["hits"] = [bench(proc.pid, ips[0], True) for _ in range(3)]
        row["pss_kib"] = pss(proc.pid)
        set_bytes = 0
        if kind == "ipset":
            size_match = re.search(r"^Size in memory:\s*(\d+)\s*$", row["kernel_set"], re.M)
            set_bytes = int(size_match[1]) if size_match else None
        elif kind == "nft":
            set_bytes = None  # The set listing provides no measured byte count.
        row["kernel_blocklist_bytes"] = set_bytes
        row["frontend_and_set_bytes"] = (
            row["pss_kib"] * 1024 + set_bytes if set_bytes is not None else None
        )
    except Exception as exc:
        row["error"] = str(exc) + str(getattr(exc, "output", ""))[-1200:]
    finally:
        if proc and proc.poll() is None:
            proc.terminate()
            proc.wait(timeout=10)
        if kind == "ipset":
            sp.run(["iptables", "-D", "INPUT", "-p", "tcp", "--dport", "18080",
                    "-m", "set", "--match-set", "blocked", "src", "-j", "DROP"])
            sp.run(["ipset", "destroy", "blocked"])
        if kind == "nft":
            sp.run(["nft", "delete", "table", "ip", "bench"])
    with (work/"samples.jsonl").open("a") as f:
        f.write(json.dumps(row) + "\n")
    print(len(ips), kind, row.get("error", [s["rps"] for s in row.get("samples", [])]), flush=True)


run("ip", "link", "set", "lo", "up")
shutil.copytree(api, work/"api", symlinks=True)
run("mount", "--bind", str(work/"api"), str(api))
backend = server("worker_processes 1; events {} http { access_log off; server { listen 127.0.0.1:18081; return 204; } }", work/"backend", "2")
try:
    for n, days in sizes:
        selected = [key for key in engines if not args.only or f"{n}:{key}" in args.only]
        if not selected:
            continue
        directory = work/str(n)
        directory.mkdir()
        src = (source/f"{days}d.txt").read_bytes()
        blob_id = hashlib.sha1(f"blob {len(src)}\0".encode() + src).hexdigest()
        expected = {30:"fb56657b1d63e09acf0a24df7c3895d5c7079730",
                    180:"9ee370703650334b19f32a5b5bb9271a645e3fc5",
                    365:"83a9fca231622a71a6da5a9ac8d191807be39618"}
        if blob_id != expected[days]:
            raise ValueError(f"{days}d source differs from upstream eb4a4a4: {blob_id}")
        rows = (line.partition(b"#")[0].strip() for line in src.splitlines())
        pool = {ipaddress.IPv4Address(ip.decode()).packed for ip in rows if ip}
        declared = int(re.search(rb"# Number of ips:\s*(\d+)", src)[1])
        assert len(pool) == declared and len(pool) >= n
        subset = sorted(pool, key=lambda ip: (hashlib.sha256(ip).digest(), ip))[:n]
        values = sorted(int.from_bytes(ip, "big") for ip in subset)
        payload = struct.pack(f"<{n}I", *values)
        blob = struct.pack("<8sIIQQ", b"IPBLV001", 1, 64, n, 0) + hashlib.sha256(payload).digest() + payload
        with (work/"sources.jsonl").open("a") as f:
            f.write(json.dumps(dict(size=n,days=days,unique=len(pool),source_sha256=hashlib.sha256(src).hexdigest(),
                                    payload_sha256=hashlib.sha256(payload).hexdigest()))+"\n")
        del pool, subset, values, src
        ips = [str(ipaddress.IPv4Address(x)) for x, in struct.iter_unpack("<I", blob[64:])]
        assert len(ips) == n and "127.0.0.1" not in ips
        (directory/"fixture.bin").write_bytes(blob)
        (directory/"ips").write_text("\n".join(ips)+"\n")
        (directory/"geo").write_text("".join(ip+" 1;\n" for ip in ips))
        for hit in (ips[0], ips[-1]):
            run("ip", "addr", "add", hit+"/32", "dev", "lo")
        for kind in selected:
            measure(kind, ips, directory)
        for hit in (ips[0], ips[-1]):
            run("ip", "addr", "del", hit+"/32", "dev", "lo")
finally:
    backend.terminate()
    backend.wait()
