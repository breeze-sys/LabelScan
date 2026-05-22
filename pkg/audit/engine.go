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

// pkg/audit/engine.go

type AuditThresholds struct {
	// 将 tau_95 改为 threshold
	Tau95 float64 `json:"threshold"`

	// 将 tau_opt 改为 mean_member_loss
	TauOpt float64 `json:"mean_member_loss"`

	// 这两个值是 main.go 现场算的，JSON 里没有也没关系
	TauD  float64 `json:"tau_d"`
	TauCV float64 `json:"tau_cv"`
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

	// ---------------------------------------------------------
	// 1. 预校验：探测目标模型对原图的初始反应
	// ---------------------------------------------------------
	tmpOrig := core.Sample{Data: sample.Data, Label: sample.Label}
	origAtk := e.attacker.Attack(tmpOrig, e.target)

	// 如果模型连原图都预测错了，直接判定为安全，省去后续所有计算
	if origAtk.Distance < 1e-6 {
		res.ShadowLoss = 23.0
		res.MeanDistance = 0
		res.VolatilityCV = 99.0
		res.Conclusion = "🟢 【 安全样本 - 非成员 (模型预测错误) 】"
		return res
	}

	// ---------------------------------------------------------
	// 2. 方案一信号：行为指纹 (影子模型判定)
	// ---------------------------------------------------------
	// 获取目标模型当前的真实预测，作为影子模型对比的基准
	targetPred, _ := e.target.Predict(sample.Data)
	sample.TargetLabel = targetPred

	logits, _ := e.shadow.PredictLogits(sample.Data)
	probs := mathutils.Softmax(logits)
	loss := mathutils.CrossEntropy(probs, sample.Label)
	res.ShadowLoss = loss

	// 【核心逻辑修正】：自动识别红灯和黄灯的水位
	// 影子模型 Loss 越小越危险。所以 Tau95 和 TauOpt 中更小的那个才是真正的“红灯线”
	redLine := e.thresholds.Tau95
	yellowLine := e.thresholds.TauOpt
	if redLine > yellowLine {
		redLine, yellowLine = yellowLine, redLine // 交换，确保 redLine 是最小最严的
	}

	s1 := SignalGreen
	if loss < redLine {
		s1 = SignalRed // 极度像成员
	} else if loss < yellowLine {
		s1 = SignalYellow // 比较像成员
	}

	// ---------------------------------------------------------
	// 3. 方案二信号：几何指纹 (边界稳定性判定)
	// ---------------------------------------------------------
	variants := mathutils.GenerateVariants(sample.Data, 0.001, 10)
	otherDists := e.probeAll(variants, sample.Label)
	allDists := append([]float64{origAtk.Distance}, otherDists...)

	dBar, std := mathutils.MeanAndStd(allDists)
	res.MeanDistance = dBar
	cv := 99.0 // 默认不平稳
	if dBar > 0 {
		cv = std / dBar
	}
	res.VolatilityCV = cv

	// 1. AuditSample 函数的结尾：
	s2 := SignalGreen
	isFar := dBar > e.thresholds.TauD // 距离比路人远

	// 【暴力拆解 AND 死亡之锁】：只要远，就是实锤成员！
	if isFar {
		s2 = SignalRed
	} else if cv < e.thresholds.TauCV {
		s2 = SignalYellow
	}

	// ---------------------------------------------------------
	// 4. 决策融合
	// ---------------------------------------------------------
	res.Conclusion = e.fusionLogic(s1, s2)
	return res
}

// 辅助函数微调：去掉原图合并，只跑变体
func (e *Engine) probeAll(variants [][]float32, label int) []float64 {
	dists := make([]float64, len(variants))
	var wg sync.WaitGroup
	for i, img := range variants {
		wg.Add(1)
		go func(idx int, targetImg []float32) {
			defer wg.Done()
			tmp := core.Sample{Data: targetImg, Label: label}
			atkRes := e.attacker.Attack(tmp, e.target)
			dists[idx] = atkRes.Distance
		}(i, img)
	}
	wg.Wait()
	return dists
}

func (e *Engine) fusionLogic(s1, s2 string) string {
	if s2 == SignalRed && s1 == SignalRed {
		return "🔴 【 确认为模型成员 - 证据闭环绝对实锤 】"
	}
	if s2 == SignalRed && s1 == SignalYellow {
		return "🔴 【 确认为模型成员 - 边界突破 & 行为特征吻合 】"
	}
	if s1 == SignalRed && s2 == SignalYellow {
		return "🔴 【 确认为模型成员 - 行为实锤 & 局部特征稳定 】"
	}
	// 核心降级：距离虽远，但真实Loss极大(S1没见过)，降级为橙色
	if s2 == SignalRed && s1 == SignalGreen {
		return "🟠 【 中度风险 - 异常距离(目标模型神经质)，但行为不符 】"
	}
	if s1 == SignalRed || s2 == SignalYellow || s1 == SignalYellow {
		return "🟡 【 风险较高 - 单一维度疑似 】"
	}
	return "🟢 【 安全样本 - 非成员 】"
}
