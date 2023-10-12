package uptp

import (
	"encoding/json"
	"net/http"
	"runtime"
	"sync/atomic"
	"time"

	"github.com/isletnet/uptp/logging"
	"github.com/lesismal/nbio/nbhttp"
)

type MqttAuthSvr struct {
	qps   uint64
	total uint64

	mux *http.ServeMux
	svr *nbhttp.Server
}

func NewEmqxAuthSvr() *MqttAuthSvr {
	s := &MqttAuthSvr{}

	s.mux = &http.ServeMux{}
	s.mux.HandleFunc("/authen", s.onAuthen)
	s.mux.HandleFunc("/author", s.onAuthor)

	s.svr = nbhttp.NewServer(nbhttp.Config{
		Network: "tcp",
		Addrs:   []string{":8888"},
		Handler: s.mux,
	}) // pool.Go)
	return s
}

// {"username":"testestest","publicip":"172.16.0.4","password":"3847384738537843","mqttid":"testestest"}
type mqttAuthen struct {
	Username string `json:"username"`
	PublicIP string `json:"publicip"`
	Password string `json:"password"`
	MqttID   string `json:"mqttid"`
}

func (s *MqttAuthSvr) onAuthen(w http.ResponseWriter, r *http.Request) {
	data := r.Body.(*nbhttp.BodyReader).RawBody()
	authen := mqttAuthen{}
	err := json.Unmarshal(data, &authen)
	if err != nil {
		logging.Info("authen error: %s", err)
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"result":       "deny", //deny
			"is_superuser": false,
		})
	}
	logging.Info("on authen: %+v", authen)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"result":       "allow", //deny
		"is_superuser": false,
	})
	atomic.AddUint64(&s.qps, 1)
}

type mqttAuthor struct {
	Username string `json:"username"`
	Action   string `json:"action"`
	Topic    string `json:"topic"`
	MqttID   string `json:"mqttid"`
}

func (s *MqttAuthSvr) onAuthor(w http.ResponseWriter, r *http.Request) {
	data := r.Body.(*nbhttp.BodyReader).RawBody()
	author := mqttAuthor{}
	err := json.Unmarshal(data, &author)
	if err != nil {
		logging.Info("author error: %s", err)
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"result":       "deny", //deny
			"is_superuser": false,
		})
	}
	logging.Info("on author: %+v", author)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"result":       "allow", //deny
		"is_superuser": false,
	})
	atomic.AddUint64(&s.qps, 1)
}

func (s *MqttAuthSvr) Run() {
	err := s.svr.Start()
	if err != nil {
		logging.Error("nbio.Start failed: %v", err)
		return
	}
	defer s.svr.Stop()

	ticker := time.NewTicker(time.Hour)
	for i := 1; true; i++ {
		<-ticker.C
		n := atomic.SwapUint64(&s.qps, 0)
		s.total += n
		logging.Error("running for %v seconds, NumGoroutine: %v, qps: %v, total: %v", i, runtime.NumGoroutine(), n, s.total)
	}
}

func writeJSON(w http.ResponseWriter, code int, obj interface{}) error {
	w.WriteHeader(http.StatusOK)
	writeContentTypeJson(w)
	jsonBytes, err := json.Marshal(obj)
	if err != nil {
		return err
	}
	_, err = w.Write(jsonBytes)
	return err
}

func writeContentTypeJson(w http.ResponseWriter) {
	header := w.Header()
	if val := header["Content-Type"]; len(val) == 0 {
		header["Content-Type"] = []string{"application/json; charset=utf-8"}
	}
}
