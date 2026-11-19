# NGX IPBlock FastLite

面向带 `ngx_http_lua_module` 的 Nginx/OpenResty 的精确 IPv4 黑名单模块。
名单由 Go 工具离线生成，在 worker 创建前一次性载入内存；请求阶段不读文件，
命中后，Nginx 默认以 **444** 关闭连接。

[English](README.en.md) · [格式](FORMAT.zh-CN.md) · [运维](docs/OPERATIONS.zh-CN.md) · [测试](docs/TESTING.zh-CN.md) · [Linux 基准](docs/BENCHMARK.zh-CN.md)

## 功能特点

- 精确 IPv4 名单在磁盘上每条占 4 字节；载入后按地址高 16 位分桶，只保存低 16 位后缀（每条 2 字节）。另有约 256 KiB 的前缀索引，查询只在对应桶内做二分查找。
- 可选 IPv4 网段封禁（`cidrs`）：启动时合并重叠或相邻的网段。未配置网段时只查精确名单；配置后仅在精确名单未命中时查网段。
- 在 `init_by_lua*` 阶段加载；pre-fork worker 可通过 COW 共享只读名单。
- 加载时校验文件格式、长度、地址顺序和地址数据的 SHA-256。校验失败会终止本次初始化。
- 支持全局或按域名启停、精确 IP 白名单、命中日志及可选的 CF 头部一致性检查。

## 支持范围

- 需要 `ngx_http_lua_module`、LuaJIT/FFI 和 `resty.sha256`；推荐直接使用 OpenResty。
- 已测试：Windows x64 OpenResty 1.25.3.2、Debian 12 x86_64 OpenResty 1.31.1.1。
- Go 1.22+ 仅用于生成名单，Nginx 运行时不需要 Go。
- 精确名单每行一个 IPv4，构建和加载默认上限均为 **200 万个去重地址**。IPv4 网段通过可选的 `cidrs` 配置添加。IPv6 和任意起止地址范围暂不支持。
- 只处理 HTTP 请求，不处理 `stream {}` 中的 TCP/UDP 流量。

## 性能横评

**测试机：** Intel Core Ultra 5 225；WSL 2 / Debian 12（内核 6.18.33.2），10 个逻辑 CPU，WSL 可见总内存约 15.32 GiB。OpenResty 1.31.8.1：单 worker 绑定 CPU 0，返回 HTTP 204 的后端绑定 CPU 2。Go 压测客户端绑定 CPU 3、4，32 并发，每次尝试新建一条 TCP 连接，只访问本机回环接口。

负载为 **90% 非名单来源 + 10% 名单来源**。每格依次为 **混合尝试次数/秒 · 总内存占用 · 程序 + IP 库体积**。体积不含 OpenResty 或 APISIX 宿主；0 B 表示未安装拦截组件。

