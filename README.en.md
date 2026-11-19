# NGX IPBlock FastLite

An exact-IPv4 blacklist module for Nginx/OpenResty with `ngx_http_lua_module`.
A Go tool builds the list offline; Nginx loads it once before workers start and
performs no file I/O on the request path. By default, Nginx closes the connection
with **444** on a blacklist hit.

[简体中文](README.md) · [Format](FORMAT.md) · [Operations](docs/OPERATIONS.md) · [Testing](docs/TESTING.md) · [Linux benchmark](docs/BENCHMARK.md)

## Performance comparison

**Test machine:** Intel Core Ultra 5 225; WSL 2 / Debian 12 (kernel 6.18.33.2), 10 logical CPUs, approximately 15.32 GiB total RAM visible to WSL. OpenResty 1.31.8.1 used one worker on CPU 0 and an HTTP 204 upstream on CPU 2. A Go client pinned to CPUs 3 and 4 used 32 concurrent slots and a fresh loopback TCP connection per attempt.

Workload: **90% unlisted + 10% listed sources**. Each cell shows **mixed attempts per second · total memory · program + IP list size**. Disk size excludes the OpenResty or APISIX host; 0 B means no blocking component was installed.

**IP list source:** [borestad/blocklist-abuseipdb](https://github.com/borestad/blocklist-abuseipdb). The 150k, 400k and 600k sets were sampled from the 30-, 180- and 365-day IPv4 lists at [one pinned revision](https://github.com/borestad/blocklist-abuseipdb/tree/eb4a4a4ec7b2453b176cec0e7da0e0fb3033121c). Every engine used the same addresses at each size.

| Engine | 150,000 IPs<br>Mixed attempts/s · Total memory · Program + IP list size | 400,000 IPs<br>Mixed attempts/s · Total memory · Program + IP list size | 600,000 IPs<br>Mixed attempts/s · Total memory · Program + IP list size |
|---|---|---|---|
| Stock OpenResty (no IP blocking) | 70,180 attempts/s · 8.84 MiB · 0 B | 73,724 attempts/s · 8.94 MiB · 0 B | 72,939 attempts/s · 9.00 MiB · 0 B |
| ngx-ipblock-fastlite | 72,170 attempts/s · 11.51 MiB · 0.59 MiB | 66,972 attempts/s · 12.72 MiB · 1.54 MiB | 72,600 attempts/s · 13.54 MiB · 2.31 MiB |
| NGINX geo | 74,016 attempts/s · 86.96 MiB · 2.46 MiB | 72,917 attempts/s · 211.69 MiB · 6.56 MiB | 71,640 attempts/s · 295.57 MiB · 9.88 MiB |
| lua-resty-ipmatcher | 64,460 attempts/s · 30.95 MiB · 2.04 MiB | 67,297 attempts/s · 54.63 MiB · 5.43 MiB | 65,113 attempts/s · 86.82 MiB · 8.17 MiB |
| iptables + ipset | 1,572 attempts/s · 12.76 MiB · 6.62 MiB | 1,575 attempts/s · 19.14 MiB · 12.86 MiB | 1,574 attempts/s · 22.95 MiB · 17.89 MiB |
| APISIX ip-restriction | 45,624 attempts/s · 132.98 MiB · 2.47 MiB | 47,321 attempts/s · 247.61 MiB · 6.57 MiB | 47,648 attempts/s · 385.24 MiB · 9.88 MiB |
| APISIX (IP restriction off) | 48,181 attempts/s · 44.93 MiB · 0 B | 51,294 attempts/s · 44.04 MiB · 0 B | 47,706 attempts/s · 43.19 MiB · 0 B |

Rates include verified allows and blocks. Controls return 204 for both groups. Blocking cases must allow exactly 90%, block exactly 10% and have zero unexpected outcomes. The project closes listed connections (444); the other HTTP engines return 403. ipset DROP sends no reply: **32 concurrent slots and a 200 ms connect timeout cap this client at about 1,600 attempts/s, not the kernel’s drop capacity.** Memory is warm front-end PSS plus the kernel set. [Method and per-round results](docs/WSL-OPTIMIZED.md).

## Features

- Each exact IPv4 entry takes four bytes on disk. In memory, the high 16 bits select a bucket; only the low 16 bits are stored (two bytes per address), plus a prefix index of about 256 KiB. Binary search visits only the selected bucket.
- Optional IPv4 CIDR blocking merges adjacent or overlapping ranges at startup. Without CIDRs only the exact list is searched; with CIDRs the range search runs only after an exact miss.
- Loading from `init_by_lua*`, allowing pre-fork workers to share read-only data
  through copy-on-write.
- Initialization fails if the file format, length, address order, or payload SHA-256 fails validation.
- Global or per-host enablement, exact-IP allowlists, optional hit logging, and
  optional CF-header consistency checks.

## Supported scope

- Requires `ngx_http_lua_module`, LuaJIT/FFI, and `resty.sha256`; OpenResty is recommended.
- Tested on Windows x64 OpenResty 1.25.3.2 and Debian 12 x86_64 OpenResty 1.31.1.1.
- Go 1.22+ is needed only to build lists; Nginx does not need Go at runtime.
- The exact-address source takes one IPv4 per line, with a builder and loader default limit of **2 million unique addresses**. Configure IPv4 networks separately with `cidrs`. IPv6 and arbitrary start/end ranges remain unsupported.
- HTTP only; TCP/UDP traffic inside `stream {}` is not inspected.

## Install and use

### 1. Build a list

Use one IPv4 address per line. Blank lines and comments are allowed. Empty input
or an invalid address fails the build.

```sh
go build -o ipblacklist-builder ./cmd/ipblacklist-builder
./ipblacklist-builder build --input ips.txt --output candidate.bin
./ipblacklist-builder verify --input candidate.bin --json
```

### 2. Deploy the runtime files

| Repository file | Example server path |
|---|---|
| `lua/ip_blacklist.lua` | `/opt/ngx-ipblock-fastlite/lua/ip_blacklist.lua` |
| Generated `candidate.bin` | `/opt/ngx-ipblock-fastlite/current.bin` |

```sh
sudo install -d -m 0755 /opt/ngx-ipblock-fastlite/lua
sudo install -m 0644 lua/ip_blacklist.lua /opt/ngx-ipblock-fastlite/lua/ip_blacklist.lua
sudo install -m 0644 candidate.bin /opt/ngx-ipblock-fastlite/current.bin
```

`/opt/ngx-ipblock-fastlite` is a recommended example and may be replaced with
any Nginx-readable directory. Neither `ipblacklist-builder` nor
`conf/nginx-integration.conf.example` is required at runtime.

### 3. Integrate with Nginx

Place both parts inside the existing `http {}`. `init()` loads the list;
`access()` performs request-time checks. Both are required.

```nginx
lua_package_path "/opt/ngx-ipblock-fastlite/lua/?.lua;;";

init_by_lua_block {
    require("ip_blacklist").init({
        path = "/opt/ngx-ipblock-fastlite/current.bin",
        -- Optional IPv4 networks shared by all enabled sites.
        -- cidrs = { "192.0.2.0/24", "198.51.100.128/25" },
        default_site = {
            enabled = true,
            require_cf_header = false,
        },

        -- Optional independent per-host configuration. A site entry does not
        -- inherit default_site, so keep enabled = true explicitly.
        -- sites = {
        --     ["www.example.com"] = {
        --         enabled = true,
        --         require_cf_header = false,
        --     },
        -- },
    })
}

access_by_lua_block {
    require("ip_blacklist").access()
}
```

Set `cidrs` at the top level of `init()`. Use the network address and a prefix
length from /0 to /32. For example, `192.0.2.0/24` covers `192.0.2.0` through
`192.0.2.255`. Host bits in `192.0.2.1/24` cause initialization to fail.
A /0 covers all IPv4 addresses. Site allowlists take precedence. Reload to apply
rule changes using the existing exact-address binary. An omitted or empty
`cidrs` array keeps the original lookup function.

The `?` is a Lua module-name placeholder: `require("ip_blacklist")` resolves
`ip_blacklist.lua`; it is not an unrestricted wildcard. The trailing `;;`
retains Lua's default search path.

`init_by_lua*` denotes this directive family (normally the `_block` or `_file`
form), not a configuration filename. Run `init()` from `init_by_lua*`, never
`init_worker_by_lua*`.

Do not add a second Lua directive when the same context already has WAF or Lua hooks:

- Append this module path to the existing `lua_package_path` value.
- Add `require(...).init(...)` to an existing `init_by_lua_file`.
- Add `.access()` to an existing `access_by_lua_file` or `access_by_lua_block`.
- A `server` or `location` with its own `access_by_lua*` overrides the inherited
  `http` hook; merge `.access()` there as well or those requests will not be checked.

See [the merge example](conf/nginx-integration.conf.example). On panel-managed
systems, inspect the effective configuration and Lua files with `nginx -T`
instead of inferring them from directory names.

## Cloudflare (optional)

Without Cloudflare, no CF configuration is needed; keep
`require_cf_header = false`.

With Cloudflare, first restore `$remote_addr` through Nginx Real IP settings and
trusted `set_real_ip_from` ranges. Do not include another range file when those
settings already exist. [cloudflare-realip.conf](conf/cloudflare-realip.conf) is
optional reference material, not a required runtime file.

`require_cf_header = true` only checks that `CF-Connecting-IP` exists and matches
`$remote_addr`; a failure returns 403 by default. It does not prove that the
connection came from Cloudflare and does not replace trusted proxy ranges or
origin access control.

## Hit logging (optional)

Configure these fields in `default_site` or the relevant `sites` entry:

| Setting | Default | Meaning |
|---|---:|---|
| `log_hits` | `false` | Master hit-log switch |
| `log_sample_rate` | `0` | `0` disables hit logging, `0.01` samples about 1%, and `1` makes every hit eligible subject to the per-second limit |
| `log_rate_per_second` | `10` | Per-worker entries per second; `0` disables logging |

Logging occurs only when `log_hits` is true, sampling is above zero, and the rate
limit is above zero. Messages use `ngx.WARN` and go to the active Nginx
`error_log`, whose level must be `warn` or more verbose. These settings affect
logging only, not 444 blocking. Use sampling and rate limiting in production.

## Verify and update

```sh
nginx -t
nginx -s reload
```

After reload, verify that a normal IP is allowed, a listed IP is closed before
the application, and existing WAF/auth checks still run. Nginx 444 sends no HTTP
status line, so clients normally report an empty response or closed connection.

See [Operations](docs/OPERATIONS.md) for atomic updates and rollback, and
[Testing](docs/TESTING.md) for automated checks.

## 151,253-address example

The repository includes a [151,253-address validation snapshot](examples/abuseipdb-30d/README.md)
from the [borestad/blocklist-abuseipdb 30-day feed](https://github.com/borestad/blocklist-abuseipdb/blob/main/abuseipdb-s100-30d.ipv4); its binary is 605,076 bytes. This third-party list is not covered by the
project's MIT license; verify upstream permission before use or redistribution.

See [historical Windows results](docs/PERFORMANCE.md) and the current
[Linux benchmark](docs/BENCHMARK.md). These results support reproducibility and
version comparisons; they are not OpenResty certification and cannot predict a
production slowdown percentage directly.

## License

Code and documentation are available under the [MIT License](LICENSE).
