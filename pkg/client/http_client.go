package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"Label-Only-MIA-Go/pkg/core"
)

// ==========================================
// 1. 结构体与通信协议定义
// ==========================================

type HTTPClient struct {
	url        string // 基础地址，如 http://localhost:8000
	httpClient *http.Client
}

// 单图请求体
type requestBody struct {
	Image []float32 `json:"image"`
}

// 标签返回体 (用于 Predict)
type responseBody struct {
	Label int `json:"label"`
}

// 分数返回体 (用于 PredictLogits - 核心新增！)
type logitsResponse struct {
	Logits []float32 `json:"logits"` // 对应 Python 返回的 [0.1, 0.8, ...]
}

// 批量请求体 (用于 PredictBatch)
type batchRequest struct {
	Images [][]float32 `json:"images"`
}

type batchResponse struct {
	Labels []int `json:"labels"`
}

// ==========================================
// 2. 初始化：NewHTTPClient (对齐 main.go 的调用)
// ==========================================

func NewHTTPClient(baseURL string) *HTTPClient {
	t := &http.Transport{
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 100,
		IdleConnTimeout:     90 * time.Second,
	}

	return &HTTPClient{
		url: baseURL,
		httpClient: &http.Client{
			Transport: t,
			Timeout:   15 * time.Second, // 边界攻击计算慢，建议给 15s 超时
		},
	}
}

// ==========================================
// 3. 核心功能实现 (实现 core.Model 接口)
// ==========================================

// Predict: 获取最终标签
func (c *HTTPClient) Predict(img core.Image) (int, error) {
	return c.PredictContext(context.Background(), img)
}

func (c *HTTPClient) PredictContext(ctx context.Context, img core.Image) (int, error) {
	endpoint := c.url + "/predict"
	payload := requestBody{Image: img}
	jsonData, _ := json.Marshal(payload)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewBuffer(jsonData))
	if err != nil {
		return -1, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return -1, err
	}
	defer resp.Body.Close()

	var result responseBody
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return -1, err
	}
	return result.Label, nil
}

// PredictLogits: 获取原始分数 (用于算方案一的 Loss) —— 【这是你最需要的改动】
func (c *HTTPClient) PredictLogits(img core.Image) ([]float32, error) {
	return c.PredictLogitsContext(context.Background(), img)
}

func (c *HTTPClient) PredictLogitsContext(ctx context.Context, img core.Image) ([]float32, error) {
	endpoint := c.url + "/predict_logits"
	payload := requestBody{Image: img}
	jsonData, _ := json.Marshal(payload)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Logits接口报错: %d", resp.StatusCode)
	}

	var result logitsResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return result.Logits, nil
}

// PredictBatch: 批量预测标签 (用于 HSJA 涡轮增压)
func (c *HTTPClient) PredictBatch(imgs []core.Image) ([]int, error) {
	return c.PredictBatchContext(context.Background(), imgs)
}

func (c *HTTPClient) PredictBatchContext(ctx context.Context, imgs []core.Image) ([]int, error) {
	if len(imgs) == 0 {
		return []int{}, nil
	}

	rawImgs := make([][]float32, len(imgs))
	for i, img := range imgs {
		rawImgs[i] = img
	}

	payload := batchRequest{Images: rawImgs}
	jsonData, _ := json.Marshal(payload)
	batchURL := c.url + "/predict_batch"

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, batchURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result batchResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return result.Labels, nil
}

func (c *HTTPClient) GetInputSize() int {
	return 3072
}
