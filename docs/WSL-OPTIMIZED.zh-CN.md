# WSL 横评方法：90% 放行、10% 名单来源 · [English](WSL-OPTIMIZED.md)

数据表和机器配置见[中文首页](../README.md#性能横评)。三档名单均取自 [borestad/blocklist-abuseipdb 固定版本](https://github.com/borestad/blocklist-abuseipdb/tree/eb4a4a4ec7b2453b176cec0e7da0e0fb3033121c)，运行前校验[各档载荷 SHA-256](../benchmark/results/wsl-sources.jsonl)。

## 负载与核验

客户端仅连接隔离网络空间中的 `127.0.0.1:18080`。32 个并发客户端显式绑定源地址：16 个已确认不在名单的回环地址，及从该档排序名单中等间隔选取的 16 个成员。每个客户端每 10 次尝试恰有一次使用名单源地址。每次新建 TCP 连接，不使用代理 IP 头、不重试；前端连接同一 204 后端。

非名单尝试必须全部返回 204。名单来源在无拦截对照中也必须返回 204；本项目必须在未收到任何 HTTP 响应字节时读到 EOF（默认 444）；geo、lua-resty-ipmatcher 和 APISIX 插件必须返回 403。ipset 使用 INPUT 链的源地址集合匹配，对命中端口的名单源执行 DROP；仅当连接超时数和该窗口内核 DROP 包增量均等于计划命中数，样本才有效。其他错误不计成拦截。

每组预热 3,200 次。首轮所有方案各跑 5 × 12,800 次；HTTP 样本较短，因此所有 HTTP 方案另做 5 × 128,000 次，表中统一取长样本。ipset 每个首轮样本已约 8 秒，使用这 5 个样本。每格速率取五个「总尝试数 / 墙钟秒」的中位数，包含正确放行和正确拒绝；不等于成功 HTTP 响应数。内存取同次运行采样后的 master 与子进程 PSS，加上 ipset 自报的集合字节数；不是峰值或整机内存。磁盘值沿用同版本组件与相同名单的[文件体积清单](../benchmark/results/wsl-optimized-footprint.json)，不含宿主。

**比较边界：** DROP 不会回复客户端。32 个槽位、10% 命中、200 ms 连接超时，本身就把 ipset 的混合尝试速率限制在约 `32 / (0.10 × 0.20) = 1,600 次/秒` 以下。这不是内核每秒丢包数上限，也不能据此判断防火墙比 HTTP 方案慢多少。测试不含 TLS、CDN、真实网络延迟或 CIDR；方案依次运行，小差异不作为稳定排名。

## 复现

在 Linux 仓库根目录执行。`lists/{150000,400000,600000}/ips` 是[原横评](WSL-COMPARISON.zh-CN.md)按固定版本生成的三个文件。先创建空的临时父目录；脚本拒绝在宿主网络或挂载空间运行。

```sh
run=$(mktemp -d)
go build -o "$run/mixedclient" ./benchmark/mixedclient
sudo unshare -nm --propagation private python3 benchmark/wsl_compare.py "$run/pilot" lists --mixed-client "$run/mixedclient" --mixed-http-count 12800
sudo unshare -nm --propagation private python3 benchmark/wsl_compare.py "$run/http" lists --mixed-client "$run/mixedclient" --skip-ipset
```

原始记录：[首轮，含 ipset 计数](../benchmark/results/wsl-mixed-pilot.jsonl) · [HTTP 长样本](../benchmark/results/wsl-mixed-http.jsonl)。此前纯放行数据仍保留在[历史记录](../benchmark/results/wsl-optimized-samples.jsonl)，两种负载不能直接计算优化百分比。
