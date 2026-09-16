# IPBLV001 binary format · [简体中文](FORMAT.zh-CN.md)

Version 1 is a portable, exact-IPv4 set. It is not XDB, a CIDR table, a range
table or an IPv6 format. Every multibyte integer is little-endian.

| Offset | Bytes | Field |
|---:|---:|---|
| 0 | 8 | ASCII magic `IPBLV001` |
| 8 | 4 | format version `uint32`, value 1 |
| 12 | 4 | header length `uint32`, value 64 |
| 16 | 8 | address count `uint64` |
| 24 | 8 | build Unix timestamp in seconds, `uint64` |
| 32 | 32 | raw SHA-256 of the complete address data section |
| 64 | `count * 4` | strictly increasing, unique `uint32` addresses |

The numeric meaning of `a.b.c.d` is `a*2^24 + b*2^16 + c*2^8 + d`. Each
numeric value is serialized little-endian. Consumers must reject an incorrect
magic, version or header size; zero/excess count; non-exact file size; digest
mismatch; and every adjacent pair where the second value is not greater.

The build timestamp identifies this artifact build, not an upstream update.
Source URL/path, source hash, raw/valid/unique counts, duplicate count and any
recognized source update timestamp live in the adjacent JSON manifest. They are
not on the request path.

Format changes require a new magic/version; v1 semantics must not be widened.
