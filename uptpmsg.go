package uptp

type uptpMsg struct {
	MsgType       uint16
	EPID          uint64
	CorrelationID uint64
	Content       []byte
}

type uptpMsgHandler interface {
	onMessage(uptpMsg)
}
