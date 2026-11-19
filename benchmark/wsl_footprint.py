"""Measure deployed file bytes and paired nftables memory deltas in an idle WSL VM.

Run after wsl_compare.py: unshare -n python3 wsl_footprint.py RUN_DIR OUTPUT.json
RUN_DIR contains the three numerically sorted ips files (or sample-N.txt).
"""
import json
import os
import statistics
import subprocess
import sys
import tempfile
import time
from pathlib import Path


def file_bytes(paths):
    seen = {}
    for path in paths:
        if path.is_file() and not path.is_symlink():
            info = path.stat()
            seen[info.st_dev, info.st_ino] = info.st_size
    return sum(seen.values())


def tree_bytes(root):
    return file_bytes(p for p in root.rglob("*") if "logs" not in p.relative_to(root).parts)


def package_bytes(packages):
    paths = subprocess.check_output(["dpkg-query", "-L", *packages], text=True).splitlines()
    return file_bytes(Path(p) for p in paths if p.startswith("/"))


def memory_sample():
    time.sleep(0.8)
    values = []
    for _ in range(5):
        fields = dict(line.split(":", 1) for line in Path("/proc/meminfo").read_text().splitlines())
        values.append({k: int(fields[k].split()[0]) for k in ("MemAvailable", "SUnreclaim", "VmallocUsed")})
        time.sleep(0.1)
    return {k: statistics.median(v[k] for v in values) for k in values[0]}


def nft_cycles(rulefile):
    repeats = []
    for i in range(6):
        before = memory_sample()
        subprocess.run(["nft", "-f", rulefile], check=True)
        loaded = memory_sample()
        subprocess.run(["nft", "delete", "table", "ip", "bench"], check=True)
        after = memory_sample()
        if i:  # The first transaction warms the tool, file and kernel caches.
            repeats.append(dict(before_kib=before, loaded_kib=loaded, after_kib=after))
    return repeats


if __name__ == "__main__":
    if os.readlink("/proc/self/ns/net") == os.readlink("/proc/1/ns/net"):
        sys.exit("Use a fresh network namespace")
    source, output = map(Path, sys.argv[1:])
    repo = Path(__file__).resolve().parents[1]
    resty, api = Path("/usr/local/openresty"), Path("/usr/local/apisix")
    ipset_pkgs = ["iptables", "ipset", "libipset13", "libxtables12", "libmnl0", "libnftnl11"]
    nft_pkgs = ["nftables", "libnftables1", "libxtables12", "libnftnl11", "libmnl0"]
    plugins = api / "apisix/plugins"
    bodies = {
        "module": (repo / "lua/ip_blacklist.lua").stat().st_size,
        "geo": 0,
        "matcher": (api / "deps/share/lua/5.1/resty/ipmatcher.lua").stat().st_size,
        "apisix": file_bytes([plugins / "ip-restriction.lua", *(plugins / "ip-restriction").glob("*.lua")]),
        "ipset": package_bytes(ipset_pkgs),
        "nft": package_bytes(nft_pkgs),
    }
    result = dict(openresty_bytes=tree_bytes(resty), apisix_bytes=tree_bytes(api),
                  component_bytes=bodies, ipset_packages=ipset_pkgs, nft_packages=nft_pkgs, sizes={})
    with tempfile.TemporaryDirectory() as tmp:
        rulefile = Path(tmp) / "rules.nft"
        for size in (150000, 400000, 600000):
            ips_file = source / str(size) / "ips"
            if not ips_file.exists():
                ips_file = source / f"sample-{size}.txt"
            ips = ips_file.read_text().splitlines()
            assert len(ips) == size
            route = {"id": "bench", "uri": "/", "upstream": {"type": "roundrobin", "nodes": {"127.0.0.1:18081": 1}},
                     "plugins": {"ip-restriction": {"blacklist": ips}}}
            nft = ("add table ip bench\nadd set ip bench blocked { type ipv4_addr; }\n" +
                   "".join("add element ip bench blocked { " + ", ".join(ips[i:i+512]) + " }\n"
                           for i in range(0, size, 512)) +
                   "add chain ip bench input { type filter hook input priority 0; policy accept; }\n" +
                   "add rule ip bench input tcp dport 18080 ip saddr @blocked counter drop\n")
            rulefile.write_bytes(nft.encode())
            rules = {
                "module": 64 + size * 4,
                "geo": sum(len(ip) + 4 for ip in ips),
                "matcher": sum(len(ip) + 1 for ip in ips),
                "apisix": len(json.dumps({"routes": [route]}) + "\n#END\n"),
                "ipset": (len("create blocked hash:ip maxelem 600000\n") + sum(len("add blocked ") + len(ip) + 1 for ip in ips)
                          + len("iptables -A INPUT -p tcp --dport 18080 -m set --match-set blocked src -j DROP\n")),
                "nft": rulefile.stat().st_size,
            }
            result["sizes"][size] = dict(rule_bytes=rules, nft_cycles=nft_cycles(rulefile))
            print("Measured", size, flush=True)
    output.write_text(json.dumps(result, indent=2) + "\n")
