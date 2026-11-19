# NGX IPBlock FastLite

[简体中文](README.md) · [Format](FORMAT.md) · [Operations](docs/OPERATIONS.md) · [Tests](docs/TESTING.md)

## Features

- Exact IPv4 matching; a listed address is closed by Nginx with **444**.
- The Go builder converts text input into a sorted binary file; loading verifies
  file length, ordering, and SHA-256.
- Addresses use a compact FFI `uint32_t` array. The top 16 IPv4 bits select a
  bucket, followed by binary search inside that bucket instead of a full scan.
- `init_by_lua*` loads the list once. Workers share read-only memory on
  pre-fork/COW platforms; requests perform no file, database, Redis, or remote lookup.
- Global and per-host controls, exact-IP allowlists, optional CF-header
  validation, and sampled rate-limited hit logging.

## Requirements

- Nginx with `ngx_http_lua_module`, LuaJIT/FFI, and `resty.sha256`.
- Go 1.22+ to build a list; Go is not required by Nginx at runtime.
- Tested locally on Windows x64 OpenResty 1.25.3.2; other versions are pending.
- Individual IPv4 addresses only; CIDR and IPv6 are not supported.

## Performance comparison

**Test machine:** Intel Core Ultra 5 225; WSL 2 / Debian 12 (kernel 6.18.33.2), 10 logical CPUs, approximately 15.32 GiB total RAM visible to WSL. OpenResty 1.31.8.1 used one worker on CPU 0 and an HTTP 204 upstream on CPU 2. Two wrk 4.1.0 threads on CPUs 3 and 4 used 32 persistent connections over loopback.

Each cell shows **allowed throughput · total memory · program + IP list size**. Disk size excludes the OpenResty or APISIX host; 0 B means no blocking component was installed.

| Engine | 150,000 IPs<br>Allowed throughput · Total memory · Program + IP list size | 400,000 IPs<br>Allowed throughput · Total memory · Program + IP list size | 600,000 IPs<br>Allowed throughput · Total memory · Program + IP list size |
|---|---|---|---|
| Stock OpenResty (no IP blocking) | 156,829 req/s · 8.89 MiB · 0 B | 154,831 req/s · 8.75 MiB · 0 B | 155,865 req/s · 8.90 MiB · 0 B |
| ngx-ipblock-fastlite | 143,473 req/s · 11.38 MiB · 0.59 MiB | 147,814 req/s ¹ · 12.61 MiB ¹ · 1.54 MiB | 145,726 req/s · 13.50 MiB · 2.31 MiB |
| NGINX geo | 139,767 req/s · 125.21 MiB · 2.46 MiB | 138,480 req/s ¹ · 265.22 MiB ¹ · 6.56 MiB | 136,059 req/s · 296.40 MiB · 9.88 MiB |
| lua-resty-ipmatcher | 138,616 req/s · 31.07 MiB · 2.04 MiB | 136,827 req/s · 56.60 MiB · 5.43 MiB | 137,113 req/s · 80.70 MiB · 8.17 MiB |
| iptables + ipset | 156,281 req/s ² · 12.30 MiB · 6.62 MiB | 157,846 req/s ² · 18.81 MiB · 12.86 MiB | 157,931 req/s ² · 22.82 MiB · 17.89 MiB |
| APISIX ip-restriction | 65,271 req/s · 132.92 MiB · 2.47 MiB | 63,415 req/s · 245.73 MiB · 6.57 MiB | 61,564 req/s · 384.49 MiB · 9.88 MiB |
| APISIX (IP restriction off) | 68,512 req/s ¹ · 43.86 MiB ¹ · 0 B | 69,649 req/s · 44.04 MiB · 0 B | 68,480 req/s · 43.24 MiB · 0 B |

¹ Repeat after unstable samples. ² Separate paired ipset retest; no consistent change in allowed throughput at any size. Memory includes the warm front end and any separate kernel set. The project row measures the optimized 16-bit suffix build (Lua file SHA-256 prefix `617e818eb705`), not other versions or production traffic. [Method and raw results](docs/WSL-OPTIMIZED.md).

## Install

### 1. Build the list

Use a non-empty TXT file with one IPv4 address per line. Blank lines and
comments are allowed; an invalid address stops the build.

