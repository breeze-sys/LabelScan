package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	// 引入项目内部包
	"Label-Only-MIA-Go/pkg/attack"
	"Label-Only-MIA-Go/pkg/client"
	"Label-Only-MIA-Go/pkg/core"
	"Label-Only-MIA-Go/pkg/dataset"
	"Label-Only-MIA-Go/pkg/worker"
)

func main() {
	// ========================================================================
	// 1. 命令行参数配置
	// ========================================================================
	url := flag.String("url", "http://127.0.0.1:8080/predict", "Python模型服务的 API 地址")
	dataPath := flag.String("data", "./data/test_batch.bin", "CIFAR-10 数据集路径")
	limit := flag.Int("limit", 100, "攻击样本数量 (-1 代表全部)")
	workers := flag.Int("workers", 20, "并发工人数 (Goroutines)")
	
	// HSJA 算法参数配置
	maxIter := flag.Int("max_iter", 50, "HSJA: 最大迭代次数")
	numEvals := flag.Int("evals", 100, "HSJA: 梯度估算的查询次数")
	initEvals := flag.Int("init_evals", 100, "HSJA: 初始化查询次数")

	flag.Parse()

	// 检查文件是否存在
	if _, err := os.Stat(*dataPath); os.IsNotExist(err) {
		log.Fatalf("❌ 错误: 数据文件未找到: %s", *dataPath)
	}

	fmt.Println("\n🛡️  Label-Only MIA (HSJA) 攻击系统启动")
	fmt.Println("========================================")
	fmt.Printf("📍 目标服务: %s\n", *url)
	fmt.Printf("📂 数据来源: %s (Limit: %d)\n", *dataPath, *limit)
	fmt.Printf("🚀 并发规模: %d Workers\n", *workers)
	fmt.Println("========================================")

	// ========================================================================
	// 2. 组件初始化 (组装流水线)
	// ========================================================================

	// [B] 初始化 HTTP 模型客户端
	fmt.Println("UNKNOWN... 正在连接模型服务...")
	modelClient := client.NewClient(*url)

	// [A] 初始化 HSJA 攻击算法
	// 使用你提供的结构体配置
	hsjaConfig := attack.HSJAConfig{
		MaxIterations: *maxIter,
		NumEvals:      *numEvals,
		InitEvals:     *initEvals,
	}
	attacker := attack.NewHSJA(hsjaConfig)
	fmt.Printf("🔧 攻击算法: HSJA (Iter=%d, Evals=%d)\n", *maxIter, *numEvals)

	// [C] 加载 CIFAR-10 数据
	fmt.Println("UNKNOWN... 正在加载数据...")
	// 注意：IsMemberSet 设为 false，假设我们攻击的是测试集（非成员）
	// 如果你跑的是训练集，这里应该设为 true
	loader := &dataset.CifarLoader{IsMemberSet: false} 
	
	samples, err := loader.LoadBatch(*dataPath, *limit)
	if err != nil {
		log.Fatalf("❌ 数据加载失败: %v", err)
	}
	fmt.Printf("✅ 数据加载完成: %d 个样本\n", len(samples))

	// ========================================================================
	// 3. 执行并发审计 (核心逻辑)
	// ========================================================================
	
	startTime := time.Now()

	// [Worker] 初始化审计员并开始干活
	// 这里的 NewAuditor 和 RunAudit 正是你提供的 pool.go 里的逻辑
	auditor := worker.NewAuditor(modelClient, attacker, *workers)
	
	// 开始跑！这里会阻塞直到所有图片处理完毕
	results := auditor.RunAudit(samples)

	duration := time.Since(startTime)

	// ========================================================================
	// 4. 结果分析与报告
	// ========================================================================
	printReport(results, duration)
}

// printReport 打印最终的统计结果
func printReport(results []core.AttackResult, duration time.Duration) {
	var successCount int
	var totalQueries int
	var totalDist float64
	var validDistCount int

	fmt.Println("\n📊 实时攻击日志:")
	fmt.Println("----------------------------------------")
	
	for _, res := range results {
		status := "❌ 失败"
		if res.IsSuccess {
			status = "✅ 成功"
			successCount++
			// 只有成功的攻击才计算距离均值，或者你可以根据需求调整
			totalDist += res.Distance
			validDistCount++
		}
		totalQueries += res.Queries

		// 简单打印日志
		fmt.Printf("ID: %-4d | %s | Label: %d->%d | Dist: %.4f | Q: %d\n", 
			res.SampleID, status, res.OriginalLabel, res.FinalLabel, res.Distance, res.Queries)
	}

	// 防止除零错误
	avgDist := 0.0
	if validDistCount > 0 {
		avgDist = totalDist / float64(validDistCount)
	}
	
	successRate := float64(successCount) / float64(len(results)) * 100
	avgTime := duration / time.Duration(len(results))

	fmt.Println("\n========================================")
	fmt.Println("             🏁 最终审计报告             ")
	fmt.Println("========================================")
	fmt.Printf("⏱️  总耗时       : %s\n", duration)
	fmt.Printf("⚡ 平均单张耗时 : %s\n", avgTime)
	fmt.Printf("🎯 攻击成功率   : %.2f%% (%d/%d)\n", successRate, successCount, len(results))
	fmt.Printf("📏 平均扰动距离 : %.4f (L2)\n", avgDist)
	fmt.Printf("🔍 总查询次数   : %d\n", totalQueries)
	fmt.Println("========================================")
}