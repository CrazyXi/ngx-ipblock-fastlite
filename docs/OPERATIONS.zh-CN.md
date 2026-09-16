# 运维说明 · [English](OPERATIONS.md)

这里说明首次接入、更新和回滚。环境要求与简单配置见[中文 README](../README.zh-CN.md)。
示例路径只是占位，需按实际 Nginx 安装调整。

## 首次接入

1. 准备非空 TXT 名单，先用 `ipblacklist-builder build` 和 `verify` 构建、
   校验。工具会拒绝错误或空输入，且不覆盖已有输出。
2. 将通过验证的 `current.bin` 和 `lua/ip_blacklist.lua` 放到 Nginx worker
   可读的目录。按[合并示例](../conf/nginx-integration.conf.example)完成两个
   **必需**步骤：把 `.init()` 放进现有 `init_by_lua*` 加载名单；把 `.access()`
   放进 `access_by_lua*` 才会逐请求检查。只配置 `.init()` 不会产生拦截。
   若下级 `server`/`location` 已有 WAF/鉴权访问钩子，必须将调用合并进去。
3. 示例使用 `default_site = { enabled = true }`，因此所有实际调用 `.access()`
   的站点都会检查。`sites` 仅用于按域名覆盖白名单、CF 校验、日志或启停。
   `enabled=true` 只控制黑名单启停。`require_cf_header` 默认是 `false`，模块
   直接检查 `$remote_addr`；直连站点以及已经正确恢复真实 IP 的 CF 站点都可
   保持该默认值。`require_cf_header=true` 只是可选的严格 CF 头校验。如果
   `http` 中已有 Real IP 配置，不要再次
   引入[可选网段文件](../conf/cloudflare-realip.conf)；不能信任非可信来源的 CF 头。
4. 针对实际主配置执行 `nginx -t`，通过后才 reload。核对加载日志中的
   条数和 SHA-256、本机诊断接口、放行 IP、命中 444；启用 CF 校验时再测
   缺失/非法头部 403。

`init()` 只能在 worker 启动前执行；模块会拒绝 worker 阶段调用，因为它会
重复校验文件并失去 worker 之间的内存共享。IPv6 跳过本 IPv4 名单。
Cloudflare 缓存命中不回源。逐一检查静态文件、API、上传、下载、内部跳转
和既有 WAF/鉴权钩子的覆盖范围。Lua 模块从 v1.3.4 起，只有显式设置
`require_cf_header=true` 才会要求并校验 `CF-Connecting-IP`。

`real_ip_recursive on` 表示：当连接来源属于 `set_real_ip_from` 的可信代理时，
Nginx 会沿指定头部寻找最后一个非可信地址作为 `$remote_addr`；关闭时直接使用
头部最后一个地址。`CF-Connecting-IP` 通常只有一个地址，所以开关在标准 CF
链路中通常没有实际差异。你的 `on` 配置可以保留，关键是只把真实 CF 出口网段
列入 `set_real_ip_from`，绝不能信任所有来源。IPv6 CF 网段用于恢复经过 IPv6
边缘连接的访客地址，不代表本项目已经支持 IPv6 黑名单。

## 更新与回滚

`scripts/update-ip-blacklist` 是 Linux 示例，包含加锁、下载、候选构建、
条数变化审核、备份、原子替换、Nginx 配置检查、reload 和新摘要健康检查。
先修改 [`deploy/ipblacklist-update.conf.example`](../deploy/ipblacklist-update.conf.example)
中的路径并手动试运行；需要时再启用附带的 systemd timer。更新失败保留
旧名单；停用封禁应设置站点 `enabled=false`，而不是发布空名单。

恢复历史版本时，用 `scripts/rollback-ip-blacklist` 指定 `VERSIONS_DIR`
里准确的文件名，再核对诊断摘要和请求行为。诊断端点只允许本机或管理员访问，
不要公开整份名单或管理接口。

命中日志要配置采样与每 worker 限速。使用 CF 时需复核代理网段；定时更新前
还应核查上游名单使用权和误封风险。本地测试数据见[性能记录](PERFORMANCE.zh-CN.md)。
