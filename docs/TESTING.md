# Testing · [简体中文](TESTING.zh-CN.md)

## Go build and unit tests

Run from the repository root:

```sh
go test ./...
go vet ./...
go build -o ipblacklist-builder ./cmd/ipblacklist-builder
./ipblacklist-builder build --input testdata/source-valid.txt --output fixture.bin
./ipblacklist-builder verify --input fixture.bin --json
```

Coverage includes strict IPv4 parsing, comments/BOM/CRLF, deduplication,
unsigned boundaries, empty-list rejection, binary format/length/digest/order
corruption, and CLI behavior.

## OpenResty integration

Test::Nginx, OpenResty, and the repository's 151,253-address fixture are required:

```sh
export TEST_NGINX_BINARY=/usr/local/openresty/nginx/sbin/nginx
export TEST_NGINX_IPBLOCK_LUA="$PWD/lua"
export TEST_NGINX_IPBLOCK_BIN="$PWD/examples/abuseipdb-30d/snapshot.bin"

prove -v t/001-integration.t t/002-cidr.t
```

The suite checks an exact hit, a miss, access-phase allow, and rejection before
content. Automated rejection uses 403 to obtain an assertable HTTP response;
default 444 closes the connection and should be checked end to end separately.

On WSL, `sudo unshare -n sh t/wsl-smoke.sh` checks suffix boundaries, CIDR merging, allowlist precedence and real 444 closes directly in OpenResty without the Perl test framework.

Strict memory check:

```sh
TEST_NGINX_USE_VALGRIND=1 \
TEST_NGINX_VALGRIND_EXIT_ON_FIRST_ERR=1 \
prove -v t/001-integration.t
```

`valgrind.suppress` only suppresses the known Nginx
`ngx_set_environment` process-exit allocation, not project call stacks.

See the [Linux benchmark](BENCHMARK.md) for Benchmark commands, raw logs, and limitations.

## Pre-deployment acceptance

- A normal IPv4 reaches the application; a listed IPv4 closes with 444 before
  content; a listed but allowlisted IPv4 is allowed.
- IPv6 does not enter this project's IPv4 lookup path.
- Every protected `server`/`location` actually runs `.access()`, including
  existing WAF/auth, internal redirect, and WebSocket paths.
- If `require_cf_header=true` is enabled, verify that a missing or mismatched
  header returns 403. Also validate trusted proxy ranges and origin access
  control; the consistency check is not source authentication.
- A new list adds and removes addresses as expected; failed builds, failed
  reloads, and rollback leave the old workers usable.
- Restrict the status endpoint to localhost/admin access and verify its count
  and data SHA-256.

Historical Windows OpenResty 1.25.3.2 results are in
[Local performance](PERFORMANCE.md). Debian 12/OpenResty 1.31.1.1
Test::Nginx, Valgrind, and PSS results are in the [Linux benchmark](BENCHMARK.md).
Neither is a production capacity guarantee.
