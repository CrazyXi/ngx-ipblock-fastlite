# 测试 · [English](TESTING.md)

从仓库根目录运行：

```sh
go test ./...
go vet ./...
go build -o ipblacklist-builder ./cmd/ipblacklist-builder
./ipblacklist-builder build --input testdata/source-valid.txt --output fixture.bin
./ipblacklist-builder verify --input fixture.bin --json
```

Go 测试覆盖 TXT 注释/BOM/CRLF、严格地址解析、去重、无符号 IPv4 边界、
二进制格式/摘要/排序损坏、空名单拒绝和 CLI。空或错误源文件不会发布候选；
测试文件在临时目录生成。

ngx_lua 集成请使用独立本地监听，检查：

- 普通 IPv4 进入业务，名单成员在业务前断连 444，名单中的白名单成员放行。
- 合法 IPv6 跳过本 IPv4 名单。直连站点无需 CF 头仍能正常工作。
  CF 站点设 `require_cf_header = true` 时，缺失或不匹配的
  `CF-Connecting-IP` 会被拒绝，不能选择伪造的访客 IP。
- 模块在 `init_by_lua*` 加载，worker 阶段调用被拒绝；本机诊断接口显示
  正确条数与地址段 SHA-256。
- 覆盖每个目标业务 location、原有 WAF/鉴权钩子、内部跳转和 WebSocket 握手。
- 新版本能新增和移除 IP；更新失败与回滚时保留可用旧名单。

151,253 条 IP 的 Windows 案例已在 OpenResty 1.25.3.2 上验证全部成员、
边界/随机查询、HTTP 444、压力与内存采样。[记录](PERFORMANCE.zh-CN.md)
仅代表本地；Linux worker 共享、多 worker reload 和线上代理链应在对应环境
单独验收，不能拿本机结果承诺线上性能。
