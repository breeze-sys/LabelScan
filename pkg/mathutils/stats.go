package mathutils

import (
	"math"
)

// ============================================================================
// 统计辅助工具库 (Statistical Helpers)
// 对应 Python: numpy (argmax, mean), torch.nn.functional (softmax)
// ============================================================================

// CrossEntropy 计算交叉熵损失 (用于判定迁移攻击信号)
// probs: 经过 Softmax 后的概率切片
// target: 该样本的真实标签索引 (TargetLabel)
func CrossEntropy(probs []float32, target int) float64 {
	if target < 0 || target >= len(probs) {
		return 10.0 // 越界容错
	}
	// 提取目标类别的概率
	p := float64(probs[target])

	// 关键：数值稳定性处理
	// 如果概率极低接近 0，Log(0) 会导致负无穷，让程序崩溃
	if p < 1e-10 {
		p = 1e-10
	}

	// 公式: Loss = -ln(p)
	return -math.Log(p)
}

// ArgMax 找到切片中最大值的索引 (Index of Maximum Value)。
// 对应 Python: np.argmax(probs)
// 用途: 将模型输出的概率数组转换为具体的类别标签 (Label)。
// 例如: [0.1, 0.8, 0.1] -> 返回 1
func ArgMax(probs []float32) int {
	if len(probs) == 0 {
		return -1 // 或者 panic，视具体需求而定
	}

	maxIndex := 0
	maxValue := probs[0]

	for i, v := range probs {
		if v > maxValue {
			maxValue = v
			maxIndex = i
		}
	}
	return maxIndex
}

// MeanVector 计算一组向量的平均值向量。
// 对应 Python: np.mean(vectors, axis=0)
// 输入: 一个包含 n 个向量的切片 (Batch)
// 输出: 一个平均向量
// 用途: HopSkipJump 算法中，需要对多次随机扰动后的梯度估算值取平均，以消除噪声。
func MeanVector(vectors [][]float32) []float32 {
	if len(vectors) == 0 {
		return nil
	}

	rows := len(vectors)
	cols := len(vectors[0])

	// 使用 float64 累加器以防止精度丢失或溢出
	sum := make([]float64, cols)

	for _, vec := range vectors {
		if len(vec) != cols {
			panic("mathutils.MeanVector: 输入向量长度不一致")
		}
		for i, val := range vec {
			sum[i] += float64(val)
		}
	}

	// 计算平均值并转回 float32
	result := make([]float32, cols)
	div := float64(rows)
	for i := 0; i < cols; i++ {
		result[i] = float32(sum[i] / div)
	}

	return result
}

// Softmax 将 Logits (未归一化的分数) 转换为概率分布。
// 对应 Python: torch.nn.functional.softmax(dim=1)
// 公式: P_i = exp(x_i) / sum(exp(x_j))
// 优化: 实现了 "Numerical Stability" (减去最大值)，防止 exp 计算溢出。
func Softmax(logits []float32) []float32 {
	if len(logits) == 0 {
		return []float32{}
	}

	// 1. 找到最大值 (为了数值稳定性)
	maxLogit := logits[0]
	for _, v := range logits {
		if v > maxLogit {
			maxLogit = v
		}
	}

	// 2. 计算指数并求和
	exps := make([]float64, len(logits))
	var sumExps float64
	for i, v := range logits {
		// 减去 maxLogit 防止溢出 (e^1000 会无穷大，但 e^(1000-1000) = 1)
		exps[i] = math.Exp(float64(v - maxLogit))
		sumExps += exps[i]
	}

	// 3. 归一化
	probs := make([]float32, len(logits))
	for i, v := range exps {
		probs[i] = float32(v / sumExps)
	}

	return probs
}

// MeanAndStd 计算一组 float64 切片的均值 (Mean) 和样本标准差 (Sample Standard Deviation)
// 样本标准差使用 n-1 的自由度 (Bessel's correction)
func MeanAndStd(data []float64) (mean float64, std float64) {
	if len(data) == 0 {
		return 0, 0
	}

	n := float64(len(data))

	// 计算均值
	var sum float64
	for _, v := range data {
		sum += v
	}
	mean = sum / n

	// 元素过少时，标准差记为0以防除零
	if len(data) < 2 {
		return mean, 0.0
	}

	// 计算样本标准差
	var varianceSum float64
	for _, v := range data {
		diff := v - mean
		varianceSum += diff * diff
	}

	// 自由度为 n-1
	variance := varianceSum / (n - 1)
	std = math.Sqrt(variance)

	return mean, std
}

// CalibrateReference 计算路人集的统计基准，产出动态双阈值 (tau_d, tau_cv)
// 输入: dists 是一个二维切片，dists[i] 代表第 i 个路人图的 11 个测距值（1张原图 + 10个变体）
// 输出: tauD (距离阈值上限), tauCV (波动率/变异系数的阈值下限)
func CalibrateReference(dists [][]float64) (tauD float64, tauCV float64) {
	var dBarList []float64
	var cvList []float64

	// 第二步：计算每张路人图的特征值（样本内统计）
	for _, group := range dists {
		if len(group) == 0 {
			continue
		}

		m, s := MeanAndStd(group)

		// 记录出该路人图的平均距离
		dBarList = append(dBarList, m)

		// 计算并记录变异系数 CV = 标准差 / 均值
		if m != 0 {
			cvList = append(cvList, s/m)
		} else {
			cvList = append(cvList, 0.0) // 容错处理
		}
	}

	// 第三步：计算全局基准（参考集统计）
	muD, sD := MeanAndStd(dBarList)
	muCV, sCV := MeanAndStd(cvList)

	// 第四步：确定最终阈值 \tau （预测区间公式）
	// 基于 n=10, 95% 置信度的 t 分布常数为 1.92
	const tDistCoeff = 1.92

	// 距离阈值（上限判定）：如果目标样本 > tauD，且 cv < tauCV 则确认为成员
	tauD = muD + tDistCoeff*sD
	tauCV = muCV - tDistCoeff*sCV

	return tauD, tauCV
}
