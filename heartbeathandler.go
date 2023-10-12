package uptp

import (
	"encoding/binary"
	"time"
)

type HeartbeatHandler struct {
	uptpMsgSend
}

func (h HeartbeatHandler) handleMessage(msg *uptpMsg) error {
	tsBuf := make([]byte, 8)
	binary.LittleEndian.PutUint64(tsBuf, uint64(time.Now().UnixNano()))
	// ctx.Write(ctx.From, UptpMsgHeartbeat, 0, tsBuf)
	h.sendUptpMsg(uptpMsg{
		MsgType:       msg.MsgType,
		EPID:          msg.EPID,
		CorrelationID: msg.CorrelationID,
		Content:       tsBuf,
	})
	return nil
}
