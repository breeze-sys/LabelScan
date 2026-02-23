package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
)

const PythonAPI = "http://127.0.0.1:8080/predict"

func main() {
	fmt.Println("🚀 Go 攻击客户端启动...")

	// 1. 加载数据
	members, err := LoadData("data/members.bin", true)
	if err != nil {
		log.Fatalf("加载 Member 失败: %v (请先运行 export_data.py)", err)
	}
	
	// 2. 拿第一张图做测试
	testImg := members[0]
	fmt.Printf("🧪 正在测试第 0 张图 (Label=%d)...\n", testImg.Label)

	// 3. 发送 HTTP 请求
	result, err := queryPythonModel(testImg.Image)
	if err != nil {
		log.Fatalf("API 请求失败: %v", err)
	}

	// 4. 输出结果
	fmt.Printf("✅ Python返回: 预测Label=%d (真实Label=%d)\n", result.Label, testImg.Label)
	if result.Label == testImg.Label {
		fmt.Println("🎉 预测正确！通信链路已打通！")
	} else {
		fmt.Println("⚠️ 预测错误 (这是正常的，不用慌)")
	}
}

// 简单的 HTTP 请求封装
func queryPythonModel(img []float32) (*PredictResponse, error) {
	reqBody := PredictRequest{Image: img}
	jsonData, _ := json.Marshal(reqBody)

	resp, err := http.Post(PythonAPI, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var res PredictResponse
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, err
	}
	return &res, nil
}