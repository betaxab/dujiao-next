package worker

import (
	"context"
	"errors"
	"time"

	"github.com/dujiao-next/internal/config"
	"github.com/dujiao-next/internal/logger"
	"github.com/dujiao-next/internal/queue"

	"github.com/hibiken/asynq"
)

const (
	affiliateConfirmInterval = time.Minute
)

// Service 异步队列服务
type Service struct {
	name     string
	server   *asynq.Server
	mux      *asynq.ServeMux
	consumer *Consumer
}

// NewService 创建异步队列服务
func NewService(cfg *config.QueueConfig, consumer *Consumer) (*Service, error) {
	if cfg == nil || !cfg.Enabled {
		return nil, errors.New("queue disabled")
	}
	if consumer == nil {
		return nil, errors.New("consumer is nil")
	}
	opt, serverCfg := queue.BuildServerConfig(cfg)
	server := asynq.NewServer(opt, serverCfg)
	mux := asynq.NewServeMux()
	consumer.Register(mux)
	return &Service{
		name:     "worker",
		server:   server,
		mux:      mux,
		consumer: consumer,
	}, nil
}

// Name 服务名称
func (s *Service) Name() string {
	if s == nil || s.name == "" {
		return "worker"
	}
	return s.name
}

// Start 启动服务
func (s *Service) Start(ctx context.Context) error {
	if s == nil || s.server == nil || s.mux == nil {
		return errors.New("worker not initialized")
	}
	if s.consumer != nil && s.consumer.AffiliateService != nil {
		go s.runAffiliateConfirmLoop(ctx)
	}
	return s.server.Run(s.mux)
}

// Stop 停止服务
func (s *Service) Stop(ctx context.Context) error {
	if s == nil || s.server == nil {
		return nil
	}
	_ = ctx
	s.server.Shutdown()
	return nil
}

func (s *Service) runAffiliateConfirmLoop(ctx context.Context) {
	if s == nil || s.consumer == nil || s.consumer.AffiliateService == nil {
		return
	}
	runOnce := func() {
		if err := s.consumer.AffiliateService.ConfirmDueCommissions(time.Now()); err != nil {
			logger.Warnw("worker_affiliate_confirm_due_failed", "error", err)
		}
	}
	runOnce()

	ticker := time.NewTicker(affiliateConfirmInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			runOnce()
		}
	}
}
