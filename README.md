# NGX IPBlock FastLite

[English](README.en.md) · [格式](FORMAT.zh-CN.md) · [运维](docs/OPERATIONS.zh-CN.md) · [测试](docs/TESTING.zh-CN.md)

## 功能特点

- 精确匹配 IPv4，命中后由 Nginx 返回 **444** 并断开连接。
- Go 工具将文本名单转换为排序二进制；加载时校验文件长度、顺序和 SHA-256。
- 地址存入紧凑的 FFI `uint32_t` 数组，先用 IPv4 高 16 位定位索引桶，再在桶内
  二分查找，避免扫描整份名单。
- 名单在 `init_by_lua*` 阶段加载一次；在支持 pre-fork/COW 的环境中由 worker
  共享只读内存。请求期间不读文件，不访问数据库、Redis 或远程接口。
- 支持全局启停、按域名配置、精确 IP 白名单、可选 CF 请求头校验以及限速采样日志。

## 环境要求

- Nginx 需要 `ngx_http_lua_module`、LuaJIT/FFI 和 `resty.sha256`。
- Go 1.22+ 只用于生成名单，Nginx 运行时不需要 Go。
- 已在 Windows x64 OpenResty 1.25.3.2 本地验证；其他版本待验证。
- 当前只支持单个 IPv4 地址，不支持 CIDR 和 IPv6。

## 性能横评

**测试机：** Intel Core Ultra 5 225；WSL 2 / Debian 12（内核 6.18.33.2），10 个逻辑 CPU，WSL 可见总内存约 15.32 GiB。OpenResty 1.31.8.1：单 worker 绑定 CPU 0，返回 HTTP 204 的后端绑定 CPU 2。wrk 4.1.0：2 个线程绑定 CPU 3、4，使用 32 个长连接，经本机回环接口压测。

每格依次为 **放行吞吐 · 总内存占用 · 程序 + IP 库体积**。体积不含 OpenResty 或 APISIX 宿主；0 B 表示未安装拦截组件。

| 方案 | 15 万 IP<br>放行吞吐 · 总内存占用 · 程序 + IP 库体积 | 40 万 IP<br>放行吞吐 · 总内存占用 · 程序 + IP 库体积 | 60 万 IP<br>放行吞吐 · 总内存占用 · 程序 + IP 库体积 |
|---|---|---|---|
| 原生 OpenResty（无 IP 拦截） | 156,829 req/s · 8.89 MiB · 0 B | 154,831 req/s · 8.75 MiB · 0 B | 155,865 req/s · 8.90 MiB · 0 B |
| ngx-ipblock-fastlite | 143,473 req/s · 11.38 MiB · 0.59 MiB | 147,814 req/s ¹ · 12.61 MiB ¹ · 1.54 MiB | 145,726 req/s · 13.50 MiB · 2.31 MiB |
| NGINX geo | 139,767 req/s · 125.21 MiB · 2.46 MiB | 138,480 req/s ¹ · 265.22 MiB ¹ · 6.56 MiB | 136,059 req/s · 296.40 MiB · 9.88 MiB |
| lua-resty-ipmatcher | 138,616 req/s · 31.07 MiB · 2.04 MiB | 136,827 req/s · 56.60 MiB · 5.43 MiB | 137,113 req/s · 80.70 MiB · 8.17 MiB |
| iptables + ipset | 156,281 req/s ² · 12.30 MiB · 6.62 MiB | 157,846 req/s ² · 18.81 MiB · 12.86 MiB | 157,931 req/s ² · 22.82 MiB · 17.89 MiB |
| APISIX ip-restriction | 65,271 req/s · 132.92 MiB · 2.47 MiB | 63,415 req/s · 245.73 MiB · 6.57 MiB | 61,564 req/s · 384.49 MiB · 9.88 MiB |
| APISIX（未启用 IP 拦截） | 68,512 req/s ¹ · 43.86 MiB ¹ · 0 B | 69,649 req/s · 44.04 MiB · 0 B | 68,480 req/s · 43.24 MiB · 0 B |

¹ 为波动后的复测值。² 为 ipset 独立配对复测吞吐，三档均未观察到稳定的放行吞吐差异。内存包含热态前端进程和独立内核名单集合。项目行是低 16 位地址后缀优化版（Lua 文件 SHA-256 前缀 `617e818eb705`）的实测，不代表其他版本或线上流量。[测试方法与原始记录](docs/WSL-OPTIMIZED.zh-CN.md)。

## 安装

### 1. 生成名单

准备非空 TXT 文件，每行一个 IPv4 地址。允许空行和注释；错误地址会停止构建。

