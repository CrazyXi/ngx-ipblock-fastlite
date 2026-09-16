# Testing · [简体中文](TESTING.zh-CN.md)

Run from the repository root:

```sh
go test ./...
go vet ./...
go build -o ipblacklist-builder ./cmd/ipblacklist-builder
./ipblacklist-builder build --input testdata/source-valid.txt --output fixture.bin
./ipblacklist-builder verify --input fixture.bin --json
```

Go tests cover TXT comments/BOM/CRLF, strict address parsing, duplicates,
unsigned IPv4 boundaries, binary format/digest/order corruption, empty-list
rejection, and the CLI. A candidate output is not published when a source is
empty or malformed. Test files are generated in temporary directories.

For a local ngx_lua integration, use a separate listener and verify:

- A normal IPv4 continues to the backend; a listed IPv4 closes with Nginx 444
  before the backend; a whitelisted listed IPv4 continues.
- A valid IPv6 skips the IPv4 list. Direct sites work without any CF header.
  For CF sites with `require_cf_header = true`, missing or mismatched
  `CF-Connecting-IP` fails closed rather than selecting a fake visitor.
- The module loads in `init_by_lua*`; a worker-phase initialization is refused.
  Confirm count and data SHA-256 through a local-only status endpoint.
- Exercise every intended business location and existing WAF/auth hook,
  including internal redirects and WebSocket handshakes.
- Verify a new list adds and removes IPs; test update failure and rollback
  while the last good list stays active.

The 151,253-IP Windows case was tested with OpenResty 1.25.3.2, including all
listed members, boundary/random lookups, HTTP 444, pressure, and memory
sampling. [Evidence](PERFORMANCE.md) is local-only; Linux worker sharing,
multi-worker reload and production proxy chains need validation on their own
host before any production performance claim.
