package main

// --- 通信协议 ---

// 发送给 Python
type PredictRequest struct {
	Image []float32 `json:"image"`
}

// 接收 Python
type PredictResponse struct {
	Label  int       `json:"label"`
	Logits []float32 `json:"logits"`
}

// --- 内部数据结构 ---

// 代表一张样本图
type Sample struct {
	ID       int
	Image    []float32 // 长度 3072
	Label    int       // 真实标签 GroundTruth
	IsMember bool      // true=训练集, false=测试集
}

// 攻击结果
type AttackResult struct {
	SampleID int
	Distance float64 // L2 距离
	IsMember bool    // 真实身份
}