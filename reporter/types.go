// Package reporter 提供测试结果归并与 CI 注解输出能力。
package reporter

// Status 表示单个测试用例的执行结果。
type Status int

const (
	StatusPassed Status = iota
	StatusFailed
	StatusSkipped
	StatusError
)

func (s Status) String() string {
	switch s {
	case StatusPassed:
		return "passed"
	case StatusFailed:
		return "failed"
	case StatusSkipped:
		return "skipped"
	case StatusError:
		return "error"
	}
	return "unknown"
}

// TestResult 是一条归一化后的测试结果记录。
type TestResult struct {
	ID      string // 用例唯一标识（同一用例跨分片重复时按 ID 去重）
	Name    string
	Status  Status
	File    string // 失败对应的源码文件（可为空）
	Line    int    // 失败对应的源码行号（0 表示未知）
	Message string // 失败/错误信息
	Shard   string // 来源分片或执行器标识
	Seq     uint64 // 全局到达序号，用于"最近一次执行为准"的判定
}

// RejectReason 区分记录被拒绝的原因。
type RejectReason int

const (
	RejectUnknownFormat RejectReason = iota // 无法识别的记录格式
	RejectMalformed                         // 记录残缺或字段非法
	RejectMissingID                         // 缺少用例标识
)

func (r RejectReason) String() string {
	switch r {
	case RejectUnknownFormat:
		return "unknown_format"
	case RejectMalformed:
		return "malformed"
	case RejectMissingID:
		return "missing_id"
	}
	return "unknown"
}

// RejectError 表示一条输入记录被明确拒绝。
type RejectError struct {
	Reason RejectReason
	Detail string
}

func (e *RejectError) Error() string {
	return "rejected (" + e.Reason.String() + "): " + e.Detail
}
