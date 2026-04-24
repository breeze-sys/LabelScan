package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	"Label-Only-MIA-Go/pkg/client"
	"Label-Only-MIA-Go/pkg/dataset"
	"Label-Only-MIA-Go/pkg/worker"
)

// 专门为 Python 训练定义的导出格式
type ShadowExportRecord struct {
	Image []float32 `json:"image"`
	Label int       `json:"target_label"`
}

func main() {
	fmt.Println("🚀 --- LabelScan-Go: 影子模型数据生产线启动 ---")

	// 1. 初始化模型客户端 (对接 8000 端口的目标模型)
	targetURL := "http://localhost:8000"
	targetModel := client.NewHTTPClient(targetURL)

	// 2. 加载二进制原始数据
	// 注意：我们要用 data_batch_1.bin 里的 10000 张图来作为影子模型的训练集
	dataPath := "data/cifar-10-batches-bin/data_batch_1.bin"
	loader := &dataset.CifarLoader{IsMemberSet: true}

	fmt.Printf("📂 正在读取二进制文件: %s\n", dataPath)
	samples, err := loader.LoadBatch(dataPath, 10000)
	if err != nil {
		log.Fatalf("❌ 加载失败: %v", err)
	}

	// 3. 调用你写好的 Relabeler 进行“涡轮增压”重标
	fmt.Println("🌀 正在启动重标引擎 (Relabeling)...")
	start := time.Now()

	// 创建你的 Relabeler 实例，设置批次大小为 128
	relabeler := worker.NewRelabeler(targetModel, 128)

	// 执行你写好的核心函数
	relabeler.RelabelAll(samples)

	fmt.Printf("✅ 重标完成！总耗时: %v\n", time.Since(start))

	// 4. 将重标后的数据转化为 JSON 格式
	fmt.Println("💾 正在将数据序列化为 shadow_train_data.json...")

	exportData := make([]ShadowExportRecord, len(samples))
	for i, s := range samples {
		exportData[i] = ShadowExportRecord{
			Image: s.Data,
			Label: s.TargetLabel,
		}
	}

	// 5. 写入文件
	file, err := os.Create("shadow_train_data.json")
	if err != nil {
		log.Fatalf("❌ 创建文件失败: %v", err)
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	// 设置不转义 HTML 字符，保证浮点数显示正常
	encoder.SetEscapeHTML(false)

	if err := encoder.Encode(exportData); err != nil {
		log.Fatalf("❌ 写入 JSON 失败: %v", err)
	}

	fmt.Println("\n=====================================================")
	fmt.Println("🎉 成功！影子模型训练集已产出。")
	fmt.Printf("📍 文件位置: ./shadow_train_data.json\n")
	fmt.Printf("📦 数据规模: %d 张图片\n", len(exportData))
	fmt.Println("👉 下一步: 将此文件交给 Member A，通过 Python 脚本进行训练。")
	fmt.Println("=====================================================")
}
