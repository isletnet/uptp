package uptp

import "sync"

type uptpcMsgDispatcher struct {
	dispatchMap map[uint16]func(*uptpMsg)
	mapMtx      sync.RWMutex

	waits sync.Map
}

func newUptpcMsgDispatcher() *uptpcMsgDispatcher {
	return &uptpcMsgDispatcher{
		dispatchMap: make(map[uint16]func(*uptpMsg)),
	}
}

func (d *uptpcMsgDispatcher) onMessage(msg uptpMsg) {
	if msg.CorrelationID != 0 {
		v, ok := d.waits.Load(msg.CorrelationID)
		if ok {
			ch, ok := v.(chan *uptpMsg)
			if ok {
				ch <- &msg
				return
			}
		}
	}
	d.mapMtx.RLock()
	h, ok := d.dispatchMap[msg.MsgType]
	d.mapMtx.RUnlock()
	if ok {
		h(&msg)
	}
}

func (d *uptpcMsgDispatcher) addWaitCh(corrID uint64, ch chan *uptpMsg) {
	d.waits.Store(corrID, ch)
}
func (d *uptpcMsgDispatcher) delWaitCh(corrID uint64) {
	d.waits.Delete(corrID)
}

func (d *uptpcMsgDispatcher) registerHandler(t uint16, h func(*uptpMsg)) {
	d.mapMtx.Lock()
	defer d.mapMtx.Unlock()

	d.dispatchMap[t] = h
}

func (d *uptpcMsgDispatcher) unregisterHandler(t uint16) {
	d.mapMtx.Lock()
	defer d.mapMtx.Unlock()

	delete(d.dispatchMap, t)
}
