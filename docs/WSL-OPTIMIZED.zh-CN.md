# 优化版 WSL 横评 · [English](WSL-OPTIMIZED.md)

本机 WSL Debian 12，单 worker、同一 HTTP 204 后端、32 个长连接。六种拦截实现中，nftables 因无法可靠测得其集合内存，不列入横评。原生 OpenResty 和 APISIX 对照不加载名单；ipset 专项对照保留已加载集合，只切换规则。[数据来源与环境](WSL-COMPARISON.zh-CN.md)

首页横评表每格从左到右：**放行吞吐 · 总内存 · 安装体积（拦截组件 + IP 库，不含 OpenResty）**。

[三档横评表见仓库首页](../README.md#性能横评)。

**总内存** = 热态前端 master 和子进程的 PSS 之和 + 独立内核名单集合。ipset 使用集合自身报告的 `Size in memory`；进程内名单已含在 PSS 中，不再重复相加。这里不是整台 WSL 的内存，也不含共同后端、压测客户端或通用内核开销。nftables 只有虚拟机级内核计数差值，不能可靠归到单个集合，因此从对比表移除。

**安装体积**仅统计组件文件及可加载的名单或规则文件，不计 OpenResty、APISIX 宿主和离线构建工具。geo 已编入宿主，只计规则；ipset 计相关工具、库及规则文件；APISIX 插件计插件文件和带名单的路由配置。对照行的 0 B 表示没有安装拦截组件或名单，不是宿主本身为零。

放行吞吐统计一个固定未命中地址收到的 HTTP 204 响应，不是命中拒绝速率。除 ² 外，吞吐取三个 3 秒样本的中位数。¹ 表示因样本波动而重测（15 万 APISIX 对照，40 万本项目和 geo）。P99、HTTP 403 命中吞吐及首轮数值见原始记录；[默认 444 断连测试](WSL-COMPARISON.zh-CN.md#优化前后对照)另列。

**² iptables + ipset：独立配对复测。** 旧记录只在正式压测前读取 DROP 计数，不能核验每个采样窗口经过规则的流量。新测试保持同一代理进程，在独立网络命名空间交替开关规则，每档四对，顺序为 AB / BA / AB / BA。启用时对 18080 端口执行一次源地址集合检查：未命中 ACCEPT，否则 DROP。每次先用绑定源地址的探测验证命中与放行，再预热 1 秒、测 5 秒。所有启用窗口均验证 ACCEPT 包计数增加、DROP 不增加；15 万档首轮的无规则样本跨度超过 15 倍，因此单独完整复测，使用复测而非首轮。表中吞吐来自 wrk 完成的 HTTP 请求，不是包计数。内存沿用原横评实测。

关规则只用于测量启用名单检查对放行吞吐的影响，不是另一种拦截方案。与 ² 匹配的无规则对照（15 / 40 / 60 万）为 155,273 req/s / 157,995 req/s / 157,948 req/s。逐对吞吐差的中位数（最小 … 最大）为 +0.7% (-2.1% … +2.6%) / +0.2% (-1.9% … +1.3%) / +0.4% (-1.2% … +0.5%)。不要将 ² 与首行不同轮次的绝对值相减，也不要把放行请求速率解释为防火墙的丢包能力。

## 原始记录

本轮模块 SHA-256 `617e818eb7056454553a9e401a0f9cbfac7459c39bedb1c6d6a1a85b25fbd414`。 [首轮](../benchmark/results/wsl-optimized-samples.jsonl) · [复测](../benchmark/results/wsl-optimized-repeat.jsonl) · [内存与磁盘原始读数](../benchmark/results/wsl-optimized-footprint.json) · [横评脚本](../benchmark/wsl_compare.py) · [体积测量脚本](../benchmark/wsl_footprint.py) · [ipset 配对样本](../benchmark/results/wsl-ipset-paired.jsonl) · [15 万复测](../benchmark/results/wsl-ipset-paired-repeat.jsonl) · [配对脚本](../benchmark/wsl_ipset_paired.py)
