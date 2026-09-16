# 验证记录 · [English](VALIDATION.md)

Go `test`/`vet` 已通过；严格 TXT 解析器及 `IPBLV001` 校验器有单元测试和
CLI 验证。Linux amd64/arm64 构建产物已从 Go 源码交叉编译，但未在生产主机运行；
构建产物由 Git 忽略。

[15 万 IP 案例](../examples/abuseipdb-30d/README.zh-CN.md)于 2026-09-15
获取，并在隔离的 Windows x64 OpenResty 1.25.3.2 上测试。名单去重后有
151,253 条 IPv4、605,076 字节二进制，地址段 SHA-256 见 `snapshot.json`。
本地最终模块 v1.3.3 检查了 684,835 个成员、边界、相邻和随机目标，
通过 HTTP 行为测试，并完成压力及内存采样。[数字](PERFORMANCE.zh-CN.md)
不代表 Linux 或线上容量保证。

仍需按实际主机核查业务钩子/location 覆盖、Linux COW/PSS、多 worker reload
与真实后端延迟。使用 CF 时还需核查可信代理链与 Pseudo IPv4 设置。
验收项见[测试](TESTING.zh-CN.md)。
