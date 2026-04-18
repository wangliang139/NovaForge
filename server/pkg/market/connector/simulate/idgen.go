package simulate

import (
	"hash/fnv"
	"strconv"
	"strings"
	"sync"
	"time"
)

type compactIDGen struct {
	mu     sync.Mutex
	lastMs uint64
	seq    uint64
}

var globalCompactIDGen = &compactIDGen{}

// GenerateCompactID returns a compact numeric order id string.
func GenerateCompactID(accountID string) string {
	ms := uint64(time.Now().UnixMilli())
	acc := accountBits(accountID)
	seq := globalCompactIDGen.nextSeq(ms)
	id := (ms << 22) | (acc << 12) | seq
	return strconv.FormatUint(id, 10)
}

func (g *compactIDGen) nextSeq(ms uint64) uint64 {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.lastMs == ms {
		g.seq = (g.seq + 1) & 0xFFF
	} else {
		g.lastMs = ms
		g.seq = 0
	}
	return g.seq
}

func accountBits(accountID string) uint64 {
	id := strings.TrimSpace(accountID)
	if id == "" {
		return 0
	}
	if n, err := strconv.ParseUint(id, 10, 64); err == nil {
		return n & 0x3FF
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte(id))
	return uint64(h.Sum32()) & 0x3FF
}
