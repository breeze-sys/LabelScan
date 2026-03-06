package audit

import (
	"Label-Only-MIA-Go/pkg/core"
	"Label-Only-MIA-Go/pkg/mathutils"
	"sync"
)

// 定义信号状态
const (
	SignalRed    = "RED"
	SignalYellow = "YELLOW"
	SignalGreen  = "GREEN"
)

type AuditThresholds struct {
	Tau95  float64
	TauOpt float64
	TauD   float64
	TauCV  float64
}

type Engine struct {
	thresholds AuditThresholds
	shadow     core.Model
	target     core.Model
	attacker   core.Attacker
}

func NewEngine(t AuditThresholds, s, tg core.Model, atk core.Attacker) *Engine {
	return &Engine{thresholds: t, shadow: s, target: tg, attacker: atk}
}

func (e *Engine) AuditSample(sample core.Sample) core.AuditResult {
	res := core.AuditResult{
		SampleID:     sample.ID,
		Label:        sample.Label,
		IsMemberTrue: sample.IsMember,
	}

	// 1. 方案一信号：影子模型行为指纹
	logits, _ := e.shadow.PredictLogits(sample.Data)
	probs := mathutils.Softmax(logits)                        // 调用 B 的 Softmax
	loss := mathutils.CrossEntropy(probs, sample.TargetLabel) // 调用 B 需补全的函数
	res.ShadowLoss = loss

	s1 := SignalGreen
	if loss < e.thresholds.Tau95 {
		s1 = SignalRed
	} else if loss < e.thresholds.TauOpt {
		s1 = SignalYellow
	}

	// 2. 方案二信号：目标模型几何指纹
	// 调用你 (Member C) 实现的变体生成
	variants := mathutils.GenerateVariants(sample.Data, 0.001, 10)
	dists := e.probeAll(sample.Data, variants, sample.Label)

	// 调用 B 的统计函数
	dBar, std := mathutils.MeanAndStd(dists)
	res.MeanDistance = dBar
	cv := 0.0
	if dBar > 0 {
		cv = std / dBar
	}
	res.VolatilityCV = cv

	s2 := SignalGreen
	if dBar > e.thresholds.TauD && cv < e.thresholds.TauCV {
		s2 = SignalRed
	} else if dBar > e.thresholds.TauD || cv < e.thresholds.TauCV {
		s2 = SignalYellow
	}

	// 3. 最终逻辑判定
	res.Conclusion = e.fusionLogic(s1, s2)
	return res
}

func (e *Engine) probeAll(orig core.Image, variants [][]float32, label int) []float64 {
	all := append([][]float32{orig}, variants...)
	dists := make([]float64, len(all))

	var wg sync.WaitGroup
	for i, img := range all {
		wg.Add(1)
		go func(idx int, targetImg []float32) {
			defer wg.Done()
			tmp := core.Sample{Data: targetImg, Label: label}
			// 11个HSJA同时冲击 A 的服务器
			atkRes := e.attacker.Attack(tmp, e.target)
			dists[idx] = atkRes.Distance
		}(i, img)
	}
	wg.Wait() // 11个点全跑完，这个样本的审计才算结束
	return dists
}

func (e *Engine) fusionLogic(s1, s2 string) string {
	if s1 == SignalRed && s2 == SignalRed {
		return "🔴 【 确认为模型成员 】"
	}
	if s1 == SignalRed || s2 == SignalRed || (s1 == SignalYellow && s2 == SignalYellow) {
		return "🟡 【 风险极高 - 高度可疑 】"
	}
	if s1 == SignalYellow || s2 == SignalYellow {
		return "🟠 【 中度风险 - 疑似成员 】"
	}
	return "🟢 【 安全样本 - 非成员 】"
}
