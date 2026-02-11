package queue

import (
	"fmt"
	"strings"
	"time"

	"github.com/dujiao-next/internal/config"
	"github.com/dujiao-next/internal/constants"

	"github.com/hibiken/asynq"
)

const (
	// DefaultQueue 默认队列名称
	DefaultQueue = constants.QueueDefault
)

// Client 队列客户端封装
type Client struct {
	client       *asynq.Client
	enabled      bool
	defaultQueue string
}

// NewClient 创建队列客户端
func NewClient(cfg *config.QueueConfig) (*Client, error) {
	if cfg == nil || !cfg.Enabled {
		return &Client{enabled: false, defaultQueue: DefaultQueue}, nil
	}
	opt := buildRedisOpt(cfg)
	client := asynq.NewClient(opt)
	return &Client{
		client:       client,
		enabled:      true,
		defaultQueue: DefaultQueue,
	}, nil
}

// Enabled 判断是否启用
func (c *Client) Enabled() bool {
	return c != nil && c.enabled && c.client != nil
}

// Close 关闭客户端
func (c *Client) Close() error {
	if c == nil || c.client == nil {
		return nil
	}
	return c.client.Close()
}

// EnqueueOrderStatusEmail 推送订单状态邮件任务
func (c *Client) EnqueueOrderStatusEmail(payload OrderStatusEmailPayload, opts ...asynq.Option) error {
	if !c.Enabled() {
		return nil
	}
	task, err := NewOrderStatusEmailTask(payload)
	if err != nil {
		return err
	}
	options := append([]asynq.Option{asynq.Queue(c.defaultQueue)}, opts...)
	_, err = c.client.Enqueue(task, options...)
	return err
}

// EnqueueOrderAutoFulfill 推送自动交付任务
func (c *Client) EnqueueOrderAutoFulfill(payload OrderAutoFulfillPayload, opts ...asynq.Option) error {
	if !c.Enabled() {
		return nil
	}
	task, err := NewOrderAutoFulfillTask(payload)
	if err != nil {
		return err
	}
	options := append([]asynq.Option{asynq.Queue(c.defaultQueue)}, opts...)
	_, err = c.client.Enqueue(task, options...)
	return err
}

// EnqueueOrderTimeoutCancel 推送订单超时取消任务
func (c *Client) EnqueueOrderTimeoutCancel(payload OrderTimeoutCancelPayload, delay time.Duration) error {
	if !c.Enabled() {
		return nil
	}
	if delay < 0 {
		delay = 0
	}
	task, err := NewOrderTimeoutCancelTask(payload)
	if err != nil {
		return err
	}
	options := []asynq.Option{asynq.Queue(c.defaultQueue), asynq.ProcessIn(delay)}
	_, err = c.client.Enqueue(task, options...)
	return err
}

// BuildServerConfig 生成队列服务配置
func BuildServerConfig(cfg *config.QueueConfig) (asynq.RedisClientOpt, asynq.Config) {
	opt := buildRedisOpt(cfg)
	concurrency := 10
	if cfg != nil && cfg.Concurrency > 0 {
		concurrency = cfg.Concurrency
	}
	queues := map[string]int{DefaultQueue: 1}
	if cfg != nil && len(cfg.Queues) > 0 {
		queues = cfg.Queues
	}
	return opt, asynq.Config{
		Concurrency: concurrency,
		Queues:      queues,
	}
}

func buildRedisOpt(cfg *config.QueueConfig) asynq.RedisClientOpt {
	host := "127.0.0.1"
	port := 6379
	password := ""
	db := 0
	if cfg != nil {
		if strings.TrimSpace(cfg.Host) != "" {
			host = strings.TrimSpace(cfg.Host)
		}
		if cfg.Port > 0 {
			port = cfg.Port
		}
		password = cfg.Password
		db = cfg.DB
	}
	return asynq.RedisClientOpt{
		Addr:     fmt.Sprintf("%s:%d", host, port),
		Password: password,
		DB:       db,
	}
}
