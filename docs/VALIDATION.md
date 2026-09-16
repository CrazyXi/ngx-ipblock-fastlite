# Validation record · [简体中文](VALIDATION.zh-CN.md)

Go `test`/`vet` pass. The builder's strict parser and `IPBLV001` verifier have
unit and CLI coverage. Linux amd64/arm64 binaries were cross-compiled from
Go source but not run on the production host; build artifacts are Git-ignored.

The [151k-IP case](../examples/abuseipdb-30d/README.md) was fetched on
2026-09-15 and used in an isolated Windows x64 OpenResty 1.25.3.2 instance.
It produced 151,253 unique IPv4s, a 605,076-byte binary, and the payload
SHA-256 recorded in `snapshot.json`. The final local module v1.3.3 checked
684,835 member, boundary, adjacent, and random targets, passed HTTP behavior
tests, and was pressure/memory sampled. [Numbers](PERFORMANCE.md) are not a
Linux or production capacity claim.

Still host-specific: actual hook/location coverage, Linux COW/PSS,
multi-worker reload, and normal backend latency. If using CF, also verify the
trusted proxy chain and Pseudo IPv4 settings. Follow [testing](TESTING.md).
