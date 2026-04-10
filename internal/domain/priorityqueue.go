package domain

import (
	"container/heap"
	"sync"
	"time"
)

type PriorityQueueItem struct {
	Value                 string
	Priority              int
	AccumulationStartTime time.Time
}

type PriorityQueue struct {
	mu    sync.RWMutex
	q     []PriorityQueueItem
	index map[string]int
}

func NewPriorityQueue() *PriorityQueue {
	return &PriorityQueue{
		q:     make([]PriorityQueueItem, 0),
		index: make(map[string]int),
	}
}

func (pq *PriorityQueue) Len() int {
	return len(pq.q)
}

func (pq *PriorityQueue) Less(i, j int) bool {
	return pq.q[i].Priority > pq.q[j].Priority
}

func (pq *PriorityQueue) Swap(i, j int) {
	pq.q[i], pq.q[j] = pq.q[j], pq.q[i]
	pq.index[pq.q[i].Value] = i
	pq.index[pq.q[j].Value] = j
}

func (pq *PriorityQueue) Push(x interface{}) {
	n := len(pq.q)
	item := x.(PriorityQueueItem)
	pq.index[item.Value] = n
	pq.q = append(pq.q, item)
}

func (pq *PriorityQueue) Pop() interface{} {
	old := pq.q
	n := len(old)
	item := old[n-1]
	delete(pq.index, item.Value)
	pq.q = old[:n-1]
	return item
}

func (pq *PriorityQueue) UpdateOrPush(value string, priority int) {
	pq.mu.Lock()
	defer pq.mu.Unlock()
	index, indexFound := pq.index[value]
	if !indexFound {
		heap.Push(pq, PriorityQueueItem{
			Value:    value,
			Priority: priority,
		})
		return
	}
	pq.q[index].Priority = priority
	heap.Fix(pq, index)
}

// AccumulateOrPush adds priorityAddition to the existing priority if the item exists and its accumulation duration has not expired,
// otherwise it pushes a new item with the provided priorityAddition as its priority
func (pq *PriorityQueue) AccumulateOrPush(value string, priorityAddition int, curTime time.Time, accumulationDuration time.Duration) {
	pq.mu.Lock()
	defer pq.mu.Unlock()
	index, indexFound := pq.index[value]
	if indexFound {
		item := pq.q[index]
		if curTime.Sub(item.AccumulationStartTime) <= accumulationDuration {
			pq.q[index].Priority += priorityAddition
			heap.Fix(pq, index)
			return
		} else {
			// accumulation duration has expired, so we remove the old item and push a new one below
			heap.Remove(pq, index)
		}
	}
	// either item doesn't exist or accumulation duration has expired, so we push a new item
	heap.Push(pq, PriorityQueueItem{
		Value:                 value,
		Priority:              priorityAddition,
		AccumulationStartTime: curTime,
	})
}

func (pq *PriorityQueue) PeekN(n int) []PriorityQueueItem {
	pq.mu.Lock()
	defer pq.mu.Unlock()
	if n > pq.Len() {
		n = pq.Len()
	}
	if n == 1 {
		return []PriorityQueueItem{pq.q[len(pq.q)-1]}
	}
	result := make([]PriorityQueueItem, n)
	for i := range n {
		item := heap.Pop(pq).(PriorityQueueItem)
		result[i] = item
	}
	for _, v := range result {
		heap.Push(pq, v)
	}
	return result
}