```sh
go build -o ipblacklist-builder ./cmd/ipblacklist-builder
./ipblacklist-builder build --input ips.txt --output candidate.bin
./ipblacklist-builder verify --input candidate.bin --json
```

### 2. 部署文件

将下列文件放到服务器：

| 必需文件 | 服务器路径 |
|---|---|
| `lua/ip_blacklist.lua` | `/opt/ngx-ipblock-fastlite/lua/ip_blacklist.lua` |
| 生成的 `candidate.bin` | `/opt/ngx-ipblock-fastlite/current.bin` |

`/opt/ngx-ipblock-fastlite` 是示例路径，可以改为其他 Nginx 可读目录。

```sh
sudo install -d -m 0755 /opt/ngx-ipblock-fastlite/lua
sudo install -m 0644 candidate.bin /opt/ngx-ipblock-fastlite/current.bin
sudo install -m 0644 lua/ip_blacklist.lua /opt/ngx-ipblock-fastlite/lua/ip_blacklist.lua
```

Nginx 运行时只需要上面两个文件：

- `ipblacklist-builder` 只用于生成或更新 `current.bin`，不用上传；只有计划让
  服务器自动更新名单时，才另外安装 Linux 版本。
- `conf/nginx-integration.conf.example` 只是参考，不用上传；从中选择需要的
  配置，合并到现有 `nginx.conf`。
- Windows 构建工具名为 `ipblacklist-builder.exe`，不能作为 Linux 程序使用。

### 3. 配置 Nginx

把下面两部分加入现有 `http {}`：`init()` 加载名单，`access()` 检查请求。
两者都必须配置。宝塔环境的主配置通常是
`/www/server/nginx/conf/nginx.conf`，请以 `nginx -T` 显示的实际配置为准。

```nginx
# 在 http {} 中，合并到现有 init_by_lua*：
lua_package_path "/opt/ngx-ipblock-fastlite/lua/?.lua;;";
init_by_lua_block {
    require("ip_blacklist").init({
        path = "/opt/ngx-ipblock-fastlite/current.bin",
        default_site = {
            enabled = true,            -- 启用黑名单，与 Cloudflare 无关。
            require_cf_header = false, -- 默认值：直接使用 Nginx 的 $remote_addr。
        },

        -- 可选：只在需要严格校验 CF 请求头的域名上启用。
        -- sites = {
        --     ["www.example.cn"] = {
        --         enabled = true,
        --         require_cf_header = true,
        --     },
        -- },
    })
}

# 必需：把黑名单查询接入每个请求的访问阶段。
access_by_lua_block {
    require("ip_blacklist").access()
}
```

如果 `server` 或 `location` 已有 `access_by_lua*`（例如 WAF），请将
`require("ip_blacklist").access()` 加入原有代码，不要重复定义同层钩子。

`enabled = true` 只表示启用黑名单，不是 CF 开关。`require_cf_header` 默认是
`false`，模块直接检查 Nginx 的 `$remote_addr`：直连站点无需 CF 配置；使用
Cloudflare 且 Nginx 已正确恢复真实 IP 时也无需改成 `true`。

`require_cf_header = true` 是可选的严格校验：它要求 `CF-Connecting-IP` 存在，
并与 Real IP 模块恢复后的 `$remote_addr` 一致，否则返回 403。只有需要这层
校验时才按域名开启。如果 Nginx 已有 Real IP 配置，不要重复引入
[可选网段文件](conf/cloudflare-realip.conf)。完整配置见[示例](conf/nginx-integration.conf.example)。

reload 前先运行 `nginx -t`，再验证正常 IP 和名单 IP（444）。更新与回滚见
[运维说明](docs/OPERATIONS.zh-CN.md)。

## 验证案例

[151,253 条 IPv4 验证案例](examples/abuseipdb-30d/README.zh-CN.md)记录了
2026-09-15 本地使用的 **151,253 条精确 IPv4**，仓库包含对应的
`snapshot.bin` 和校验摘要。案例也说明如何获取当前名单并自行构建。

本地测试中，地址数组与索引合计约 **847 KiB**；完整测试路径相对无钩子路径
的吞吐差异中位数为 **2.90%**。这些数字仅代表本地 Windows 测试，不能直接
推算线上性能。详情见[性能记录](docs/PERFORMANCE.zh-CN.md)。

## 注意事项

- 只能在 `init_by_lua*` 加载，不能放到 `init_worker_by_lua*`；模块会拒绝
  worker 阶段初始化。空名单会被拒绝，更新失败保留旧名单；停用请显式关闭站点。
- IPv6 不命中这份 IPv4 名单。Cloudflare 缓存命中不回源。
- 代码和文档使用 [MIT 许可证](LICENSE)；MIT 许可证不覆盖第三方名单，
  使用或再分发前需自行确认上游授权。
