package main

import (
	"Label-Only-MIA-Go/pkg/client"
	"Label-Only-MIA-Go/pkg/mathutils"
	"encoding/json"
	"fmt"
	"log"
	"os"
)

// 对应你导出的 JSON 结构
type ShadowData struct {
	Image []float32 `json:"image"`
	Label int       `json:"target_label"`
}

func main() {
	fmt.Println("🔍 --- 影子模型权重 (.pth) 真实性核查 ---")

	// 1. 加载你发给 Member A 的那份“标准教材”
	jsonFile, err := os.Open("shadow_train_data.json")
	if err != nil {
		log.Fatalf("❌ 找不到训练数据 JSON，请确保该文件在根目录: %v", err)
	}
	defer jsonFile.Close()

	// 为了速度，我们只解析前几个样本
	decoder := json.NewDecoder(jsonFile)
	// 跳过数组开头的 [
	_, _ = decoder.Token()

	fmt.Println("[*] 正在从教材中抽取样本进行测试...")

	// 2. 初始化连接 (确保 8001 端口已启动并加载了那个新 pth)
	shadowClient := client.NewHTTPClient("http://localhost:8001")

	count := 0
	for decoder.More() && count < 5 {
		var s ShadowData
		_ = decoder.Decode(&s)

		// 3. 获取影子模型的反应
		logits, err := shadowClient.PredictLogits(s.Image)
		if err != nil {
			log.Fatalf("❌ 无法连接影子服务器: %v", err)
		}

		// 计算概率分布和 Loss
		probs := mathutils.Softmax(logits)
		shadowPred := mathutils.ArgMax(probs)
		loss := mathutils.CrossEntropy(probs, s.Label)

		fmt.Printf("\n--- 测试样本 #%d ---\n", count)
		fmt.Printf("教材要求标签 (Target API给的): %d\n", s.Label)
		fmt.Printf("影子模型实际预测结果:         %d\n", shadowPred)
		fmt.Printf("影子模型对目标标签的预测概率: %.8f\n", probs[s.Label])
		fmt.Printf("计算出的真实 ShadowLoss:     %.4f\n", loss)

		if shadowPred == s.Label && loss < 0.5 {
			fmt.Println("✅ [结论]: 该样本拟合完美！")
		} else {
			fmt.Println("❌ [结论]: 拟合失败！影子模型根本不认这个标签。")
		}
		count++
	}

	fmt.Println("\n---------------------------------------")
	fmt.Println("📢 结果分析指导：")
	fmt.Println("1. 如果预测概率 > 0.5 且 Loss < 0.7: 说明权重是对的，可以开火。")
	fmt.Println("2. 如果预测概率极低（如 0.000001）: 100% 是标签索引偏移或模型练废了。")
}
