package utils

import (
	"crypto/md5"
	"encoding/binary"
	"encoding/hex"
	"strings"
)

type _hash struct{}

var Hash = _hash{}

const hashBits = 64

func (h _hash) ShortMd5(text string) string {
	hash := md5.Sum([]byte(text))
	return hex.EncodeToString(hash[:8])
}

func (h _hash) Md5(text string) (string, error) {
	hash := md5.Sum([]byte(text))
	return hex.EncodeToString(hash[:]), nil
}

func (h _hash) hashWord(word string) uint64 {
	hash := md5.Sum([]byte(word))
	return binary.LittleEndian.Uint64(hash[:8])
}

// 计算SimHash
func (h _hash) Simhash(text string) uint64 {
	tokens := strings.Fields(strings.ToLower(text))

	// 特征向量，统计每个位的权重
	var vector [hashBits]float64

	// 词频统计
	freq := make(map[string]float64)
	for _, token := range tokens {
		freq[token]++
	}

	// 计算TF-IDF权重（此例只使用TF）
	var words []string
	var weights []float64
	for word, f := range freq {
		words = append(words, word)
		weights = append(weights, f)
	}

	// 累加向量
	for i, word := range words {
		hash := h.hashWord(word)
		for j := 0; j < hashBits; j++ {
			bit := (hash >> j) & 1
			if bit == 1 {
				vector[j] += weights[i]
			} else {
				vector[j] -= weights[i]
			}
		}
	}

	// 生成最终的SimHash
	var simhash uint64 = 0
	for i := 0; i < hashBits; i++ {
		if vector[i] >= 0 {
			simhash |= 1 << i
		}
	}

	return simhash
}
