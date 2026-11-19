# Linux 基准测试 · [English](BENCHMARK.md)

三档真实名单的横向对比见[首页横评：10% 名单来源下的混合吞吐、总内存和安装体积](../README.md#性能横评)。本页保留较早版本的 Linux 测试，不作为当前版本的新成绩。

测试日期：2026-09-22。测试使用 OpenResty 维护的 Test::Nginx 框架，场景、
参数和结果均由本项目执行并发布，不属于 OpenResty 官方标准成绩、认证或背书。

## 环境

- Debian 12 x86_64，阿里云 KVM
- 2 vCPU（1 核 2 线程），701 MiB 内存，无 Swap
- OpenResty 官方 Debian 包 1.31.1.1
- Test::Nginx 0.32、weighttp 0.5、Valgrind
- 名单 151,253 条，二进制 605,076 字节
- 地址数组与 16 位前缀索引共 867,160 字节
- 吞吐测试使用 1 个 Nginx worker；PSS 补充测试使用 2 个 worker
- 压测客户端与 Nginx 位于同一台主机

## Test::Nginx Benchmark

使用 Test::Nginx Benchmark 模式做随机顺序 A/B：

- 基线组同样加载模块和名单，只不执行 `access()`。
- 保护组执行一次未命中的精确 IPv4 查询。
- 每轮每组预热 20,000 次，再请求 1,000,000 次，并发 50。
- 共 8 轮；测试块顺序随机；所有响应错误数均为 0。

| 轮次 | 基线 req/s | 保护组 req/s | 吞吐差异 |
|---:|---:|---:|---:|
| 1 | 160,948 | 150,057 | -6.77% |
| 2 | 162,091 | 150,873 | -6.92% |
| 3 | 169,406 | 151,047 | -10.84% |
| 4 | 164,834 | 150,784 | -8.52% |
| 5 | 162,506 | 151,517 | -6.76% |
| 6 | 161,175 | 151,419 | -6.05% |
| 7 | 164,085 | 149,993 | -8.59% |
| 8 | 162,773 | 150,971 | -7.25% |
| 中位数 | **162,640** | **150,922** | **-7.09%（配对中位数）** |

这个比例来自只有 `204` 空响应的极短路径，固定 Lua/access 成本会被明显放大；
同机压测还会让客户端与服务端争用同一个物理核心。因此它适合比较本项目的
A/B 差异，不能直接当作真实网站的吞吐下降比例。

## 功能与 Valgrind

- Test::Nginx 集成测试：11/11 通过，覆盖命中、未命中、access 放行和业务前拒绝。
- Valgrind Memcheck 严格模式：11/11 通过，启用
  `exit-on-first-error`，未发现本项目引起的非法内存访问或未抑制泄漏。
- `valgrind.suppress` 仅抑制 Nginx `ngx_set_environment` 在进程退出时固定的
  48 字节环境分配残留；该堆栈不经过本项目代码。

## 实际内存（PSS）

以下不是 Test::Nginx 官方基准项，而是项目补充的 Linux 进程内存观测。
2 个 worker 下读取 `/proc/<pid>/smaps_rollup`，PSS 为 master 与两个 worker 之和：

- 冷启动基线：6,001 KiB；加载模块和名单：7,340 KiB；增加 **1,339 KiB**。
- 保护组运行 200,000 次请求后：12,356 KiB。
- 累计运行 2,200,000 次请求后：12,374 KiB，比 200,000 次时增加 **18 KiB**；
  本轮没有观察到明显的持续增长。

冷启动增量只计算一次共享页，结果与 `init_by_lua*` 预加载后通过
pre-fork/COW 在 worker 间共享名单的设计一致。不要用 RSS 相加评估共享名单，
否则会重复计数。

## 复现

```sh
export TEST_NGINX_BINARY=/usr/local/openresty/nginx/sbin/nginx
export TEST_NGINX_IPBLOCK_LUA="$PWD/lua"
export TEST_NGINX_IPBLOCK_BIN="$PWD/examples/abuseipdb-30d/snapshot.bin"

prove -v t/001-integration.t

for round in $(seq 1 8); do
  TEST_NGINX_BENCHMARK='1000000 50' \
  TEST_NGINX_BENCHMARK_WARMUP=20000 \
  prove -v t/100-benchmark.t
done

TEST_NGINX_USE_VALGRIND=1 \
TEST_NGINX_VALGRIND_EXIT_ON_FIRST_ERR=1 \
prove -v t/001-integration.t
```

原始记录：

- [8 轮 Benchmark](../benchmark/results/test-nginx-official-8rounds.log)
- [严格 Valgrind](../benchmark/results/test-nginx-valgrind-strict.log)
