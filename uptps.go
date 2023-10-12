package uptp

import "fmt"

type Uptps struct {
	opt        UptpsOption
	dispatcher *uptpsMsgDispatcher
}

type uptpMsgSend interface {
	sendUptpMsg(msg uptpMsg) error
}

func NewUptps(opt UptpsOption) *Uptps {
	return &Uptps{
		opt:        opt,
		dispatcher: newUptpsMsgDispatcher(),
	}
}

func (s *Uptps) Start() {
	mqttOpt := uptpsMqttOption{
		addrs:   fmt.Sprintf("tcp://%s:%d", s.opt.Host, s.opt.Port),
		secret:  s.opt.Secret,
		handler: s.dispatcher,
	}
	mqttHandler := newUptpsMqttHandler(mqttOpt)
	hbHandler := HeartbeatHandler{mqttHandler}
	uptpcHandler := uptpcInfoHandler{mqttHandler}
	s.dispatcher.registerHandler(msgIDTypeHeartbeat, hbHandler.handleMessage)
	s.dispatcher.registerHandler(msgIDTypeUptpcInfoReport, uptpcHandler.handleBaseInfoReport)
	mqttHandler.start(2)
	for i := 0; i < s.opt.WorkerNum; i++ {
		go s.dispatcher.handleMessage()
	}
}

type UptpsOption struct {
	Host      string
	Port      int
	Secret    string
	WorkerNum int
}
