package domain

type SymbolVolumeMapping struct {
	SymbolName string
	Volume     uint64
}

type VolumeMaxHeap struct {
	data []SymbolVolumeMapping
}

func (h VolumeMaxHeap) Len() int           { return len(h.data) }
// For a MaxHeap, Less(i, j) should return true if element i > element j
func (h VolumeMaxHeap) Less(i, j int) bool { return h.data[i].Volume > h.data[j].Volume } 
func (h VolumeMaxHeap) Swap(i, j int)      { h.data[i], h.data[j] = h.data[j], h.data[i] }

func (h *VolumeMaxHeap) Push(x interface{}) {
    h.data = append(h.data, x.(SymbolVolumeMapping))
}

func (h *VolumeMaxHeap) Pop() interface{} {
    old := h.data
    n := len(old)
    x := old[n-1]
    h.data = old[0 : n-1]
    return x
}