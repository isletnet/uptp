package uptp

import (
	"crypto/tls"
	"fmt"
	"math/rand"
	"strconv"
	"strings"
	"sync"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/golang-jwt/jwt/v4"
)

type uptpsMqttOption struct {
	addrs   string
	secret  string
	handler uptpMsgHandler
}

type uptpsMqttHandler struct {
	opt uptpsMqttOption

	mcs []mqtt.Client
	mtx sync.RWMutex
}

func newUptpsMqttHandler(opt uptpsMqttOption) *uptpsMqttHandler {
	return &uptpsMqttHandler{
		opt: opt,
	}
}

func (mc *uptpsMqttHandler) start(num int) {
	for i := 0; i < num; i++ {
		c := mc.createMqttClient()
		if c == nil {
			continue
		}
		mc.mcs = append(mc.mcs, c)
		// gLog.Println(LvINFO, "start mqtt client: ", i)
	}
	if len(mc.mcs) == 0 {
		panic("no mqtt connect")
	}
}

func (mc *uptpsMqttHandler) sendUptpMsg(msg uptpMsg) error {
	mc.mtx.RLock()
	defer mc.mtx.RUnlock()
	num := len(mc.mcs)
	if num == 0 {
		return fmt.Errorf("no active mqtt client")
	}

	c := mc.mcs[rand.Intn(num)]
	if token := c.Publish(fmt.Sprintf("uptpc/%d/%d/%d", msg.EPID, msg.MsgType, msg.CorrelationID), 0, false, msg.Content); token.Wait() && token.Error() != nil {
		return token.Error()
	}
	return nil
}

func (mc *uptpsMqttHandler) delClient(client mqtt.Client) {
	mc.mtx.Lock()
	defer mc.mtx.Unlock()
	var index = -1
	for i, c := range mc.mcs {
		if c == client {
			index = i
			break
		}
	}
	if index < 0 {
		return
	}
	mc.mcs = append(mc.mcs[:index], mc.mcs[index+1:]...)
}

func (mc *uptpsMqttHandler) addClient(c mqtt.Client) {
	mc.mtx.Lock()
	mc.mcs = append(mc.mcs, c)
	mc.mtx.Unlock()
}

func (mc *uptpsMqttHandler) onDisconnect(c mqtt.Client, e error) {
	mc.delClient(c)
	go mc.reconnectMqtt()
}

func (mc *uptpsMqttHandler) reconnectMqtt() {
	client := mc.createMqttClient()
	if client == nil {
		time.Sleep(time.Second)
		go mc.reconnectMqtt()
		return
	}
	mc.addClient(client)
	// gLog.Println(LvINFO, "add mqtt client")
}

func (mc *uptpsMqttHandler) createMqttClient() mqtt.Client {
	acl := emqxACL{}
	acl.ALL = make([]string, 0)
	acl.ALL = append(acl.ALL, "#")
	expiresTime := time.Now().Add(time.Hour * 24 * 360 * 10)
	password, err := NewAuthJwt(mc.opt.secret).genToken(acl, expiresTime)
	if err != nil {
		// gLog.Println(LvERROR, "gen mqtt jwt fail: ", err)
		return nil
	}
	tlsconfig := &tls.Config{
		InsecureSkipVerify: true,
	}
	opts := mqtt.NewClientOptions()
	opts.SetTLSConfig(tlsconfig)
	name := fmt.Sprintf("uptps_%d", time.Now().Nanosecond())
	opts.SetClientID(name)
	opts.AddBroker(mc.opt.addrs)
	opts.SetUsername(name)
	opts.SetPassword(password)
	opts.SetKeepAlive(time.Second * 60)
	opts.SetAutoReconnect(false)
	opts.SetConnectionLostHandler(mc.onDisconnect)
	// opts.SetOnConnectHandler(handler.onConnect)
	client := mqtt.NewClient(opts)
	if token := client.Connect(); token.Wait() && token.Error() != nil {
		// gLog.Println(LvERROR, "connnect mqtt fail : ", token.Error())
		return nil
	}
	if token := client.Subscribe("$share/sgrp/uptpcwill/+", 0, func(c mqtt.Client, m mqtt.Message) {
		topics := strings.Split(m.Topic(), "/")
		if len(topics) != 3 {
			// gLog.Println(LvERROR, "wrong lastewill topic : ", m.Topic())
			return
		}
		epid, _ := strconv.ParseUint(topics[1], 10, 64)
		if epid == 0 {
			// gLog.Println(LvERROR, "wrong node id in topic : ", topics[1])
			return
		}
		// gLog.Printf(LvINFO, "%d offline", nodeID)
		msg := uptpMsg{
			EPID:    epid,
			MsgType: msgIDTypeOffline,
		}
		mc.opt.handler.onMessage(msg)
	}); token.Wait() && token.Error() != nil {
		client.Disconnect(10)
		return nil
	}

	if token := client.Subscribe("$share/sgrp/uptps/+/+/+", 0, func(c mqtt.Client, m mqtt.Message) {
		// log.Println(m.Topic(), ":", string(m.Payload()))
		topics := strings.Split(m.Topic(), "/")
		if len(topics) != 4 {
			// gLog.Println(LvERROR, "wrong topic : ", m.Topic())
			return
		}
		epid, _ := strconv.ParseUint(topics[1], 10, 64)
		if epid == 0 {
			// gLog.Println(LvERROR, "wrong node id in topic : ", topics[1])
			return
		}
		msgType, err := strconv.ParseUint(topics[2], 10, 16)
		if err != nil {
			return
		}
		corrID, err := strconv.ParseUint(topics[3], 10, 64)
		if err != nil {
			return
		}

		mc.opt.handler.onMessage(uptpMsg{
			EPID:          epid,
			MsgType:       uint16(msgType),
			CorrelationID: corrID,
			Content:       m.Payload(),
		})
	}); token.Wait() && token.Error() != nil {
		client.Disconnect(10)
		return nil
	}
	return client
}

type authJwt struct {
	key string
}

func NewAuthJwt(apiKey string) *authJwt {
	return &authJwt{
		key: apiKey,
	}
}

type emqxACL struct {
	PUB []string `json:"pub"`
	SUB []string `json:"sub"`
	ALL []string `json:"all"`
}
type jwtCustomClaims struct {
	ACL emqxACL `json:"acl"`
	jwt.RegisteredClaims
}

func (aj *authJwt) genToken(acl emqxACL, expiresAt time.Time) (string, error) {

	// Set custom claims
	claims := &jwtCustomClaims{
		ACL: acl,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expiresAt),  //过期时间
			IssuedAt:  jwt.NewNumericDate(time.Now()), //签发时间
			NotBefore: jwt.NewNumericDate(time.Now()), //生效时间
		},
	}

	// Create token with claims
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	//return token.SignedString([]byte("secret"))
	return token.SignedString([]byte(aj.key))
}
