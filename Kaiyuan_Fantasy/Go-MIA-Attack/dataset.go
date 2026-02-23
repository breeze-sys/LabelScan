package main

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
)

const (
	ImageSize  = 3 * 32 * 32
	SampleBytes = 4 + (ImageSize * 4) // Label(4) + Pixels(3072*4)
)

// LoadData 从 .bin 文件读取样本
// isMember: 标记这些数据是否为成员
func LoadData(filename string, isMember bool) ([]Sample, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, fmt.Errorf("无法打开文件: %v", err)
	}
	defer file.Close()

	var samples []Sample
	idCounter := 0

	for {
		// 1. 读取 Label (int32)
		var label int32
		err := binary.Read(file, binary.LittleEndian, &label)
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}

		// 2. 读取 Image (3072 * float32)
		pixels := make([]float32, ImageSize)
		err = binary.Read(file, binary.LittleEndian, &pixels)
		if err != nil {
			return nil, err
		}

		// 3. 存入切片
		samples = append(samples, Sample{
			ID:       idCounter,
			Image:    pixels,
			Label:    int(label),
			IsMember: isMember,
		})
		idCounter++
	}

	fmt.Printf("[Dataset] 从 %s 加载了 %d 张样本 (Member=%v)\n", filename, len(samples), isMember)
	return samples, nil
}