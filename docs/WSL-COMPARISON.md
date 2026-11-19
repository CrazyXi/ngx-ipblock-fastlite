# Historical WSL comparison (before optimization) · [简体中文](WSL-COMPARISON.zh-CN.md)

This page preserves the initial comparison and storage A/B. See the [optimized comparison](WSL-OPTIMIZED.md) for the latest memory, disk footprint and throughput measurements.

Project-run measurements on Debian 12 under WSL 2, 2026-11-19. The project’s IP blocklist module v1.4.0 and five alternatives processed the same exact addresses. HTTP policies ran on direct client IPs and the firewall used TCP source addresses. CIDR, CF header checks and access logging were disabled.

## Inputs

The pinned 30-, 180- and 365-day feeds contained 153,496, 436,778 and 754,110 unique IPv4 addresses. After verifying the Git blob IDs, rank the four-byte network-order addresses by SHA-256 and take 150,000, 400,000 and 600,000 respectively. Sort each subset numerically for all engines. Address distribution also varies across feed windows. [Upstream commit](https://github.com/borestad/blocklist-abuseipdb/tree/eb4a4a4ec7b2453b176cec0e7da0e0fb3033121c) · [Source and subset hashes](../benchmark/results/wsl-sources.jsonl)

## Measurements

These historical all-allowed ipset rates do not measure drop capacity. See the [paired all-allowed samples](../benchmark/results/wsl-ipset-paired.jsonl) and the [current 10%-listed comparison](../README.en.md#performance-comparison). Native nftables is omitted for lack of attributable total memory; raw samples are retained.

The stock OpenResty control forwards to the same upstream using OpenResty. Firewall runs add a kernel source-address check to this setup. The project module runs an HTTP access hook that selects the site policy, parses the client address and looks it up. Throughput covers the full request path.

Throughput and allowed-request P99 latency are medians of three 3-second samples. PSS is the warm front-end process total in MiB. Readiness is one elapsed sample from configuring or loading prepared rules through the first allowed 204, including startup. For each list size, the combined memory column sums front-end PSS and any separate kernel blocklist set using the unrounded measurements. An unmeasured set leaves the total unknown.

| Entries | Engine | Allowed req/s | P99 ms | Hit 403 req/s | Front-end PSS MiB | Kernel blocklist set MiB | Front end + set MiB | Ready s |
|---:|---|---:|---:|---:|---:| ---: | ---: |---:|
| 150,000 | Stock OpenResty (no IP blocking) | 162,693 | 0.478 | — | 9.0 | — | 8.95 | 0.029 |
| 150,000 | ngx-ipblock-fastlite | 148,143 | 0.506 | 288,781 | 12.2 | — | 12.23 | 0.030 |
| 150,000 | NGINX geo | 142,385 | 0.497 | 347,575 | 125.1 | — | 125.15 | 0.089 |
| 150,000 | lua-resty-ipmatcher | 140,334 | 0.567 | 284,526 | 31.0 | — | 31.04 | 0.078 |
| 150,000 | iptables + ipset | 160,193 | 0.478 | — | 8.9 | 3.48 | 12.40 | 0.310 |
| 150,000 | APISIX (IP restriction off) | 71,928 | 1.090 | — | 43.9 | — | 43.89 | 0.157 |
| 150,000 | APISIX ip-restriction | 71,110 | 1.160 | 121,284 | 133.3 | — | 133.31 | 0.925 |
| 400,000 | Stock OpenResty (no IP blocking) | 156,534 | 0.473 | — | 8.9 | — | 8.90 | 0.017 |
| 400,000 | ngx-ipblock-fastlite | 149,212 | 0.503 | 289,988 | 14.3 | — | 14.33 | 0.029 |
| 400,000 | NGINX geo | 142,454 | 0.523 | 348,497 | 312.0 | — | 312.05 | 0.171 |
| 400,000 | lua-resty-ipmatcher | 141,272 | 0.574 | 285,754 | 56.4 | — | 56.43 | 0.197 |
| 400,000 | iptables + ipset | 162,912 | 0.489 | — | 8.9 | 10.02 | 18.92 | 0.652 |
| 400,000 | APISIX (IP restriction off) | 78,833 | 0.950 | — | 43.7 | — | 43.66 | 0.147 |
| 400,000 | APISIX ip-restriction | 66,621 | 1.300 | 109,298 | 245.6 | — | 245.64 | 2.769 |
| 600,000 | Stock OpenResty (no IP blocking) | 158,297 | 0.483 | — | 8.9 | — | 8.89 | 0.029 |
| 600,000 | ngx-ipblock-fastlite | 148,896 | 0.503 | 281,617 | 17.0 | — | 17.01 | 0.029 |
| 600,000 | NGINX geo | 142,731 | 0.511 | 348,454 | 296.6 | — | 296.62 | 0.256 |
| 600,000 | lua-resty-ipmatcher | 142,788 | 0.586 | 283,867 | 88.0 | — | 88.02 | 0.379 |
| 600,000 | iptables + ipset | 160,060 | 0.493 | — | 8.8 | 13.92 | 22.73 | 0.931 |
| 600,000 | APISIX (IP restriction off) | 70,786 | 1.080 | — | 44.3 | — | 44.34 | 0.158 |
| 600,000 | APISIX ip-restriction | 63,671 | 1.620 | 110,594 | 385.2 | — | 385.20 | 4.415 |

HTTP engines returned 403 for listed clients to allow response accounting. The project’s IP blocklist module normally closes with 444, which changes connection reuse. For firewall hits we verified TCP timeouts and DROP counters. A dropped packet has no HTTP response rate.

At 600,000 entries the allowed client was always 127.0.0.1. Its /16 bucket was empty, so the exact lookup ended at the prefix index. Median front-end CPU time per request was 6.30 µs for direct proxying and 6.70 µs with the module. The roughly 0.40 µs difference covers the full path, including Lua access handling, host and client address reads, IPv4 parsing, the allowlist check and the prefix lookup. Individual steps were not timed. Firewall cases checked packet source addresses in the kernel before OpenResty forwarded allowed requests. Process CPU figures do not isolate kernel set lookup costs.

The project module’s front-end PSS includes its blocklist and index. Both firewalls store their sets in the Linux kernel. The kernel column uses `Size in memory` from `ipset list -terse`. The native nftables run did not record set memory. A dash in this column means the engine used no separate kernel blocklist set. At 600,000 entries the two measured components for iptables + ipset sum to about 22.73 MiB. Total memory for native nftables cannot be ranked from this run.

## Before and after the storage change

This run compares the original module with 16-bit suffix storage and incremental GC during loading. Each list size used four interleaved A/B pairs with a fresh single-worker front end per run. Each workload rotated through 2,048 IPv4 addresses via a Real IP header trusted only from the loopback client. Allowed addresses came from occupied buckets. wrk measured requests forwarded to the same 204 upstream. The project's load client counted connections closed without a response on default 444 hits. Each workload used 32 concurrent clients, a one-second warm-up and a three-second sample. Values are medians of four runs per version, shown as original → optimized.

| Entries | Warm front-end total PSS (MiB) | Allowed req/s | 444 closes/s |
|---:|---:|---:|---:|
| 150,000 | 13.19 → 12.12 | 136,222 → 137,172 | 54,504 → 54,586 |
| 400,000 | 15.19 → 13.41 | 137,900 → 138,924 | 54,742 → 54,208 |
| 600,000 | 17.39 → 14.19 | 134,743 → 136,187 | 54,829 → 54,834 |

Front-end PSS fell in all 12 pairs and includes the master, worker, list and index. Median allowed throughput improved by roughly 0.7% to 1.1%. At 400,000 entries the 444 close rate fell in all four pairs, with a median decline of about 1.0%. These comparisons use the paired runs with rotating addresses and default connection closure. [Per-run data](../benchmark/results/wsl-suffix-ab.jsonl)

## Reproduction and scope

WSL kernel 6.18.33.2, Intel Core Ultra 5 225, 10 logical CPUs. OpenResty 1.31.8.1 and lua-resty-ipmatcher 0.6.1 shipped with APISIX 3.20.0. iptables 1.8.9, ipset 7.17, nftables 1.0.6, wrk 4.1.0. A fresh network namespace contained one front-end worker on CPU 0, a common 204 backend on CPU 2 and two wrk threads on CPUs 3 and 4. Concurrency was 32 with keepalive and a 3-second warm-up per engine. APISIX ran standalone against the same upstream, with an additional plugin-off route baseline.

One fixed miss was used for allowed throughput, one fixed hit for HTTP rejection throughput and both end members for correctness probes. Results describe hot addresses. Engine startup order was fixed, so small differences should be read alongside sample variation. Front-end PSS covers the master and child processes. The load generator, common upstream and kernel sets are accounted separately or excluded. The raw data includes front-end CPU time. [Raw samples](../benchmark/results/wsl-samples.jsonl) · [Runner](../benchmark/wsl_compare.py)

```sh
rev=eb4a4a4ec7b2453b176cec0e7da0e0fb3033121c
src=$(mktemp -d)
for days in 30 180 365; do
  curl -fL --compressed "https://raw.githubusercontent.com/borestad/blocklist-abuseipdb/$rev/abuseipdb-s100-${days}d.ipv4" -o "$src/${days}d.txt" || exit 1
done
sudo unshare -nm --propagation private python3 benchmark/wsl_compare.py "$src/run" "$src"
```
