package audio

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/go-audio/audio"
	"github.com/go-audio/wav"
)

const (
	sampleRate  = 8000
	bitDepth    = 16
	numChannels = 1
)

// WriteWAV 将 PCM int16 样本写入 WAV 文件
// filePath: 输出文件路径
// samples: 所有 PCM 样本（8kHz, 16-bit, mono）
func WriteWAV(filePath string, samples []int16) error {
	if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
		return fmt.Errorf("create dir: %w", err)
	}

	f, err := os.Create(filePath)
	if err != nil {
		return fmt.Errorf("create wav file: %w", err)
	}
	defer f.Close()

	enc := wav.NewEncoder(f, sampleRate, bitDepth, numChannels, 1)

	intSamples := make([]int, len(samples))
	for i, s := range samples {
		intSamples[i] = int(s)
	}

	buf := &audio.IntBuffer{
		Data: intSamples,
		Format: &audio.Format{
			SampleRate:  sampleRate,
			NumChannels: numChannels,
		},
		SourceBitDepth: bitDepth,
	}

	if err := enc.Write(buf); err != nil {
		return fmt.Errorf("write pcm: %w", err)
	}

	return enc.Close()
}

// PCMToFloat32 将 int16 PCM 转换为 float32 [-1, 1]，用于 WebSocket 实时推送
func PCMToFloat32(samples []int16) []float32 {
	out := make([]float32, len(samples))
	for i, s := range samples {
		out[i] = float32(s) / 32768.0
	}
	return out
}
