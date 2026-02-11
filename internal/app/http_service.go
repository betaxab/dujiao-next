package app

import (
	"context"
	"errors"
	"net/http"
)

// HTTPService HTTP 服务封装
type HTTPService struct {
	name   string
	server *http.Server
}

// NewHTTPService 创建 HTTP 服务
func NewHTTPService(addr string, handler http.Handler) *HTTPService {
	return &HTTPService{
		name: "http",
		server: &http.Server{
			Addr:    addr,
			Handler: handler,
		},
	}
}

// Name 服务名称
func (s *HTTPService) Name() string {
	if s == nil || s.name == "" {
		return "http"
	}
	return s.name
}

// Start 启动服务
func (s *HTTPService) Start(ctx context.Context) error {
	if s == nil || s.server == nil {
		return errors.New("http server not initialized")
	}
	if err := s.server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// Stop 停止服务
func (s *HTTPService) Stop(ctx context.Context) error {
	if s == nil || s.server == nil {
		return nil
	}
	return s.server.Shutdown(ctx)
}
