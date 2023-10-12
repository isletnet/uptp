package uptp

import "github.com/isletnet/uptp/logging"

type uptpcInfoHandler struct {
	uptpMsgSend
}

func (h *uptpcInfoHandler) handleBaseInfoReport(msg *uptpMsg) error {
	logging.Info(string(msg.Content))
	return nil
}
