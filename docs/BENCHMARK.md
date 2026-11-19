# Linux benchmark · [简体中文](BENCHMARK.zh-CN.md)

See the [README comparison with 10% listed sources](../README.en.md#performance-comparison) for the three list sizes. This page preserves Linux results for an earlier build; these are not fresh measurements of the current version.

Test date: 2026-09-22. The tests use the OpenResty-maintained Test::Nginx
framework, but the scenarios, parameters, execution, and results are supplied
by this project. They are not an official OpenResty score, certification, or endorsement.

## Environment

- Debian 12 x86_64 on Alibaba Cloud KVM
- 2 vCPUs (one core, two threads), 701 MiB RAM, no swap
- Official OpenResty Debian package 1.31.1.1
- Test::Nginx 0.32, weighttp 0.5, and Valgrind
- 151,253 entries; 605,076-byte binary file
- 867,160 bytes for the address array and 16-bit prefix index
- One Nginx worker for throughput; two workers for the supplemental PSS test
- Load generator and Nginx on the same host

## Test::Nginx Benchmark

Test::Nginx Benchmark mode ran an A/B test with randomized block order:

- Baseline loaded the same module and list but did not call `access()`.
- Protected ran one exact IPv4 miss.
- Each group used 20,000 warm-up requests followed by 1,000,000 requests at
  concurrency 50.
- Eight rounds were run in randomized block order with zero response errors.

| Round | Baseline req/s | Protected req/s | Throughput delta |
|---:|---:|---:|---:|
| 1 | 160,948 | 150,057 | -6.77% |
| 2 | 162,091 | 150,873 | -6.92% |
| 3 | 169,406 | 151,047 | -10.84% |
| 4 | 164,834 | 150,784 | -8.52% |
| 5 | 162,506 | 151,517 | -6.76% |
| 6 | 161,175 | 151,419 | -6.05% |
| 7 | 164,085 | 149,993 | -8.59% |
| 8 | 162,773 | 150,971 | -7.25% |
| Median | **162,640** | **150,922** | **-7.09% paired median** |

This ratio comes from an intentionally tiny `204` response path, which magnifies
fixed Lua/access overhead. The client and server also contend for one physical
core. It is useful for this project's A/B comparison, not as a direct estimate of
production website slowdown.

## Functional and Valgrind results

- Test::Nginx integration suite: 11/11 assertions passed, covering hit, miss,
  access allow, and rejection before content.
- Strict Valgrind Memcheck: 11/11 passed with `exit-on-first-error`; no project
  invalid memory access or unsuppressed leak was reported.
- `valgrind.suppress` only suppresses a fixed 48-byte process-exit allocation in
  Nginx `ngx_set_environment`; its stack does not pass through project code.

## Actual memory (PSS)

This is a supplemental Linux process-memory observation, not an official
Test::Nginx benchmark item. With two workers, `/proc/<pid>/smaps_rollup` PSS was
summed across the master and both workers:

- Cold baseline: 6,001 KiB; module and list loaded: 7,340 KiB; **+1,339 KiB**.
- Protected after 200,000 requests: 12,356 KiB.
- Protected after 2,200,000 total requests: 12,374 KiB, **+18 KiB** versus the
  200,000-request sample; no material continuing growth was observed in this run.

The cold increment is counted once across shared pages, consistent with the
design in which `init_by_lua*` loads the list before workers share it through
pre-fork COW. Summed RSS would double-count these shared pages.

## Reproduce

```sh
export TEST_NGINX_BINARY=/usr/local/openresty/nginx/sbin/nginx
export TEST_NGINX_IPBLOCK_LUA="$PWD/lua"
export TEST_NGINX_IPBLOCK_BIN="$PWD/examples/abuseipdb-30d/snapshot.bin"

prove -v t/001-integration.t

for round in $(seq 1 8); do
  TEST_NGINX_BENCHMARK='1000000 50' \
  TEST_NGINX_BENCHMARK_WARMUP=20000 \
  prove -v t/100-benchmark.t
done

TEST_NGINX_USE_VALGRIND=1 \
TEST_NGINX_VALGRIND_EXIT_ON_FIRST_ERR=1 \
prove -v t/001-integration.t
```

Raw logs:

- [Eight-round Benchmark](../benchmark/results/test-nginx-official-8rounds.log)
- [Strict Valgrind](../benchmark/results/test-nginx-valgrind-strict.log)
