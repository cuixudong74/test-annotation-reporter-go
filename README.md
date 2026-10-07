# test-annotation-reporter-go

把分散在多个分片 / 执行器上的测试结果归并成一份统一报告，并生成可直接
定位到代码行的 CI 平台注解（GitHub Actions / GitLab / Azure DevOps）。

## 构建与使用

```sh
go build ./cmd/tar
tar -format jsonl -platform github [-max-annotations 50] [result-files...]
```

- 不带文件参数时从 stdin 读取（分片 id 可用 `-shard` 指定，默认 `stdin`）。
- 每个输入文件视为一个分片，分片 id 即文件路径。
- 报告与注解输出到 stdout；存在失败/错误用例时退出码为 1。
- 输出在固定输入下逐字节一致，可直接与历史报告 diff。

## 输入格式（`-format`）

### `jsonl` — 原生行式 JSON（每行一条记录）

```json
{"suite":"pkg","name":"TestX","status":"failed","file":"pkg/x.go","line":42,"message":"boom","seq":3}
```

- `status`: `passed` | `failed` | `skipped` | `error`（亦接受 `pass`/`fail`/`skip`）。
- `file`/`line`/`message` 可选；`seq` 可选，缺省时用行号，用于断点续传去重。

### `junit` — JUnit XML

解析 `<testcase>` 的 `name`/`classname`/`file`/`line` 属性与
`<failure>`/`<error>`/`<skipped>` 子元素。流式解码：损坏点之前的用例保留，
损坏的尾部以 `malformed` 原因拒绝。

### `gotest` — `go test -json` 事件流

从 `output` 事件中提取 `file.go:行号: 消息` 位置信息；流中断时未结束的
用例被标记为 `error` 而不是静默丢失。

### 非法记录的处理

不支持的记录**绝不静默丢弃**，按类型拒绝并计数（报告中的 `rejected=`）：

| 原因 | 含义 |
|---|---|
| `malformed` | 记录无法解析（坏 JSON、坏 XML、截断行） |
| `unknown-format` | 未知的状态值 / 事件类型 / 格式名 |
| `missing-field` | 缺少必填字段（如用例名） |
| `duplicate` | 分片内序号回退，判定为重复摄入 |

## 归并语义

- 同一用例（`suite`+`name`）在多个分片出现时，**最后被接受的记录生效**，
  与串行执行"最后一次执行为准"一致。
- 每个分片维护水位线：序号不大于已见序号的记录判为 `duplicate` 拒绝，
  因此中断后重新摄入同一文件不会重复计数。
- 快照按 `(suite, name)` 排序，摄入进行中也可取到一致快照。
- 内存上界 = 不同用例数 + 分片数，与输入记录总数无关。

## 注解平台（`-platform`）

| 平台 | 输出 | 转义规则 |
|---|---|---|
| `github` | `::error file=…,line=…::消息` | 数据 `% \r \n`，属性另转义 `: ,` |
| `azure` | `##vso[task.logissue …]消息` | 数据 `% \r \n`，属性另转义 `; ]` |
| `gitlab` | Code Quality JSON（每行一条） | JSON 编码本身保证转义 |

所有平台都会转义换行，消息中的特殊字符既不能截断注解，也无法注入
额外的平台指令。

## 注解上限与聚合（`-max-annotations`，默认 50）

1. 先按 `(文件, 行号, 消息)` 去重：同一位置同一原因只报一次。
2. 按优先级排序（error 优先，再按文件/行号/消息），保证截断确定性。
3. 超出上限时保留前 N 条，追加一条 warning 汇总注解，说明被省略的数量。

## 测试

```sh
go test ./...
```

覆盖：多分片乱序归并与串行一致性、重复用例与断点续传、非法输入分类、
三平台注解转义、注解上限与去重、端到端逐字节确定性。
