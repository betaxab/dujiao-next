package service

import (
	"errors"
	"strings"
	"testing"

	"github.com/dujiao-next/internal/i18n"
	"github.com/dujiao-next/internal/models"

	"github.com/shopspring/decimal"
)

func TestBuildOrderStatusContent(t *testing.T) {
	tests := []struct {
		name                string
		locale              string
		status              string
		payload             string
		wantSubjectContains []string
		wantBodyContains    []string
	}{
		{
			name:   "paid_zh",
			locale: i18n.LocaleZH,
			status: "paid",
			wantSubjectContains: []string{
				"订单状态更新",
				"已支付",
			},
			wantBodyContains: []string{
				"已收到您的付款",
				"订单号：DJ-PAID",
			},
		},
		{
			name:   "canceled_en",
			locale: i18n.LocaleEN,
			status: "canceled",
			wantSubjectContains: []string{
				"Order status updated",
				"Canceled",
			},
			wantBodyContains: []string{
				"The order has been canceled",
				"Order No: DJ-CANCEL",
			},
		},
		{
			name:    "delivered_with_payload_tw",
			locale:  i18n.LocaleTW,
			status:  "delivered",
			payload: "CODE-A\nCODE-B",
			wantSubjectContains: []string{
				"訂單狀態更新",
				"已交付",
			},
			wantBodyContains: []string{
				"交付內容",
				"CODE-A",
			},
		},
		{
			name:    "delivered_no_payload_en",
			locale:  i18n.LocaleEN,
			status:  "delivered",
			payload: "",
			wantSubjectContains: []string{
				"Order status updated",
				"Delivered",
			},
			wantBodyContains: []string{
				"Delivery completed",
				"Order No: DJ-DELIVER",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := OrderStatusEmailInput{
				OrderNo:         pickOrderNo(tt.status),
				Status:          tt.status,
				Amount:          models.NewMoneyFromDecimal(decimal.NewFromFloat(19.8)),
				Currency:        "USD",
				FulfillmentInfo: tt.payload,
			}
			subject, body := buildOrderStatusContent(input, tt.locale)
			for _, expected := range tt.wantSubjectContains {
				if !strings.Contains(subject, expected) {
					t.Fatalf("subject missing %q: %s", expected, subject)
				}
			}
			for _, expected := range tt.wantBodyContains {
				if !strings.Contains(body, expected) {
					t.Fatalf("body missing %q: %s", expected, body)
				}
			}
		})
	}
}

func pickOrderNo(status string) string {
	switch status {
	case "paid":
		return "DJ-PAID"
	case "canceled":
		return "DJ-CANCEL"
	default:
		return "DJ-DELIVER"
	}
}

func TestIsEmailRecipientRejected(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "smtp_550_no_such_recipient",
			err:  errors.New("550 No such recipient here"),
			want: true,
		},
		{
			name: "smtp_user_unknown",
			err:  errors.New("SMTP 5.1.1 user unknown"),
			want: true,
		},
		{
			name: "smtp_550_mailbox_unavailable",
			err:  errors.New("550 mailbox unavailable"),
			want: true,
		},
		{
			name: "network_timeout",
			err:  errors.New("dial tcp timeout"),
			want: false,
		},
		{
			name: "nil_error",
			err:  nil,
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isEmailRecipientRejected(tt.err); got != tt.want {
				t.Fatalf("isEmailRecipientRejected() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNormalizeEmailSendError(t *testing.T) {
	rejected := errors.New("550 No such recipient here")
	if got := normalizeEmailSendError(rejected); !errors.Is(got, ErrEmailRecipientRejected) {
		t.Fatalf("normalizeEmailSendError() expected ErrEmailRecipientRejected, got %v", got)
	}

	networkErr := errors.New("dial tcp timeout")
	if got := normalizeEmailSendError(networkErr); !errors.Is(got, networkErr) {
		t.Fatalf("normalizeEmailSendError() should keep original error, got %v", got)
	}

	if got := normalizeEmailSendError(nil); got != nil {
		t.Fatalf("normalizeEmailSendError(nil) should be nil, got %v", got)
	}
}
