package internal

type internalHeap []*internalItem

func (h internalHeap) Len() int { return len(h) }

func (h internalHeap) Less(i, j int) bool {
	ai := h[i].ev
	aj := h[j].ev
	if ai == nil && aj == nil {
		return false
	}
	if ai == nil {
		return true
	}
	if aj == nil {
		return false
	}
	if !ai.Ts.Equal(aj.Ts) {
		return ai.Ts.Before(aj.Ts)
	}
	if ai.SourceSeq != aj.SourceSeq {
		return ai.SourceSeq < aj.SourceSeq
	}
	// 最后兜底：SourceID 字典序稳定 tie-break（不承载语义）。
	return ai.SourceID < aj.SourceID
}

func (h internalHeap) Swap(i, j int) { h[i], h[j] = h[j], h[i] }

func (h *internalHeap) Push(x any) { *h = append(*h, x.(*internalItem)) }

func (h *internalHeap) Pop() any {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[:n-1]
	return x
}
