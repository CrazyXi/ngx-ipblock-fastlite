# 验证记录 · [English](VALIDATION.md)

## 当前自动化验证

- `go test ./...`、`go vet ./...` 和构建工具编译通过。
- 严格 TXT 解析、去重、空名单拒绝及 `IPBLV001` 格式/摘要/排序校验有单元测试。
- Debian 12 x86_64、OpenResty 1.31.1.1、Test::Nginx 0.32：集成测试
  11/11 通过。
- 同一 Linux 环境的严格 Valgrind 测试 11/11 通过；吞吐和 PSS 数据见
  [Linux 基准](BENCHMARK.zh-CN.md)。

## 151,253 条地址案例

[验证快照](../examples/abuseipdb-30d/README.zh-CN.md)包含 151,253 条去重 IPv4，
`snapshot.bin` 为 605,076 字节，数据摘要记录在 `snapshot.json`。

该快照曾在 Windows x64 OpenResty 1.25.3.2 上进行全部成员、边界、相邻、
随机查询、HTTP 444、压力和内存测试；历史数字见
[Windows 性能记录](PERFORMANCE.zh-CN.md)。当前 Linux 测试也使用同一快照。

## 不代表线上验收

公开测试不能替代实际环境中的配置验收。上线前仍需检查 access 钩子覆盖、
WAF/鉴权顺序、真实代理链、reload/回滚、业务延迟和误封风险。使用 Cloudflare
时还需验证可信代理网段、源站访问控制和 Pseudo IPv4 设置。
