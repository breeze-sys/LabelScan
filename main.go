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
	"os"
	"strings"
)

func main() {
	fmt.Println("=====================================================")
	fmt.Println("🛡️  LabelScan-Go: 高性能黑盒模型隐私审计工具 (最终总装版)")
	fmt.Println("=====================================================")

	// 1. 资产加载 (读取 Member A 提供的影子模型阈值)
	configData, err := ioutil.ReadFile("shadow_config.json")
	if err != nil {
		log.Fatal("❌ 错误：缺少 shadow_config.json。")
	}
	var thresholds audit.AuditThresholds
	if err := json.Unmarshal(configData, &thresholds); err != nil {
		log.Fatalf("❌ 配置解析失败: %v", err)
	}

	// 2. 环境初始化
	targetAPI := "http://localhost:8000"
	shadowAPI := "http://localhost:8001"

	targetModel := client.NewHTTPClient(targetAPI)
	shadowModel := client.NewHTTPClient(shadowAPI)

	hsja := attack.NewHSJA(attack.HSJAConfig{
		MaxQueries:    5000,
		MaxIterations: 40,
		NumEvals:      100,
	})

	// 3. 现场定标
	fmt.Println("\n🔍 阶段一：正在进行现场定标 (寻找 10 个预测正确的路人)...")
	loader := &dataset.CifarLoader{}
	candidates, _ := loader.GetRandomStrangers("data/cifar-10-batches-bin/test_batch.bin", 100)

	var refDists [][]float64
	validStrangers := 0
	for i := 0; i < len(candidates) && validStrangers < 10; i++ {
		s := candidates[i]
		tmpOrig := core.Sample{Data: s.Data, Label: s.Label}
		resOrig := hsja.Attack(tmpOrig, targetModel)
		if resOrig.Distance < 1e-5 {
			fmt.Printf("   [提示] 样本 #%d 预测错误，已自动剔除...\n", i) //
			continue
		}
		fmt.Printf("   [定标中] 正在探测有效路人 %d/10 的几何特征...\n", validStrangers+1)
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

	thresholds.TauD, thresholds.TauCV = mathutils.CalibrateReference(refDists)

	fmt.Println("\n--- 🕵️ 阈值诊断报告 (核心排查数据) ---")
	fmt.Printf("👉 迁移红灯 (Tau95): %.4f\n", thresholds.Tau95)
	fmt.Printf("👉 迁移黄灯 (TauOpt): %.4f\n", thresholds.TauOpt)
	fmt.Printf("👉 距离红线 (TauD):   %.4f\n", thresholds.TauD)
	fmt.Printf("👉 波动绿线 (TauCV):  %.4f\n", thresholds.TauCV)
	fmt.Println("-------------------------------------------\n")

	// 4. 构造混合测试包 (50成员 + 50路人)
	fmt.Println("📦 阶段二：构造混合测试包 (50 真实成员 + 50 真实路人)...")

	// A. 【核心对齐】：读取真实成员索引文件，加载 50 个【真正的目标模型成员】
	indexData, err := ioutil.ReadFile("target_members.json")
	if err != nil {
		log.Fatal("❌ 错误：缺少 target_members.json，请先运行 python export_members.py 生成它！")
	}
	var memberIndices []int
	if err := json.Unmarshal(indexData, &memberIndices); err != nil {
		log.Fatalf("❌ 解析成员索引失败: %v", err)
	}

	loaderM := &dataset.CifarLoader{}
	// 🚨【已修正为 :=】：在这里声明全新的 members 变量，并复用 err 变量
	members, err := loaderM.LoadByIndices("data/cifar-10-batches-bin", memberIndices)
	if err != nil {
		log.Fatalf("❌ 加载真实成员数据失败: %v", err)
	}

	// B. 加载 50 个【绝对没见过的路人】 (从测试集直接顺序读取前 50 张)
	loaderNM := &dataset.CifarLoader{IsMemberSet: false}
	nonMembers, _ := loaderNM.LoadBatch("data/cifar-10-batches-bin/test_batch.bin", 50)

	// 🚨【已修正为 :=】：在这里声明全新的 targetSamples 变量
	targetSamples := append(members, nonMembers...)

	// 5. 并发审计流水线
	engine := audit.NewEngine(thresholds, shadowModel, targetModel, hsja)
	pool := worker.NewAuditPool(engine, 20)
	fmt.Println("🚀 阶段三：全自动化审计流水线开启...")
	finalReports := pool.RunAudit(targetSamples)

	// ---------------------------------------------------------
	// 6. 【核心修正】：效能评估逻辑
	// ---------------------------------------------------------
	fmt.Println("\n=====================================================")
	fmt.Println("📈 LabelScan-Go 最终审计效能战报")
	fmt.Println("-----------------------------------------------------")

	var tp, fp, tn, fn int
	for _, r := range finalReports {
		// 只有这样，统计口径才符合“隐私预警工具”的真实逻辑
		predIsMember := strings.Contains(r.Conclusion, "🔴")

		if predIsMember == r.IsMemberTrue {
			if r.IsMemberTrue {
				tp++ // 成功抓获真成员
			} else {
				tn++ // 正确放行真路人
			}
		} else {
			if r.IsMemberTrue {
				fn++ // 漏网之鱼 (真成员被判绿)
			} else {
				fp++ // 冤假错案 (路人被判红/黄)
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
	fmt.Printf("   > 查准率 (Precision): %.2f%% (判定为成员中真实的比例)\n", precision)
	fmt.Printf("   > 查全率 (Recall):    %.2f%% (成功抓获的真成员比例)\n", recall)
	fmt.Println("=====================================================")

	// 7. 持久化
	_ = os.Mkdir("output", 0755)
	file, err := os.Create("output/audit_report.json")
	if err == nil {
		defer file.Close()
		enc := json.NewEncoder(file)
		enc.SetIndent("", "  ")
		enc.Encode(finalReports)
		fmt.Println("💾 详细报告已保存至 output/audit_report.json")
	} else {
		fmt.Printf("⚠️ 警告：无法创建战报文件: %v\n", err)
	}
}
