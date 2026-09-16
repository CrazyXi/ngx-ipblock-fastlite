# Local performance · [简体中文](PERFORMANCE.zh-CN.md)

These are **Windows localhost** observations on OpenResty 1.25.3.2, one worker,
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

The source retains three-way binary search and a bounded-distance midpoint
shift. A tested `lower_bound` variant did not consistently beat it. Raw local
experiment reports remain in Git-ignored `localtest/` on the development host.
