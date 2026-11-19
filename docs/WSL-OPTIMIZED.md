# Optimized module on WSL · [简体中文](WSL-OPTIMIZED.zh-CN.md)

Local WSL Debian 12, one worker, the same HTTP 204 upstream and 32 persistent connections. Of the six blocking implementations tested, native nftables is omitted because its set memory could not be isolated. Stock OpenResty and APISIX controls load no list; the paired ipset control keeps the set resident and toggles only the rules. [Inputs and environment](WSL-COMPARISON.md)

Each cell reads **allowed throughput · total memory · installed size (blocking component + IP list; excludes OpenResty)**.

[See the three-size comparison in the README](../README.en.md#performance-comparison).

**Total memory** = warm PSS of the front-end master and children plus the separate kernel blocklist, if any. ipset reports its own `Size in memory`; in-process lists are already in PSS. This is not whole-VM memory: the common upstream, load generator and general kernel overhead are excluded. Native nftables is omitted because VM-wide counter deltas cannot reliably isolate its set memory.

**Installed size** counts the component and loadable list or rules only. OpenResty, the APISIX host and the offline builder are excluded. Built-in geo counts rules only; ipset counts the tools, libraries and saved rules; the APISIX plugin counts plugin files and its list-bearing route. A control’s 0 B means no blocking component or list, not an empty host.

Throughput measures HTTP 204 responses for one fixed unlisted client, not rejections. Except for ², rates are medians of three 3-second samples. ¹ denotes repeat runs after unstable samples (150k APISIX control; 400k project and geo). See the raw records for P99, 403 hit rates and first-pass values; [default 444 closes](WSL-COMPARISON.md#before-and-after-the-storage-change) were tested separately.

**² iptables + ipset: separate paired retest.** The old run read DROP counters before the timed samples, not across them. The retest kept the proxy running in an isolated network namespace, toggling rules in four AB / BA / AB / BA pairs per size. Enabled rules do one source-set lookup on port 18080: ACCEPT nonmembers, otherwise DROP. Source-bound probes verified both paths before each 1-second warm-up and 5-second sample. ACCEPT packet counts increased and DROP counts were unchanged in every enabled window. The initial 150k rule-off samples spanned over 15×, so this size was repeated in full and uses the repeat. Rates come from completed wrk requests, not packets. Memory is from the original comparison.

The rule-off run is a control for the effect of enabling the lookup on allowed throughput, not a separate blocking engine. Matched rule-off medians for ² (150k / 400k / 600k): 155,273 req/s / 157,995 req/s / 157,948 req/s. Median paired percentage changes (min … max): +0.7% (-2.1% … +2.6%) / +0.2% (-1.9% … +1.3%) / +0.4% (-1.2% … +0.5%). Do not subtract ² from the first row of the older campaign or interpret allowed requests/s as DROP capacity.

## Artifacts

Module SHA-256 `617e818eb7056454553a9e401a0f9cbfac7459c39bedb1c6d6a1a85b25fbd414`。 [First pass](../benchmark/results/wsl-optimized-samples.jsonl) · [Repeat](../benchmark/results/wsl-optimized-repeat.jsonl) · [Footprint readings](../benchmark/results/wsl-optimized-footprint.json) · [Benchmark runner](../benchmark/wsl_compare.py) · [Footprint runner](../benchmark/wsl_footprint.py) · [Paired ipset samples](../benchmark/results/wsl-ipset-paired.jsonl) · [150k repeat](../benchmark/results/wsl-ipset-paired-repeat.jsonl) · [Paired runner](../benchmark/wsl_ipset_paired.py)
