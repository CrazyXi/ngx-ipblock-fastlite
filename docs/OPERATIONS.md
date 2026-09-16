# Operations · [简体中文](OPERATIONS.zh-CN.md)

This page covers first deployment, update, and rollback. See the
[English README](../README.md) for requirements and the small setup example.
Paths in the example files are placeholders; adjust them to your host.

## First deployment

1. Build or download a non-empty TXT list. Run `ipblacklist-builder build`
   and `verify` before activation. The builder refuses malformed or empty input
   and does not overwrite an existing output file.
2. Install the verified `current.bin` and `lua/ip_blacklist.lua` in directories
   readable by Nginx. Complete both **required** stages from the
   [merge example](../conf/nginx-integration.conf.example): `.init()` in the
   existing `init_by_lua*` loads data, while `.access()` in `access_by_lua*`
   performs request checks. Configuring only `.init()` blocks nothing. Merge
   the call into any child WAF/auth access hook that overrides inheritance.
3. The example enables `default_site`, so every site that actually calls
   `.access()` is checked. Use optional `sites` entries only for per-host
   whitelist, CF validation, logging, or enablement overrides. `enabled=true`
   controls blocking only. `require_cf_header` defaults to `false`, so direct
   sites and CF sites with correctly restored Real IPs both use `$remote_addr`.
   Set `require_cf_header=true` only for optional strict CF-header validation.
   If Real IP is already configured in `http`, do not
   include the optional [`cloudflare-realip.conf`](../conf/cloudflare-realip.conf)
   again. Never trust a client-supplied CF header from an untrusted hop.
4. Run `nginx -t` with the actual main config and reload only after it passes.
   Confirm the load notice (count and SHA-256), local-only status endpoint, an
   allowed IP, a blocked IP (444), and CF header rejection (403) if enabled.

`init()` must run before workers start. A worker-phase call is rejected: it
would repeat file verification and lose pre-worker memory sharing. IPv6 passes
this IPv4 list. Cloudflare cache hits do not reach the origin. Review static,
API, upload, download, internal redirect, and existing WAF/auth hook coverage.
From Lua module v1.3.4, CF header validation runs only when
`require_cf_header=true` is explicitly configured.

With `real_ip_recursive on`, when the peer is in `set_real_ip_from`, Nginx
selects the last non-trusted address from the configured header; with it off,
Nginx uses the header's last address. `CF-Connecting-IP` normally contains one
address, so the switch usually makes no practical difference on a standard CF
path. Keeping it on is fine; the security boundary is trusting only actual CF
egress ranges. IPv6 CF ranges restore addresses received through IPv6 edge
connections but do not add IPv6 blocklist support to this project.

## Updates and rollback

`scripts/update-ip-blacklist` is a Linux example for locking, downloading,
building a candidate, checking count changes, backing up, atomically replacing,
testing Nginx, reloading, and verifying the new digest. Fill in
[`deploy/ipblacklist-update.conf.example`](../deploy/ipblacklist-update.conf.example)
with your paths, first run it manually, and enable the included systemd timer
only if desired. A failed update retains the last good list; use the site's
`enabled=false` setting to stop blocking instead of publishing an empty list.

To recover an earlier saved version, use `scripts/rollback-ip-blacklist` with
the exact version filename from `VERSIONS_DIR`, then verify the status digest
and request behavior. Restrict the status endpoint to localhost/admin access;
do not expose a full list or management endpoint publicly.

Enable hit logging only with sampling and a per-worker rate limit. If using CF,
refresh trusted proxy ranges. Review upstream feed rights and false positives
before scheduled updates. Local test results are in [performance](PERFORMANCE.md).
