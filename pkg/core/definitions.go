package core

// ==========================================
// 1. 全局配置与常量
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
	TargetLabel int    `json:"target_label"` // 影子模型重标后的标签
	IsMember    bool   `json:"is_member"`    // 真实身份
	Filename    string `json:"filename"`
}

// AuditResult 存储多维审计报告 (给 main.go 做统计用的)
type AuditResult struct {
	SampleID     int
	Label        int
	IsMemberTrue bool

	ShadowLoss   float64
	MeanDistance float64
	VolatilityCV float64
	Conclusion   string
}

// AttackResult 仅作为 Attacker (如 HSJA) 的原始输出
// ⚠️【修复点】：补全了 HSJA 算法需要的所有字段
type AttackResult struct {
	SampleID      int
	OriginalLabel int     // 原始预测标签 (Before)
	FinalLabel    int     // 最终预测标签 (After)
	IsSuccess     bool    // 攻击是否成功
	Queries       int     // 消耗的查询次数
	Distance      float64 // 探测到的到边界的距离
	IsMember      bool    // (可选) 标记真实身份
}

// ==========================================
// 3. 接口契约
// ==========================================

// Model 接口
type Model interface {
	// Predict 返回最终标签 (Label-Only 基础功能)
	Predict(img Image) (int, error)

	// PredictBatch 批量预测 (HSJA 涡轮增压)
	PredictBatch(imgs []Image) ([]int, error)

	// PredictLogits 返回原始概率/分数数组 (为了在 Go 本地算 Loss)
	PredictLogits(img Image) ([]float32, error)

	GetInputSize() int
}

// Attacker 接口
type Attacker interface {
	Attack(sample Sample, model Model) AttackResult
}
