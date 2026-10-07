package reporter

import (
	"sort"
	"sync"
)

// Merger 归并来自多个分片/执行器的结果，并发安全。
// 同一 ID 的用例重复出现时，以最近一次（序号最大）到达的结果为准，
// 因此归并结果与按到达顺序串行执行完全一致，与分片交错顺序无关。
type Merger struct {
	mu      sync.Mutex
	results map[string]TestResult
	seq     uint64
}

// NewMerger 创建一个归并器。
func NewMerger() *Merger {
	return &Merger{results: make(map[string]TestResult)}
}

// Add 摄入一条结果；同一 ID 后到的记录覆盖先到的。
func (m *Merger) Add(r TestResult) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.seq++
	r.Seq = m.seq
	m.results[r.ID] = r
}

// Ingest 从 ch 消费记录直到通道关闭；拒绝记录被计数并可通过 OnReject 回调观察。
// 输入中断后可用同一 Merger 继续 Ingest，不会重复计数（按 ID 去重）。
func (m *Merger) Ingest(ch <-chan Record, onReject func(*RejectError)) (rejected int) {
	for rec := range ch {
		if rec.Reject != nil {
			rejected++
			if onReject != nil {
				onReject(rec.Reject)
			}
			continue
		}
		if rec.Result != nil {
			m.Add(*rec.Result)
		}
	}
	return rejected
}

// Snapshot 返回当前已归并结果的一致快照，按 ID 排序，输出确定。
// 可在数据未全部到齐前随时调用，得到的是某一时刻的一致视图。
func (m *Merger) Snapshot() Report {
	m.mu.Lock()
	results := make([]TestResult, 0, len(m.results))
	for _, r := range m.results {
		results = append(results, r)
	}
	m.mu.Unlock()

	sort.Slice(results, func(i, j int) bool { return results[i].ID < results[j].ID })
	counts := map[Status]int{
		StatusPassed:  0,
		StatusFailed:  0,
		StatusSkipped: 0,
		StatusError:   0,
	}
	for _, r := range results {
		counts[r.Status]++
	}
	return Report{Results: results, Counts: counts}
}
