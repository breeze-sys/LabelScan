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
	fmt.Println("🛡️  LabelScan-Go: 高性能黑盒模型隐私审计工具 (最终版)")
	fmt.Println("=====================================================")

	// ---------------------------------------------------------
	// 1. 资产加载 (Member A 提供：影子模型两道闸门)
	// ---------------------------------------------------------
	configData, err := ioutil.ReadFile("shadow_config.json")
	if err != nil {
		log.Fatal("❌ 错误：缺少 shadow_config.json。请 Member A 提供 tau_95 和 tau_opt")
	}
	var thresholds audit.AuditThresholds
	if err := json.Unmarshal(configData, &thresholds); err != nil {
		log.Fatalf("❌ 配置解析失败: %v", err)
	}

	// ---------------------------------------------------------
	// 2. 环境初始化 (B & C 的集成)
	// ---------------------------------------------------------
	targetAPI := "http://localhost:8000" // 目标模型 (黑盒)
	shadowAPI := "http://localhost:8001" // 影子模型 (用于信号一)

	targetModel := client.NewHTTPClient(targetAPI)
	shadowModel := client.NewHTTPClient(shadowAPI)

	// 配置边界攻击器 (HSJA)
	hsja := attack.NewHSJA(attack.HSJAConfig{
		MaxQueries:    5000,
		MaxIterations: 40,
		NumEvals:      100,
	})

	// ---------------------------------------------------------
	// 3. 现场定标 (Calibration)：算出当前模型的“水位线”
	// ---------------------------------------------------------
	fmt.Println("\n🔍 阶段一：正在抽取 10 张路人图进行现场定标...")
	loader := &dataset.CifarLoader{}
	strangers, _ := loader.GetRandomStrangers("data/test_batch.bin", 10)

	var refDists [][]float64
	for i, s := range strangers {
		fmt.Printf("   [定标中] 探测路人样本 #%d 的地理特征...\n", i+1)

		// 为路人生成 10 个变体，测量 11 个点 (原图 + 变体)
		variants := mathutils.GenerateVariants(s.Data, 0.001, 10)
		points := append([]core.Image{s.Data}, variants...)

		var groupDists []float64
		for _, img := range points {
			tmp := core.Sample{Data: img, Label: s.Label}
			res := hsja.Attack(tmp, targetModel)
			groupDists = append(groupDists, res.Distance)
		}
		refDists = append(refDists, groupDists)
	}

	// 调用 B 的统计函数锁定 TauD (距离上限) 和 TauCV (波动下限)
	thresholds.TauD, thresholds.TauCV = mathutils.CalibrateReference(refDists)
	fmt.Printf("📊 定标成功：距离红线 %.4f | 波动绿线 %.4f\n", thresholds.TauD, thresholds.TauCV)

	// ---------------------------------------------------------
	// 4. 构造混合测试包 (50成员 + 50非成员，用于科学验证准确率)
	// ---------------------------------------------------------
	fmt.Println("\n📦 阶段二：构造混合测试包 (50名成员 + 50名路人)...")

	// 加载 50 个确定练过的成员 (从 data_batch_1.bin)
	loaderM := &dataset.CifarLoader{IsMemberSet: true}
	members, _ := loaderM.LoadBatch("data/data_batch_1.bin", 50)

	// 加载 50 个确定没见过的路人 (从 test_batch.bin)
	loaderNM := &dataset.CifarLoader{IsMemberSet: false}
	nonMembers, _ := loaderNM.LoadBatch("data/test_batch.bin", 50)

	// 混合进入审计流水线
	targetSamples := append(members, nonMembers...)

	// ---------------------------------------------------------
	// 5. 并发审计流水线 (Engine + Worker Pool)
	// ---------------------------------------------------------
	engine := audit.NewEngine(thresholds, shadowModel, targetModel, hsja)
	pool := worker.NewAuditPool(engine, 20) // 开启 20 个审计窗口

	fmt.Println("🚀 阶段三：全自动化审计流水线开启...")
	finalReports := pool.RunAudit(targetSamples)

	// ---------------------------------------------------------
	// 6. 自动化对账与效能评估 (最后的战报)
	// ---------------------------------------------------------
	fmt.Println("\n=====================================================")
	fmt.Println("📈 LabelScan-Go 最终审计效能战报")
	fmt.Println("-----------------------------------------------------")

	var tp, fp, tn, fn int // 统计学术语：真阳、假阳、真阴、假阴

	for _, r := range finalReports {
		// 逻辑：如果结论里带 🔴 或 🟡/🟠，则视为工具判定为“成员风险”
		predIsMember := strings.Contains(r.Conclusion, "🔴") ||
			strings.Contains(r.Conclusion, "🟡") ||
			strings.Contains(r.Conclusion, "🟠")

		if predIsMember == r.IsMemberTrue {
			// 命中答案
			if r.IsMemberTrue {
				tp++
			} else {
				tn++
			}
		} else {
			// 判定错误
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
	fmt.Printf("   > 查准率 (Precision): %.2f%% (报红的样本中确为成员的比例)\n", precision)
	fmt.Printf("   > 查全率 (Recall):    %.2f%% (成功抓获的真成员比例)\n", recall)
	fmt.Println("=====================================================")
	fmt.Println("💡 提示：详细 JSON 报告已生成在 output/ 目录下。")
}
