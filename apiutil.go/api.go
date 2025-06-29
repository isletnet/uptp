package apiutil

import (
	"encoding/json"
	"errors"
	"net"
	"net/http"

	"github.com/go-chi/chi"
	"github.com/go-chi/chi/middleware"
)

type ApiServer struct {
	*chi.Mux
}

func NewApiServer() *ApiServer {
	ret := &ApiServer{
		Mux: chi.NewRouter(),
	}
	ret.Use(middleware.Logger)

	return ret
}

func (s *ApiServer) AddRoute(pattern string, fn func(chi.Router)) {
	s.Route(pattern, fn)
}

func (s *ApiServer) Serve(l net.Listener) error {
	server := http.Server{
		Handler: s,
	}
	return server.Serve(l)
}

type ApiResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

func SendAPIRespWithOk(w http.ResponseWriter, body ApiResponse) error {
	w.Header().Set("Content-Type", "application/json")

	// 设置message默认值ok
	if body.Code == 0 && body.Message == "" {
		body.Message = "ok"
	}
	data, err := json.Marshal(body)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
		return err
	}
	w.WriteHeader(http.StatusOK)
	_, err = w.Write(data)

	return err
}

func ParseHttpResponse(rsp *http.Response, v any) error {
	defer rsp.Body.Close()
	if rsp.StatusCode != http.StatusOK {
		return errors.New(rsp.Status)
	}
	ar := &ApiResponse{
		Data: v,
	}
	return json.NewDecoder(rsp.Body).Decode(ar)
}
