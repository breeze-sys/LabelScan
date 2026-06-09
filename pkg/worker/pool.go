package worker

import (
	"context"

	"Label-Only-MIA-Go/pkg/audit"
	"Label-Only-MIA-Go/pkg/core"
	"fmt"
	"sync"
)

// AuditPool 审计工厂，负责大规模并发调度
type AuditPool struct {
	Engine      *audit.Engine // 注入我们的判决大脑
	WorkerCount int
}

func NewAuditPool(e *audit.Engine, count int) *AuditPool {
	return &AuditPool{
		Engine:      e,
		WorkerCount: count,
	}
}

// RunAudit 并发运行三段式审计逻辑
// 输入: 1000 个待审计样本
// 输出: 1000 个带红绿灯结论的审计报告
func (p *AuditPool) RunAudit(samples []core.Sample) []core.AuditResult {
	return p.RunAuditContext(context.Background(), samples)
}

func (p *AuditPool) RunAuditContext(ctx context.Context, samples []core.Sample) []core.AuditResult {
	var wg sync.WaitGroup

	// 1. 创建任务通道和结果通道
	// 使用 AuditResult 而不是 AttackResult
	jobs := make(chan core.Sample, len(samples))
	resultsChan := make(chan core.AuditResult, len(samples))

	fmt.Printf("🚀 审计工厂启动：并发工人数 %d，待审计样本数 %d\n", p.WorkerCount, len(samples))

	// 2. 启动工人 (Workers)
	for i := 0; i < p.WorkerCount; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for s := range jobs {
				if ctx.Err() != nil {
					return
				}
				// 核心变动：调用我们的 engine 执行全流程审计（Loss + 距离 + 判决）
				res := p.Engine.AuditSampleContext(ctx, s)

				// 打印一下实时进度，方便总指挥监控
				if s.ID%50 == 0 {
					fmt.Printf("[Worker %d] 正在处理样本 #%d...\n", workerID, s.ID)
				}

				select {
				case resultsChan <- res:
				case <-ctx.Done():
					return
				}
			}
		}(i)
	}

	// 3. 塞入任务数据
queueLoop:
	for _, s := range samples {
		select {
		case jobs <- s:
		case <-ctx.Done():
			break queueLoop
		}
	}
	close(jobs) // 塞完任务记得关门，否则工人会一直死等

	// 4. 异步收集结果，防止主线程阻塞
	go func() {
		wg.Wait()
		close(resultsChan)
	}()

	// 5. 汇总最终战报
	var finalResults []core.AuditResult
	for r := range resultsChan {
		finalResults = append(finalResults, r)
	}

	fmt.Printf("✅ 审计任务全部完成，共计产出 %d 份报告。\n", len(finalResults))
	return finalResults
}
