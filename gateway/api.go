package gateway

import (
	"encoding/json"
	"net/http"
	"sync"

	"github.com/go-chi/chi"
	"github.com/go-chi/chi/middleware"
)

type apiServer struct {
	once sync.Once
	*chi.Mux
}

func (s *apiServer) addRoute(pattern string, fn func(chi.Router)) {
	s.once.Do(func() {
		s.Mux = chi.NewRouter()
		s.Use(middleware.Logger)
	})
	s.Route(pattern, fn)
}

func (s *apiServer) serve() error {
	return http.ListenAndServe("0.0.0.0:3000", s)
}

type apiResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

func sendAPIRespWithOk(w http.ResponseWriter, body apiResponse) error {
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
