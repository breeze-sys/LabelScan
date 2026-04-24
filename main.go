package main

import (
	"Label-Only-MIA-Go/pkg/attack"
	"Label-Only-MIA-Go/pkg/audit"
	"Label-Only-MIA-Go/pkg/client"
	"Label-Only-MIA-Go/pkg/core"
	"Label-Only-MIA-Go/pkg/dataset"
	"Label-Only-MIA-Go/pkg/mathutils"
	"Label-Only-MIA-Go/pkg/worker"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"strings"
)

func main() {
	fmt.Println("=====================================================")
	fmt.Println("🛡️  LabelScan-Go: 高性能黑盒模型隐私审计工具 (诊断版)")
	fmt.Println("=====================================================")

	// ---------------------------------------------------------
	// 1. 资产加载 (从 JSON 读取迁移攻击阈值)
	// ---------------------------------------------------------
	configData, err := ioutil.ReadFile("shadow_config.json")
	if err != nil {
		log.Fatal("❌ 错误：缺少 shadow_config.json")
	}
	var thresholds audit.AuditThresholds
	if err := json.Unmarshal(configData, &thresholds); err != nil {
		log.Fatalf("❌ 配置解析失败: %v", err)
	}

	// ---------------------------------------------------------
	// 2. 环境初始化
	// ---------------------------------------------------------
	targetAPI := "http://localhost:8000" // 确保路径包含 /predict
	shadowAPI := "http://localhost:8001"

	targetModel := client.NewHTTPClient(targetAPI)
	shadowModel := client.NewHTTPClient(shadowAPI)

	hsja := attack.NewHSJA(attack.HSJAConfig{
		MaxQueries:    5000,
		MaxIterations: 40,
		NumEvals:      100,
	})

	// ---------------------------------------------------------
	// 3. 现场定标 (Calibration)：核心修复逻辑
	// ---------------------------------------------------------
	fmt.Println("\n🔍 阶段一：正在进行现场定标 (寻找 10 个有效路人)...")
	loader := &dataset.CifarLoader{}

	// 【关键修改 1】：加载 100 张备选路人图，防止 10 张不够挑导致的死循环
	candidates, _ := loader.GetRandomStrangers("data/cifar-10-batches-bin/test_batch.bin", 100)

	var refDists [][]float64
	validStrangers := 0

	for i := 0; i < len(candidates) && validStrangers < 10; i++ {
		s := candidates[i]

		// 预探测：模型必须能认对这张图 (Dist > 0)
		tmpOrig := core.Sample{Data: s.Data, Label: s.Label}
		resOrig := hsja.Attack(tmpOrig, targetModel)

		if resOrig.Distance < 1e-5 {
			fmt.Printf("   [跳过] 路人 #%d 预测错误 (Dist=0)，尝试下一个...\n", i+1)
			continue
		}

		fmt.Printf("   [定标中] 正在探测有效路人 %d/10 的地理特征...\n", validStrangers+1)

		// 生成变体并测距
		variants := mathutils.GenerateVariants(s.Data, 0.001, 10)
		points := append([][]float32{s.Data}, variants...)
		var groupDists []float64
		for _, img := range points {
			tmp := core.Sample{Data: img, Label: s.Label}
			res := hsja.Attack(tmp, targetModel)
			groupDists = append(groupDists, res.Distance)
		}
		refDists = append(refDists, groupDists)
		validStrangers++
	}

	if validStrangers < 5 {
		log.Fatal("❌ 严重错误：无法找到足够的有效路人样本，请检查模型准确率或数据对齐！")
	}

	// 调用统计函数算出 TauD 和 TauCV
	thresholds.TauD, thresholds.TauCV = mathutils.CalibrateReference(refDists)

	// 【关键修改 2】：强制诊断打印
	fmt.Println("\n--- 🕵️ 阈值诊断报告 (核心排查数据) ---")
	fmt.Printf("👉 迁移红灯 (Tau95): %.4f (来自JSON)\n", thresholds.Tau95)
	fmt.Printf("👉 迁移黄灯 (TauOpt): %.4f (来自JSON)\n", thresholds.TauOpt)
	fmt.Printf("👉 距离红线 (TauD):   %.4f (正常应在 0.3-0.7)\n", thresholds.TauD)
	fmt.Printf("👉 波动绿线 (TauCV):  %.4f (正常应在 0.01-0.1)\n", thresholds.TauCV)
	if thresholds.TauD > 1.5 {
		fmt.Println("⚠️  警告：TauD 过高，会导致严重漏报 (Recall低)！")
	}
	fmt.Println("-------------------------------------------\n")

	// ---------------------------------------------------------
	// 4. 构造混合测试包 (各拿 5 个，总计 10 个样本做快速诊断)
	// ---------------------------------------------------------
	fmt.Println("📦 阶段二：构造混合测试包 (5 成员 + 5 路人)...")
	loaderM := &dataset.CifarLoader{IsMemberSet: true}
	members, _ := loaderM.LoadBatch("data/cifar-10-batches-bin/data_batch_1.bin", 5)

	loaderNM := &dataset.CifarLoader{IsMemberSet: false}
	nonMembers, _ := loaderNM.LoadBatch("data/cifar-10-batches-bin/test_batch.bin", 5)

	targetSamples := append(members, nonMembers...)

	// ---------------------------------------------------------
	// 5. 并发审计流水线
	// ---------------------------------------------------------
	engine := audit.NewEngine(thresholds, shadowModel, targetModel, hsja)
	pool := worker.NewAuditPool(engine, 20)

	fmt.Println("🚀 阶段三：全自动化审计流水线开启...")
	finalReports := pool.RunAudit(targetSamples)

	// ---------------------------------------------------------
	// 6. 战报评估
	// ---------------------------------------------------------
	fmt.Println("\n=====================================================")
	fmt.Println("📈 LabelScan-Go 最终审计效能战报")
	fmt.Println("-----------------------------------------------------")

	var tp, fp, tn, fn int
	for _, r := range finalReports {
		predIsMember := strings.Contains(r.Conclusion, "🔴") ||
			strings.Contains(r.Conclusion, "🟡") ||
			strings.Contains(r.Conclusion, "🟠")

		if predIsMember == r.IsMemberTrue {
			if r.IsMemberTrue {
				tp++
			} else {
				tn++
			}
		} else {
			if r.IsMemberTrue {
				fn++
			} else {
				fp++
			}
		}
	}

	total := len(finalReports)
	accuracy := float64(tp+tn) / float64(total) * 100
	precision := 0.0
	if (tp + fp) > 0 {
		precision = float64(tp) / float64(tp+fp) * 100
	}
	recall := 0.0
	if (tp + fn) > 0 {
		recall = float64(tp) / float64(tp+fn) * 100
	}

	fmt.Printf("   > 总审计样本数：   %d\n", total)
	fmt.Printf("   > 审计准确率 (ACC): %.2f%%\n", accuracy)
	fmt.Printf("   > 查准率 (Precision): %.2f%%\n", precision)
	fmt.Printf("   > 查全率 (Recall):    %.2f%%\n", recall)
	fmt.Println("=====================================================")
}
