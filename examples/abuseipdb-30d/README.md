# 151,253-IPv4 validation case · [简体中文](README.zh-CN.md)

The 2026-09-15 local snapshot of the upstream
[30-day IPv4 feed](https://github.com/borestad/blocklist-abuseipdb/blob/main/abuseipdb-s100-30d.ipv4)
contained **151,253 unique IPs**. The builder produced a **605,076-byte**
`IPBLV001` file. Exact source, binary, and payload SHA-256 values are in
[snapshot.json](snapshot.json); the snapshot was used for the historical
Windows tests and the current Linux Test::Nginx/Valgrind tests.

The repository includes the exact `snapshot.bin` and public checksum record
`snapshot.json`. The upstream repository credits AbuseIPDB but does not state
a license granting downstream redistribution. This project's MIT license does
not cover the third-party list; verify the applicable rights before use or
redistribution.

To build a case with the provider's **current** feed from the repository root:

```sh
go build -o ipblacklist-builder ./cmd/ipblacklist-builder
./ipblacklist-builder build \
  --url https://raw.githubusercontent.com/borestad/blocklist-abuseipdb/main/abuseipdb-s100-30d.ipv4 \
  --output case-current.bin
./ipblacklist-builder verify --input case-current.bin --json
```

The upstream list changes frequently. A new build will not necessarily have the
historical count or hashes. Keep a site's exact-IP whitelist and change-review
thresholds when using any real threat feed; see [operations](../../docs/OPERATIONS.md).
