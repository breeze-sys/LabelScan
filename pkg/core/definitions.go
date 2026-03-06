package core

// ==========================================
// 1. 全局配置与常量 (保持现状)
// ==========================================
const (
	ImgChannels   = 3
	ImgHeight     = 32
	ImgWidth      = 32
	FlattenedSize = ImgChannels * ImgHeight * ImgWidth
)

// ==========================================
// 2. 核心数据类型
// ==========================================

// Image 基础数据单元
type Image []float32

// Sample 代表一个测试样本
type Sample struct {
	ID          int    `json:"id"`
	Data        Image  `json:"image"`
	Label       int    `json:"label"`        // 原始分类标签
	TargetLabel int    `json:"target_label"` // 影子模型重标后的标签 (用于算迁移Loss)
	IsMember    bool   `json:"is_member"`    // 真实身份：是否为训练集成员 (用于对账)
	Filename    string `json:"filename"`
}

// AuditResult 存储多维审计报告 (这是你的核心输出)
type AuditResult struct {
	SampleID     int
	Label        int
	IsMemberTrue bool // 真相标签

	// 维度一：迁移攻击得分
	ShadowLoss float64

	// 维度二 & 三：边界探测得分
	MeanDistance float64 // d-bar
	VolatilityCV float64 // CV (标准差/均值)

	// 最终判决结论
	Conclusion string // "🔴 【 确认为模型成员 】" 等
}

// ==========================================
// 3. 接口契约 (A, B, C 三方的握手标准)
// ==========================================

// Model 接口：队友 A (API 实现者) 必须支持 Logits 返回
type Model interface {
	// Predict 返回最终标签 (Label-Only 基础功能)
	Predict(img Image) (int, error)

	// PredictBatch 批量预测 (HSJA 涡轮增压)
	PredictBatch(imgs []Image) ([]int, error)

	// PredictLogits 返回原始概率/分数数组 (为了在 Go 本地算 Loss)
	PredictLogits(img Image) ([]float32, error)

	GetInputSize() int
}

// Attacker 接口：定义探测内核的标准 (例如 HSJA)
// 注意：它返回的是探测过程的原始数据，交给 Engine 进行最终审计
type Attacker interface {
	Attack(sample Sample, model Model) AttackResult
}

// AttackResult 仅作为 Attacker (如 HSJA) 的原始输出
type AttackResult struct {
	SampleID int
	Distance float64 // 探测到的到边界的距离
	Queries  int     // 消耗的查询次数
}