**IP 库来源：** [borestad/blocklist-abuseipdb](https://github.com/borestad/blocklist-abuseipdb)。15 万、40 万、60 万档分别从[同一固定版本](https://github.com/borestad/blocklist-abuseipdb/tree/eb4a4a4ec7b2453b176cec0e7da0e0fb3033121c)的 30 天、180 天、365 天 IPv4 名单中抽样；每档的所有方案使用相同地址。

| 方案 | 15 万 IP<br>混合尝试/秒 · 总内存占用 · 程序 + IP 库体积 | 40 万 IP<br>混合尝试/秒 · 总内存占用 · 程序 + IP 库体积 | 60 万 IP<br>混合尝试/秒 · 总内存占用 · 程序 + IP 库体积 |
|---|---|---|---|
| 原生 OpenResty（无 IP 拦截） | 70,180 次/秒 · 8.84 MiB · 0 B | 73,724 次/秒 · 8.94 MiB · 0 B | 72,939 次/秒 · 9.00 MiB · 0 B |
| ngx-ipblock-fastlite | 72,170 次/秒 · 11.51 MiB · 0.59 MiB | 66,972 次/秒 · 12.72 MiB · 1.54 MiB | 72,600 次/秒 · 13.54 MiB · 2.31 MiB |
| NGINX geo | 74,016 次/秒 · 86.96 MiB · 2.46 MiB | 72,917 次/秒 · 211.69 MiB · 6.56 MiB | 71,640 次/秒 · 295.57 MiB · 9.88 MiB |
| lua-resty-ipmatcher | 64,460 次/秒 · 30.95 MiB · 2.04 MiB | 67,297 次/秒 · 54.63 MiB · 5.43 MiB | 65,113 次/秒 · 86.82 MiB · 8.17 MiB |
| iptables + ipset | 1,572 次/秒 · 12.76 MiB · 6.62 MiB | 1,575 次/秒 · 19.14 MiB · 12.86 MiB | 1,574 次/秒 · 22.95 MiB · 17.89 MiB |
| APISIX ip-restriction | 45,624 次/秒 · 132.98 MiB · 2.47 MiB | 47,321 次/秒 · 247.61 MiB · 6.57 MiB | 47,648 次/秒 · 385.24 MiB · 9.88 MiB |
| APISIX（未启用 IP 拦截） | 48,181 次/秒 · 44.93 MiB · 0 B | 51,294 次/秒 · 44.04 MiB · 0 B | 47,706 次/秒 · 43.19 MiB · 0 B |

速率包含已核验的放行和拦截。无规则对照全部返回 204；启用拦截时，每轮必须恰好放行 90%、拦截 10%，无异常才纳入。项目命中以 444 断连，其余 HTTP 方案返回 403。ipset 的 DROP 不回复客户端：**32 并发、200 ms 连接超时决定了约 1,600 次/秒的客户端上限，不是内核丢包能力。** 内存为热态前端 PSS 加内核名单集合。[测试方法与逐轮记录](docs/WSL-OPTIMIZED.zh-CN.md)。

## 安装使用

### 1. 生成名单

TXT 文件每行一个 IPv4；允许空行和注释。空名单或非法地址会导致构建失败。

```sh
go build -o ipblacklist-builder ./cmd/ipblacklist-builder
./ipblacklist-builder build --input ips.txt --output candidate.bin
./ipblacklist-builder verify --input candidate.bin --json
```

### 2. 部署文件

Nginx 运行时只需要两个文件：

| 仓库文件 | 示例部署路径 |
|---|---|
| `lua/ip_blacklist.lua` | `/opt/ngx-ipblock-fastlite/lua/ip_blacklist.lua` |
| 生成的 `candidate.bin` | `/opt/ngx-ipblock-fastlite/current.bin` |

```sh
sudo install -d -m 0755 /opt/ngx-ipblock-fastlite/lua
sudo install -m 0644 lua/ip_blacklist.lua /opt/ngx-ipblock-fastlite/lua/ip_blacklist.lua
sudo install -m 0644 candidate.bin /opt/ngx-ipblock-fastlite/current.bin
```

`/opt/ngx-ipblock-fastlite` 只是推荐示例，可改成任意 Nginx 可读目录。
`ipblacklist-builder` 和 `conf/nginx-integration.conf.example` 都不是运行时必需文件。

### 3. 接入 Nginx

下面配置放在现有 `http {}` 中。`init()` 负责加载名单，`access()` 才负责逐请求
查询，两者缺一不可。

```nginx
lua_package_path "/opt/ngx-ipblock-fastlite/lua/?.lua;;";

init_by_lua_block {
    require("ip_blacklist").init({
        path = "/opt/ngx-ipblock-fastlite/current.bin",
        -- 可选：所有启用站点共用的 IPv4 网段。
        -- cidrs = { "192.0.2.0/24", "198.51.100.128/25" },
        default_site = {
            enabled = true,
            require_cf_header = false,
        },

        -- 可选：按域名使用独立配置。站点项不会继承 default_site，
        -- 因此必须显式保留 enabled = true。
        -- sites = {
        --     ["www.example.cn"] = {
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

网段写在 `init()` 的顶层，使用网络地址和 `/0` 至 `/32` 的前缀长度。
例如 `192.0.2.0/24` 覆盖 `192.0.2.0` 至 `192.0.2.255`，写成
`192.0.2.1/24` 会使初始化失败。`/0` 覆盖全部 IPv4。站点白名单优先。
修改配置后 reload 即生效，已有精确名单二进制可继续使用。未配置网段或
`cidrs = {}` 时，请求沿用原查询函数。

这里的 `?` 是 Lua 模块名占位符；`require("ip_blacklist")` 会定位到
`ip_blacklist.lua`，不是任意文件匹配。路径末尾的 `;;` 保留 Lua 默认搜索路径。

`init_by_lua*` 表示这一组指令（通常使用 `_block` 或 `_file` 形式），不是配置文件名。
`init()` 必须在 `init_by_lua*` 执行，不能放进 `init_worker_by_lua*`。

同一层级已有 Lua/WAF 配置时不要再定义第二个同类指令：

- 已有 `lua_package_path`：把本项目路径追加到原字符串中，不能重复声明指令。
- 已有 `init_by_lua_file`：把 `require(...).init(...)` 加入原初始化 Lua 文件。
- 已有 `access_by_lua_file` 或 `access_by_lua_block`：把 `.access()` 加入原访问钩子。
- `server` 或 `location` 自己定义了 `access_by_lua*` 时，会覆盖从 `http` 继承的
  钩子；这些位置也必须合并 `.access()`，否则对应请求不会检查黑名单。

完整合并参考：[conf/nginx-integration.conf.example](conf/nginx-integration.conf.example)。
宝塔等面板环境请先用 `nginx -T` 确认实际生效的配置和 Lua 文件，不要仅按目录名猜测。

## Cloudflare（可选）

不用 Cloudflare 时，不需要任何 CF 配置，保持 `require_cf_header = false` 即可。

使用 Cloudflare 时，应先用 Nginx Real IP 模块和可信 `set_real_ip_from` 网段正确
恢复 `$remote_addr`；若已有 Real IP 配置，不要重复引入网段文件。
[cloudflare-realip.conf](conf/cloudflare-realip.conf) 仅为可选参考，并非必需文件。

`require_cf_header = true` 只会检查 `CF-Connecting-IP` 是否存在并与
`$remote_addr` 一致；失败时默认返回 403。它不能单独证明连接来自 Cloudflare，
也不能替代可信代理网段或源站访问控制。

## 命中日志（可选）

命中日志默认关闭，在 `default_site` 或相应 `sites` 项中配置：

| 参数 | 默认值 | 作用 |
|---|---:|---|
| `log_hits` | `false` | 命中日志总开关 |
| `log_sample_rate` | `0` | `0` 关闭采样，`0.01` 表示约 1%；设为 `1` 时所有命中均可记录，实际条数仍受每秒上限约束 |
| `log_rate_per_second` | `10` | 每个 worker 每秒上限；`0` 禁止记录 |

只有 `log_hits = true`、采样比例大于 0、限速大于 0 时才会写日志。日志使用
`ngx.WARN`，进入当前 Nginx `error_log`；日志级别需为 `warn` 或更详细。
这些参数只控制日志，不影响 444 拦截。生产环境建议采样并限速。

## 验证与更新

```sh
nginx -t
nginx -s reload
```

reload 后至少验证：正常 IP 放行、名单 IP 在到达业务前被 444 断开、现有 WAF/
鉴权仍执行。444 不返回 HTTP 状态行，客户端通常显示空响应或连接被关闭。

更新、原子替换和回滚见[运维说明](docs/OPERATIONS.zh-CN.md)。自动化测试见
[测试说明](docs/TESTING.zh-CN.md)。

## 151,253 条 IP 示例

仓库包含一个来自 [borestad/blocklist-abuseipdb 的 30 天名单](https://github.com/borestad/blocklist-abuseipdb/blob/main/abuseipdb-s100-30d.ipv4)、含 151,253 条精确 IPv4 的
[验证快照](examples/abuseipdb-30d/README.zh-CN.md)，二进制大小为 605,076 字节。
该第三方名单的授权不属于本项目 MIT 许可证，使用或再分发前请确认上游许可。

Windows 历史测试和 Linux Test::Nginx/Valgrind 结果分别见
[Windows 性能记录](docs/PERFORMANCE.zh-CN.md)与[Linux 基准](docs/BENCHMARK.zh-CN.md)。
这些结果用于复现和版本比较，不代表 OpenResty 官方认证，也不能直接推算线上性能。

## 许可证

代码和文档使用 [MIT License](LICENSE)。
