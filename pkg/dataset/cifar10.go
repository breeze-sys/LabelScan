package dataset

import (
	"Label-Only-MIA-Go/pkg/core"
	"bufio"
	"fmt"
	"io"
	"math/rand"
	"os"
	"time"
)

type CifarLoader struct {
	IsMemberSet bool // 标记当前加载的文件是否属于训练集成员
}

// 1. LoadBatch: 顺序加载 (用于加载 1000 张审计目标图)
func (l *CifarLoader) LoadBatch(path string, limit int) ([]core.Sample, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var samples []core.Sample
	reader := bufio.NewReader(file)
	const recordSize = 3073
	buffer := make([]byte, recordSize)

	count := 0
	for {
		if limit != -1 && count >= limit {
			break
		}
		_, err := io.ReadFull(reader, buffer)
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}

		pixels := make(core.Image, 3072)
		for i := 0; i < 3072; i++ {
			pixels[i] = float32(buffer[i+1]) / 255.0
		}

		samples = append(samples, core.Sample{
			ID:       count,
			Data:     pixels,
			Label:    int(buffer[0]),
			IsMember: l.IsMemberSet,
			Filename: fmt.Sprintf("%s_#%d", path, count),
		})
		count++
	}
	return samples, nil
}

// 2. GetRandomStrangers: 随机抽取 (用于现场定标，算出判定水位线)
// 对应任务：把路人图固定下来
func (l *CifarLoader) GetRandomStrangers(path string, count int) ([]core.Sample, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	// CIFAR-10 固定规格
	const recordSize = 3073
	const totalRecords = 10000

	rand.Seed(time.Now().UnixNano())
	strangers := make([]core.Sample, 0, count)

	for i := 0; i < count; i++ {
		// 随机产生一个索引 [0, 9999]
		randomIndex := rand.Intn(totalRecords)

		// 移动文件指针到该记录的起始位置
		_, err := file.Seek(int64(randomIndex*recordSize), 0)
		if err != nil {
			return nil, err
		}

		buffer := make([]byte, recordSize)
		_, err = io.ReadFull(file, buffer)
		if err != nil {
			return nil, err
		}

		pixels := make(core.Image, 3072)
		for j := 0; j < 3072; j++ {
			pixels[j] = float32(buffer[j+1]) / 255.0
		}

		strangers = append(strangers, core.Sample{
			ID:       -1 - i, // 负数 ID 代表定标样本
			Data:     pixels,
			Label:    int(buffer[0]),
			IsMember: false, // 明确是路人
			Filename: fmt.Sprintf("stranger_%d", randomIndex),
		})
	}

	return strangers, nil
}
