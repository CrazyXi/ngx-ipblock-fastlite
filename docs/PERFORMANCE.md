# Historical Windows performance · [简体中文](PERFORMANCE.zh-CN.md)

This page preserves historical **Windows localhost** results from v1.3.3.
See the [Linux benchmark](BENCHMARK.md) for current Test::Nginx, Valgrind, and PSS results.

The test used OpenResty 1.25.3.2, one worker,
32 Go clients, and the [151,253-IP snapshot](../examples/abuseipdb-30d/README.md).
They are not results for Linux, another library, or a production website.

| v1.3.3 test | Observation |
|---|---:|
| 20 s, one allowed IP | 52,324 req/s; 0 errors |
| 20 s, 24 rotating allowed IPs | 51,346 req/s; 0 errors |
| 10 s, listed IP | 4,881 expected Nginx 444 closes/s; 0 errors |
| Same-instance no-hook vs full-hook 204 test, six alternating 5 s pairs | 2.23%–3.67% lower throughput with the hook; paired median 2.90% |

The no-hook route and protected route differ slightly in URI/response header;
the percentage is the **whole local test-path difference**, not just binary
search. A thin 204 endpoint magnifies fixed per-request cost. Real upstream
work, CPU headroom, TLS/WAF, and caching will change the observed effect. An
online slowdown percentage cannot be calculated from this test.

For exact structures, the 151,253-address FFI array is 605,012 bytes; the /16
offset index is 262,148 bytes. This **867,160-byte** net array size excludes
Lua, allocator, worker and OS overhead. A local master+worker process sample
after pressure was about 27.8 MiB summed Working Set and 9.8–10.0 MiB summed
Private Bytes; Windows sums are not Linux shared-memory/PSS measurements.

That version retained three-way binary search and a bounded-distance midpoint
shift. A tested `lower_bound` variant did not consistently beat it. Raw local
experiment reports remain in Git-ignored `localtest/` on the development host.

## IPv4 CIDR regression test (2026-11-19)

Windows OpenResty 1.25.3.2, one worker, 32 concurrent clients, 151,253 exact
addresses. Four rotating misses reached equivalent 204 endpoints. After warm-up,
six four-second samples per variant ran in balanced order (ABC, BCA, CAB, CBA,
BAC, ACB). C included 256 disjoint /25 networks.

| Round | A previous req/s | B CIDR off | C CIDR on |
|---:|---:|---:|---:|
| 1 | 59,679 | 59,228 | 58,185 |
| 2 | 58,616 | 59,435 | 58,498 |
| 3 | 58,991 | 58,170 | 58,460 |
| 4 | 59,057 | 58,939 | 58,374 |
| 5 | 58,798 | 58,996 | 58,052 |
| 6 | 42,168 | 41,224 | 41,156 |

Median paired throughput changes were **−0.48%** for B versus A and **−1.27%**
for C versus B. All responses were as expected. All three groups slowed down in
round six, which is included. These measurements cover allowed requests.

In a separate initialization test, three loads per variant in one VM gave median
post-GC worker LuaJIT heap increments of **861.7 KiB** (A), **862.0 KiB** (B),
and **877.5 KiB** (C), with the configuration input already allocated. The CIDR
index payload uses eight bytes per merged interval, or 2 KiB for this sample.
