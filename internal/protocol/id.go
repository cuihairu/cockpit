package protocol

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
)

var idCounter uint64
var idMutex sync.Mutex

// GenerateID 生成唯一 ID
// 格式: {前缀}-{计数器}-{随机数}
func GenerateID() string {
	idMutex.Lock()
	idCounter++
	count := idCounter
	idMutex.Unlock()

	// 生成随机部分
	randomBytes := make([]byte, 4)
	rand.Read(randomBytes)
	randomPart := hex.EncodeToString(randomBytes)

	return formatID(count, randomPart)
}

// GenerateIDWithPrefix 生成带前缀的 ID
func GenerateIDWithPrefix(prefix string) string {
	id := GenerateID()
	if prefix != "" {
		return prefix + "-" + id
	}
	return id
}

// formatID 格式化 ID
func formatID(count uint64, random string) string {
	// 使用 hex 编码计数器，使 ID 更短
	countPart := hex.EncodeToString([]byte{
		byte(count >> 24),
		byte(count >> 16),
		byte(count >> 8),
		byte(count),
	})
	return countPart + "-" + random
}
