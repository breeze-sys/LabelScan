package mathutils

import (
	"fmt"
	"math"
	"testing"
)

// TestL2DistancePythonMatched 验证 Go 语言的 L2 计算和 Python np.linalg.norm 完全一致 (Task 13)
func TestL2DistancePythonMatched(t *testing.T) {
	fmt.Println("=== 测试 L2Distance (Python np.linalg.norm 对账) ===")
	// 用 Python 的 np.linalg.norm 计算出的一组对照数据:
	// a = np.array([0.1, 0.5, 0.9])
	// b = np.array([0.2, 0.4, 0.6])
	// diff = [-0.1, 0.1, 0.3]
	// dist = sqrt(0.01 + 0.01 + 0.09) = sqrt(0.11) ≈ 0.33166247903554

	a := []float32{0.1, 0.5, 0.9}
	b := []float32{0.2, 0.4, 0.6}

	// 此时 basic.go -> distance.go 中有 L2Distance
	gotDist := L2Distance(a, b)
	wantDist := 0.33166247903554

	fmt.Printf("  Go L2 计算结果: %.14f\n", gotDist)
	fmt.Printf("  Python 对照结果: %.14f\n", wantDist)

	// 题目要求：小数点后四位必须绝对一致
	if math.Abs(gotDist-wantDist) > 1e-4 {
		t.Errorf("L2Distance 对账失败! = %v, 期望同 Python 保持一致 = %v", gotDist, wantDist)
	} else {
		fmt.Println("  ✅ 成功: Go 侧 L2 距离算法和 Python numpy 小数点后四位完美匹配！")
	}
}
