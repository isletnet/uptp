package uptp

import "sync"

type tunnelAddJudger struct {
	mtx   sync.Mutex
	cache map[uint64]int64
}

func newTunnelAddJudger() *tunnelAddJudger {
	return &tunnelAddJudger{
		cache: make(map[uint64]int64),
	}
}
func (j *tunnelAddJudger) checkAndAdd(k uint64, v int64, overwrite bool) bool {
	j.mtx.Lock()
	defer j.mtx.Unlock()
	ov, exist := j.cache[k]
	if !exist || (overwrite && v < ov) {
		j.cache[k] = v
		return true
	}
	return false
}

func (j *tunnelAddJudger) delete(k uint64) {
	j.mtx.Lock()
	delete(j.cache, k)
	j.mtx.Unlock()
}
