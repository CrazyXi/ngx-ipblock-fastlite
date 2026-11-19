# 测试说明 · [English](TESTING.md)

## Go 构建与单元测试

在仓库根目录运行：

```sh
go test ./...
go vet ./...
go build -o ipblacklist-builder ./cmd/ipblacklist-builder
./ipblacklist-builder build --input testdata/source-valid.txt --output fixture.bin
./ipblacklist-builder verify --input fixture.bin --json
```

测试覆盖严格 IPv4 解析、注释/BOM/CRLF、去重、无符号边界、空名单拒绝，
以及二进制格式、长度、摘要、排序损坏和 CLI 行为。

## OpenResty 集成测试

需要 Test::Nginx、OpenResty 和仓库中的 151,253 条测试快照：

```sh
export TEST_NGINX_BINARY=/usr/local/openresty/nginx/sbin/nginx
export TEST_NGINX_IPBLOCK_LUA="$PWD/lua"
export TEST_NGINX_IPBLOCK_BIN="$PWD/examples/abuseipdb-30d/snapshot.bin"

prove -v t/001-integration.t t/002-cidr.t
```

该测试验证精确命中、未命中、access 放行和在内容阶段前拒绝。自动化拒绝用
403 取得可断言的 HTTP 响应；默认 444 会直接关闭连接，应另做端到端验证。

本机 WSL 可用 `sudo unshare -n sh t/wsl-smoke.sh` 直接验证后缀分桶边界、CIDR 合并、白名单优先及真实 444 断连，不依赖 Perl 测试框架。

严格内存检查：

```sh
TEST_NGINX_USE_VALGRIND=1 \
TEST_NGINX_VALGRIND_EXIT_ON_FIRST_ERR=1 \
prove -v t/001-integration.t
```

`valgrind.suppress` 只屏蔽 Nginx `ngx_set_environment` 的已知进程退出分配，
不屏蔽本项目调用栈。

Benchmark 的复现命令、原始日志和限制见[Linux 基准](BENCHMARK.zh-CN.md)。

## 部署前验收

- 正常 IPv4 能到达业务；名单 IPv4 在业务前断连 444；白名单成员放行。
- IPv6 不进入本项目的 IPv4 查询路径。
- 每个需要保护的 `server`/`location` 都实际执行 `.access()`，包括已有
  WAF/鉴权、内部跳转和 WebSocket 路径。
- 若启用 `require_cf_header=true`，验证头部缺失或与 `$remote_addr` 不一致时
  返回 403；同时验证可信代理网段和源站访问控制，不能把一致性检查当成来源认证。
- 新名单能新增和移除地址；构建失败、reload 失败和回滚时旧 worker 继续可用。
- 状态接口只允许本机或管理员访问，并显示预期条数与数据 SHA-256。

Windows OpenResty 1.25.3.2 的历史结果见[本地性能](PERFORMANCE.zh-CN.md)；
Debian 12/OpenResty 1.31.1.1 的 Test::Nginx、Valgrind 和 PSS 结果见
[Linux 基准](BENCHMARK.zh-CN.md)。两者都不是线上容量保证。