```sh
go build -o ipblacklist-builder ./cmd/ipblacklist-builder
./ipblacklist-builder build --input ips.txt --output candidate.bin
./ipblacklist-builder verify --input candidate.bin --json
```

### 2. Deploy the files

| Required file | Server path |
|---|---|
| `lua/ip_blacklist.lua` | `/opt/ngx-ipblock-fastlite/lua/ip_blacklist.lua` |
| Generated `candidate.bin` | `/opt/ngx-ipblock-fastlite/current.bin` |

`/opt/ngx-ipblock-fastlite` is an example path. Any Nginx-readable directory
may be used.

```sh
sudo install -d -m 0755 /opt/ngx-ipblock-fastlite/lua
sudo install -m 0644 candidate.bin /opt/ngx-ipblock-fastlite/current.bin
sudo install -m 0644 lua/ip_blacklist.lua /opt/ngx-ipblock-fastlite/lua/ip_blacklist.lua
```

Nginx needs only those two files at runtime:

- `ipblacklist-builder` only creates or updates `current.bin`; do not upload it
  unless the server will perform automatic updates, in which case use a Linux build.
- `conf/nginx-integration.conf.example` is reference material, not a deployed file;
  merge the needed directives into the existing `nginx.conf`.
- A Windows builder is named `ipblacklist-builder.exe` and cannot run on Linux.

### 3. Configure Nginx

Add both parts below to the existing `http {}`. `init()` loads the list and
`access()` checks requests; both are required. A BT/aaPanel-style installation
usually uses `/www/server/nginx/conf/nginx.conf`; confirm the active files with
`nginx -T`.

```nginx
# Inside http {} (merge with any existing init_by_lua*):
lua_package_path "/opt/ngx-ipblock-fastlite/lua/?.lua;;";
init_by_lua_block {
    require("ip_blacklist").init({
        path = "/opt/ngx-ipblock-fastlite/current.bin",
        default_site = {
            enabled = true,            -- Enables blocking; unrelated to Cloudflare.
            require_cf_header = false, -- Default: use Nginx's $remote_addr.
        },

        -- Optional: enable strict CF-header validation only for selected hosts.
        -- sites = {
        --     ["www.example.com"] = {
        --         enabled = true,
        --         require_cf_header = true,
        --     },
        -- },
    })
}

# Required: run the lookup during each request's access phase.
access_by_lua_block {
    require("ip_blacklist").access()
}
```

If a `server` or `location` already has `access_by_lua*` for a WAF or
authentication, add `require("ip_blacklist").access()` to that existing code.
Do not define a second hook at the same level.

`enabled = true` enables blocking; it is not a CF switch. `require_cf_header`
defaults to `false`, so the module checks Nginx's `$remote_addr`. Direct sites
need no CF configuration, and CF sites with correctly restored Real IPs do not
need to set it to `true` either.

Setting `require_cf_header = true` is an optional strict check: the
`CF-Connecting-IP` header must exist and match the Real-IP-restored
`$remote_addr`, otherwise the module returns 403. Enable it only for selected
CF hosts. If Nginx already has Real IP settings, do not include the
[optional range file](conf/cloudflare-realip.conf) again. See the
[merge example](conf/nginx-integration.conf.example).

Run `nginx -t` before reload, then test a normal IP and a listed IP (444).
See [Operations](docs/OPERATIONS.md) for updates and rollback.

## Validation case

The [151,253-IPv4 validation case](examples/abuseipdb-30d/README.md) documents a
2026-09-15 local snapshot of **151,253 unique IPv4s**. The repository includes
its `snapshot.bin` and checksums; the case also shows how to fetch the current
feed and build a new copy.

In the local test, the address array and prefix index used about **847 KiB**;
the median throughput difference between the complete protected path and the
no-hook path was **2.90%**. These Windows-local numbers are not a production
estimate. See [Performance](docs/PERFORMANCE.md).

## Notes

- Load only in `init_by_lua*`, never `init_worker_by_lua*`; the module rejects
  worker-phase initialization. Empty lists are rejected; failed updates keep
  the last good list. Disable a site rather than publishing an empty list.
- IPv6 bypasses this IPv4 list. Cloudflare cache hits do not reach the origin.
- Code and docs are [MIT licensed](LICENSE). The license does not cover the
  third-party list; verify upstream rights before use or redistribution.
