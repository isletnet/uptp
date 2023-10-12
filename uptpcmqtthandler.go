package uptp

import (
	"crypto/tls"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/isletnet/uptp/logging"
)

type uptpcMqttHandler struct {
	mtx sync.RWMutex
	mc  mqtt.Client
	opt uptpcMqttOption
}

func newUptpcMqttHandler(opt uptpcMqttOption) *uptpcMqttHandler {
	return &uptpcMqttHandler{
		opt: opt,
	}
}

type uptpcMqttOption struct {
	host       string
	port       int
	name       string
	epid       uint64
	handler    uptpMsgHandler
	mqttPrefix string

	disConnCallback func(error)
}

func (h *uptpcMqttHandler) connectMqtt() error {
	opts := mqtt.NewClientOptions()
	opts.SetClientID(h.opt.name)

	tlsconfig := &tls.Config{
		InsecureSkipVerify: true,
	}
	opts.SetTLSConfig(tlsconfig)
	opts.AddBroker(fmt.Sprintf("ssl://%s:%d", h.opt.host, h.opt.port))
	opts.SetUsername(h.opt.name)
	opts.SetPassword("")
	opts.SetKeepAlive(30 * time.Second)
	opts.SetAutoReconnect(false)
	if h.opt.mqttPrefix == "uptpc" {
		opts.SetWill(fmt.Sprintf("uptpcwill/%d", h.opt.epid), " ", 0, false)
	}
	// opts.SetOnConnectHandler(func(c mqtt.Client) {
	// 	h.opt.handler.onConnect()
	// })
	opts.SetConnectionLostHandler(func(c mqtt.Client, err error) {
		h.mtx.Lock()
		defer h.mtx.Unlock()
		h.mc = nil
		h.opt.disConnCallback(err)
	})
	client := mqtt.NewClient(opts)
	if token := client.Connect(); token.Wait() && token.Error() != nil {
		return token.Error()
	}
	if token := client.Subscribe(fmt.Sprintf("%s/%d/+/+", h.opt.mqttPrefix, h.opt.epid), 0, func(c mqtt.Client, m mqtt.Message) {
		topics := strings.Split(m.Topic(), "/")
		if len(topics) != 4 {
			logging.Error("wrong topic : %s", m.Topic())
			return
		}
		epid, _ := strconv.ParseUint(topics[1], 10, 64)
		if epid != h.opt.epid {
			logging.Error("wrong node id in topic : %s", topics[1])
			return
		}
		msgType, err := strconv.ParseUint(topics[2], 10, 16)
		if err != nil {
			logging.Error("wrong msgType id in topic : %s", topics[2])
			return
		}
		corrID, err := strconv.ParseUint(topics[3], 10, 64)
		if err != nil {
			logging.Error("wrong correlation id in topic : %s", topics[3])
			return
		}
		h.opt.handler.onMessage(uptpMsg{
			EPID:          epid,
			MsgType:       uint16(msgType),
			CorrelationID: corrID,
			Content:       m.Payload(),
		})
	}); token.Wait() && token.Error() != nil {
		client.Disconnect(50)
		return token.Error()
	}
	h.mc = client
	return nil
}
func (h *uptpcMqttHandler) sendUptpMsg(msg *uptpMsg) error {
	return h.sendUptpMsgTo(fmt.Sprintf("uptps/%d", h.opt.epid), msg)
}

func (h *uptpcMqttHandler) sendUptpMsgTo(to string, msg *uptpMsg) error {
	h.mtx.RLock()
	defer h.mtx.RUnlock()
	if h.mc == nil {
		return fmt.Errorf("no active mqtt client")
	}

	if token := h.mc.Publish(fmt.Sprintf("%s/%d/%d", to, msg.MsgType, msg.CorrelationID), 0, false, msg.Content); token.Wait() && token.Error() != nil {
		return token.Error()
	}
	return nil

}
