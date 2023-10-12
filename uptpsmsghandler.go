package uptp

// type HandlerInterface interface {
// 	handleMessage(msg *uptpMsg) error
// }

type uptpMsgHandle func(msg *uptpMsg) error

type uptpsMsgDispatcher struct {
	msgCh    chan *uptpMsg
	handlers map[uint16]uptpMsgHandle
}

func newUptpsMsgDispatcher() *uptpsMsgDispatcher {
	return &uptpsMsgDispatcher{
		msgCh:    make(chan *uptpMsg, 1000),
		handlers: make(map[uint16]uptpMsgHandle),
	}
}

func (h *uptpsMsgDispatcher) onMessage(msg uptpMsg) {
	h.msgCh <- &msg
}

func (h *uptpsMsgDispatcher) registerHandler(msgType uint16, hi uptpMsgHandle) {
	h.handlers[msgType] = hi
}
func (h *uptpsMsgDispatcher) handleMessage() {
	for msg := range h.msgCh {
		h, ok := h.handlers[msg.MsgType]
		if ok {
			err := h(msg)
			if err != nil {
				//todo
			}
		}
	}
}
