# test-annotation-reporter-go

测试结果归并与 CI 注解输出工具。把分散在多个分片、多个执行器上的测试结果
汇总成一份统一报告，并生成能在流水线平台上直接定位到代码位置的注解。

## 组成

- `reporter/`：核心库（解析、归并、报告、注解）。
- `cmd/testreport/`：命令行工具。

## 输入格式

通过 `reporter.Format` 或 CLI 的 `-format` / `路径:格式` 指定：

| 格式 | 标识 | 说明 |
| --- | --- | --- |
| `go test -json` | `gotest` | 逐行 JSON 事件流；从 `output` 事件中提取 `file.go:行号: 消息` 形式的源码位置；包级 `fail`（如编译错误）记为 `error` |
| JUnit XML | `junit` | 流式解析 `<testcase>`；优先使用 `file`/`line` 属性，缺失时从 failure/error 文本中提取 `file.go:行号:` |

用例状态归一化为 `passed` / `failed` / `skipped` / `error`。

### 非法记录处理

不支持的记录**不会被静默丢弃**，而是以 `RejectError` 形式产出，原因可区分：

- `unknown_format`：无法识别的格式或事件类型；
- `malformed`：记录残缺、JSON/XML 损坏；
- `missing_id`：缺少用例标识（如 JUnit `<testcase>` 无 `name`）。

单行损坏不影响后续记录；XML 结构整体损坏时剩余输入被明确拒绝并说明原因。

## 归并语义

- `Merger` 并发安全，多个分片的结果可以任意顺序、并行到达。
- 每条记录到达时分配单调递增序号；**同一用例 ID 重复出现时以最近一次到达
  （序号最大）的结果为准**。
- 因此归并结果与"按到达顺序串行摄入"完全一致，与分片间的交错顺序无关。
- `Snapshot()` 可在数据未全部到齐前随时调用，返回按用例 ID 排序的一致快照；
  内存占用只随**唯一用例数**增长，重复记录不会累积。
- 输入中断后重放同一批数据不会重复计数（按 ID 去重）。

## 注解输出

`Annotator{Platform, Max}` 把 `failed`/`error` 用例转换为平台注解：

| 平台 | 输出形式 | 转义规则 |
| --- | --- | --- |
| `github` | `::error file=…,line=…::消息` | 消息：`% \r \n`；属性额外转义 `: ,`，防止路径截断属性或注入指令 |
| `azure` | `##vso[task.logissue …]消息` | `%→%AZP25`、`\r \n`、`]→%5D`、`;→%3B` |
| `gitlab` | JSON Lines（每行一个对象） | 由 `encoding/json` 保证转义安全 |

聚合与上限：

- 同一 `(文件, 行号, 消息)` 组合只产出一条注解；
- 注解按优先级排序（`error` 优先，其后按位置与消息字典序），输出确定；
- 超过 `Max` 时按优先级保留，并追加一条聚合注解说明被省略的数量；
- `IncrementalAnnotator` 支持失败到达即输出，已产出的组合不会重复，
  输入中断后继续摄入也不会重复注解。

## 确定性

报告与注解均不含时间戳、按固定顺序输出；固定输入与固定随机种子下重复
运行逐字节一致，可直接与历史报告 diff。

## CLI 用法

```sh
testreport -format gotest -platform github -max 50 \
    shard1.json shard2.json junit-results.xml:junit
```

- 报告写到 stdout；注解写到 `-annotations` 指定的文件（默认 stderr）；
- 被拒绝的记录以 `REJECT` 前缀打印到 stderr；
- 存在 `failed`/`error` 用例时退出码为 1，参数或 IO 错误为 2。

## 库用法示例

```go
merger := reporter.NewMerger()
ch := make(chan reporter.Record, 256)
go func() {
    defer close(ch)
    reporter.ParseStream(reporter.FormatGoTestJSON, "shard-1", r, ch)
}()
merger.Ingest(ch, func(e *reporter.RejectError) { log.Println(e) })

rep := merger.Snapshot()
rep.WriteText(os.Stdout)
reporter.Annotator{Platform: reporter.PlatformGitHub, Max: 50}.
    WriteAnnotations(rep, os.Stderr)
```

## 测试

```sh
go test ./...
```

覆盖：多分片乱序归并与串行一致性、重复用例覆盖、非法/截断输入、
JUnit 位置提取、三平台注解转义、数量上限与聚合、增量注解去重、
确定性输出。
