package service

import (
	"errors"
	"testing"

	"github.com/dujiao-next/internal/queue"
	"github.com/dujiao-next/internal/repository"
)

type orderStatusEmailOrderRepoStub struct {
	repository.OrderRepository
	receiver string
	err      error
}

func (s orderStatusEmailOrderRepoStub) ResolveReceiverEmailByOrderID(_ uint) (string, error) {
	return s.receiver, s.err
}

func TestEnqueueOrderStatusEmailTaskIfEligibleSkipTelegramPlaceholder(t *testing.T) {
	queueClient, err := queue.NewClient(nil)
	if err != nil {
		t.Fatalf("new queue client failed: %v", err)
	}
	t.Cleanup(func() {
		_ = queueClient.Close()
	})

	skipped, err := enqueueOrderStatusEmailTaskIfEligible(
		orderStatusEmailOrderRepoStub{receiver: "telegram_123@login.local"},
		queueClient,
		101,
		"paid",
	)
	if err != nil {
		t.Fatalf("enqueue helper returned error: %v", err)
	}
	if !skipped {
		t.Fatalf("expected task skipped for telegram placeholder email")
	}
}

func TestEnqueueOrderStatusEmailTaskIfEligibleSkipEmptyReceiver(t *testing.T) {
	queueClient, err := queue.NewClient(nil)
	if err != nil {
		t.Fatalf("new queue client failed: %v", err)
	}
	t.Cleanup(func() {
		_ = queueClient.Close()
	})

	skipped, err := enqueueOrderStatusEmailTaskIfEligible(
		orderStatusEmailOrderRepoStub{receiver: "   "},
		queueClient,
		102,
		"paid",
	)
	if err != nil {
		t.Fatalf("enqueue helper returned error: %v", err)
	}
	if !skipped {
		t.Fatalf("expected task skipped for empty receiver email")
	}
}

func TestEnqueueOrderStatusEmailTaskIfEligibleEnqueueNormalReceiver(t *testing.T) {
	queueClient, err := queue.NewClient(nil)
	if err != nil {
		t.Fatalf("new queue client failed: %v", err)
	}
	t.Cleanup(func() {
		_ = queueClient.Close()
	})

	skipped, err := enqueueOrderStatusEmailTaskIfEligible(
		orderStatusEmailOrderRepoStub{receiver: "buyer@example.com"},
		queueClient,
		103,
		"paid",
	)
	if err != nil {
		t.Fatalf("enqueue helper returned error: %v", err)
	}
	if skipped {
		t.Fatalf("expected task enqueued for normal receiver email")
	}
}

func TestEnqueueOrderStatusEmailTaskIfEligibleFallbackWhenLookupFailed(t *testing.T) {
	queueClient, err := queue.NewClient(nil)
	if err != nil {
		t.Fatalf("new queue client failed: %v", err)
	}
	t.Cleanup(func() {
		_ = queueClient.Close()
	})

	skipped, err := enqueueOrderStatusEmailTaskIfEligible(
		orderStatusEmailOrderRepoStub{err: errors.New("lookup failed")},
		queueClient,
		104,
		"paid",
	)
	if err != nil {
		t.Fatalf("enqueue helper returned error: %v", err)
	}
	if skipped {
		t.Fatalf("expected fallback enqueue when receiver lookup failed")
	}
}
