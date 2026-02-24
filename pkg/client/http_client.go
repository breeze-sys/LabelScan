package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	// 引入核心定义，确保类型匹配
	// 请确保你的 go.mod 名字是 Label-Only-MIA-Go
	"Label-Only-MIA-Go/pkg/core"
)

// ==========================================
// 1. 结构体定义
// ==========================================

// HTTPClient 封装了与 Python 服务端的通信逻辑
// 它实现了 core.Model 接口
type HTTPClient struct {
	url        string       // Python 服务的地址，例如 http://127.0.0.1:5000/predict
	httpClient *http.Client // 内置的 http 客户端，用于管理连接池
}

// requestBody 发送给 Python 的 JSON 数据结构
type requestBody struct {
	Image []float32 `json:"image"` // 对应 Python 端的 request.json['image']
}

// responseBody Python 返回的 JSON 数据结构
type responseBody struct {
	Label int `json:"label"` // 对应 Python 端的 return {"label": 5}
}

// ==========================================
// 2. 初始化函数
// ==========================================

// NewClient 创建并配置一个高性能的 HTTP 客户端
func NewClient(targetURL string) *HTTPClient {
	// 配置连接池 (关键优化！防止高并发下端口耗尽)
	t := &http.Transport{
		MaxIdleConns:        100,              // 最大空闲连接数
		MaxIdleConnsPerHost: 100,              // 对同一个 Host 的最大连接数 (咱们只连一个 Python)
		IdleConnTimeout:     90 * time.Second, // 空闲超时
	}

	return &HTTPClient{
		url: targetURL,
		httpClient: &http.Client{
			Transport: t,
			Timeout:   10 * time.Second, // 防止请求卡死，10秒超时
		},
	}
}

// ==========================================
// 3. 核心功能实现 (实现 core.Model 接口)
// ==========================================

// Load 只是为了满足接口定义，HTTP 模式下不需要加载本地模型文件
// 所以这里留空即可
func (c *HTTPClient) Load(path string) error {
	return nil
}

// Predict 发送图片给 Python Server 并获取预测结果
func (c *HTTPClient) Predict(img core.Image) (int, error) {
	// 1. 准备请求数据
	// 注意：img 是 []float32，直接封装进结构体
	payload := requestBody{
		Image: img,
	}

	// 2. 序列化为 JSON
	jsonData, err := json.Marshal(payload)
	if err != nil {
		return -1, fmt.Errorf("JSON编码失败: %v", err)
	}

	// 3. 发送 POST 请求
	// 使用 c.httpClient 而不是 http.Post，以复用连接
	resp, err := c.httpClient.Post(c.url, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return -1, fmt.Errorf("请求发送失败: %v", err)
	}
	defer resp.Body.Close() // 必须关闭，否则内存泄露

	// 4. 检查状态码
	if resp.StatusCode != http.StatusOK {
		// 读取错误信息
		bodyBytes, _ := io.ReadAll(resp.Body)
		return -1, fmt.Errorf("服务器报错 (Code %d): %s", resp.StatusCode, string(bodyBytes))
	}

	// 5. 解析返回结果
	var result responseBody
	// 使用 json.Decoder 解析流，比 Unmarshal 稍微快一点
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return -1, fmt.Errorf("解析响应失败: %v", err)
	}

	// 6. 返回预测的 Label
	return result.Label, nil
}