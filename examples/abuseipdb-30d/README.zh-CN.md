# 151,253 条 IPv4 验证案例 · [English](README.md)

上游[30 天 IPv4 名单](https://github.com/borestad/blocklist-abuseipdb/blob/main/abuseipdb-s100-30d.ipv4)
在 2026-09-15 的本地快照中有 **151,253 条去重 IPv4**，构建后为
**605,076 字节** `IPBLV001` 文件。[snapshot.json](snapshot.json)记录原始源、
二进制及地址段的 SHA-256；该快照用于已有的本地功能和压力测试。

仓库包含完整的 `snapshot.bin` 和公开校验摘要 `snapshot.json`。上游仓库注明
数据来自 AbuseIPDB，但未标出允许下游再分发的许可证；项目 MIT 许可证不覆盖
该第三方名单，使用或再分发前需自行确认相应授权。

从仓库根目录获取上游**当前**名单并构建：

```sh
go build -o ipblacklist-builder ./cmd/ipblacklist-builder
./ipblacklist-builder build \
  --url https://raw.githubusercontent.com/borestad/blocklist-abuseipdb/main/abuseipdb-s100-30d.ipv4 \
  --output case-current.bin
./ipblacklist-builder verify --input case-current.bin --json
```

上游不断更新，新构建的条数和摘要通常不同。使用真实威胁名单时应配置
精确 IP 白名单及变更幅度审核，参见[运维说明](../../docs/OPERATIONS.zh-CN.md)。
