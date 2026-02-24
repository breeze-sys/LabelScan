package worker

import (
	"Label-Only-MIA-Go/pkg/core"
	"fmt"
	"sync"
)

type Auditor struct {
	Model       core.Model
	Attacker    core.Attacker
	WorkerCount int
}

func NewAuditor(m core.Model, a core.Attacker, count int) *Auditor {
	return &Auditor{Model: m, Attacker: a, WorkerCount: count}
}

// RunAudit 让 20 个工人同时跑复杂的 Attack 函数
func (a *Auditor) RunAudit(samples []core.Sample) []core.AttackResult {
	var wg sync.WaitGroup
	jobs := make(chan core.Sample, len(samples))
	resultsChan := make(chan core.AttackResult, len(samples))

	fmt.Printf("🚀 审计工厂启动：并发工人数 %d\n", a.WorkerCount)

	for i := 0; i < a.WorkerCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for s := range jobs {
				// 运行队长写的攻击逻辑，并将 Model 借给他用
				res := a.Attacker.Attack(s,a.Model)
				resultsChan <- res
			}
		}()
	}

	for _, s := range samples {
		jobs <- s
	}
	close(jobs)

	// 异步关闭回收通道
	go func() {
		wg.Wait()
		close(resultsChan)
	}()

	var finalResults []core.AttackResult
	for r := range resultsChan {
		finalResults = append(finalResults, r)
	}
	return finalResults
}
