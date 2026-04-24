package main

import (
	"Label-Only-MIA-Go/pkg/client"
	"Label-Only-MIA-Go/pkg/core"
	"Label-Only-MIA-Go/pkg/dataset"
	"fmt"
	"log"
)

func main() {
	fmt.Println("🧪 --- [最终版] 批量对账与模型识别校验 ---")

	// 1. 初始化客户端 (连接刚才启动的 8000 端口目标模型)
	// 注意：PredictBatch 会自动拼接成 http://localhost:8000/predict_batch
	apiURL := "http://localhost:8000/predict"
	httpClient := client.NewHTTPClient(apiURL)

	// 2. 加载数据
	// 【注意】：请确保这里的路径是你刚下载解压后的 .bin 文件！
	dataPath := "data/cifar-10-batches-bin/test_batch.bin"
	loader := &dataset.CifarLoader{}

	fmt.Printf("📂 正在尝试读取文件: %s\n", dataPath)
	samples, err := loader.LoadBatch(dataPath, 3) // 只取3张
	if err != nil {
		log.Fatalf("❌ 读取失败，请检查路径是否正确: %v", err)
	}

	// 3. 打印指纹用于肉眼对比
	fmt.Printf("[Go端] 样本 #0 原始标签: %d\n", samples[0].Label)
	fmt.Printf("[Go端] 样本 #0 像素指纹: %v...\n", samples[0].Data[:5])

	// 4. 发起批量预测请求
	imgs := []core.Image{samples[0].Data, samples[1].Data, samples[2].Data}
	fmt.Println("[Go端] 🚀 正在请求 Python Server (8000端口)...")

	labels, err := httpClient.PredictBatch(imgs)
	if err != nil {
		log.Fatalf("❌ 网络请求失败: %v", err)
	}

	// 5. 核心判定：看预测结果和原始标签是否一致
	fmt.Println("\n--- 📝 最终对账单 ---")
	matchCount := 0
	for i, pred := range labels {
		status := "❌ 错位"
		if pred == samples[i].Label {
			status = "✅ 匹配"
			matchCount++
		}
		fmt.Printf("样本 #%d | 原始标签: %d | 模型预测: %d | 结果: %s\n",
			i, samples[i].Label, pred, status)
	}

	if matchCount > 0 {
		fmt.Println("\n🎉 恭喜！数据对齐成功，模型可以正常识别二进制数据了！")
	} else {
		fmt.Println("\n😱 警告：预测全部错误！说明数据解析还是乱码，或者归一化没做。")
	}
}
