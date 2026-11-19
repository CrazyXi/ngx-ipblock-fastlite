# Validation record · [简体中文](VALIDATION.zh-CN.md)

## Current automated validation

- `go test ./...`, `go vet ./...`, and builder compilation pass.
- Unit tests cover strict TXT parsing, deduplication, empty-list rejection, and
  `IPBLV001` format/digest/order validation.
- Debian 12 x86_64, OpenResty 1.31.1.1, Test::Nginx 0.32: 11/11 integration
  assertions pass.
- Strict Valgrind also passes 11/11 in that Linux environment. Throughput and
  PSS observations are in the [Linux benchmark](BENCHMARK.md).

## 151,253-address case

The [validation snapshot](../examples/abuseipdb-30d/README.md) contains 151,253
unique IPv4 addresses. `snapshot.bin` is 605,076 bytes and `snapshot.json`
records its data digest.

The same snapshot was previously tested on Windows x64 OpenResty 1.25.3.2 for
all-member, boundary, adjacent, random, HTTP 444, load, and memory behavior;
historical numbers are in [Windows performance](PERFORMANCE.md). The current
Linux tests use this snapshot as well.

## Not a production acceptance test

Public results do not replace validation on the deployment host. Before release,
verify access-hook coverage, WAF/auth ordering, the real proxy chain,
reload/rollback, application latency, and false-positive handling. Cloudflare
deployments must also validate trusted proxy ranges, origin access control, and
Pseudo IPv4 settings.
