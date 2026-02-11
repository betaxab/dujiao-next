package stripe

import (
	"encoding/json"
	"testing"
	"time"
)

func TestParseAndValidateConfig(t *testing.T) {
	cfg, err := ParseConfig(map[string]interface{}{
		"secret_key":           " sk_test_123 ",
		"webhook_secret":       " whsec_123 ",
		"success_url":          "https://example.com/payment?stripe_return=1",
		"cancel_url":           "https://example.com/payment?stripe_cancel=1",
		"payment_method_types": []interface{}{"card"},
	})
	if err != nil {
		t.Fatalf("parse config failed: %v", err)
	}
	if cfg.SecretKey != "sk_test_123" {
		t.Fatalf("unexpected secret key: %s", cfg.SecretKey)
	}
	if cfg.APIBaseURL != defaultAPIBaseURL {
		t.Fatalf("unexpected default api base url: %s", cfg.APIBaseURL)
	}
	if err := ValidateConfig(cfg); err != nil {
		t.Fatalf("validate config failed: %v", err)
	}
}

func TestVerifyAndParseWebhookCheckoutCompleted(t *testing.T) {
	now := time.Unix(1760000000, 0)
	cfg := &Config{
		WebhookSecret:           "whsec_test_abc",
		WebhookToleranceSeconds: 300,
	}
	payload := map[string]interface{}{
		"id":   "evt_test_1",
		"type": "checkout.session.completed",
		"data": map[string]interface{}{
			"object": map[string]interface{}{
				"object":         "checkout.session",
				"id":             "cs_test_123",
				"payment_status": "paid",
				"currency":       "usd",
				"amount_total":   1288,
				"created":        now.Unix(),
				"metadata": map[string]interface{}{
					"payment_id": "1001",
					"order_no":   "ORDER-1001",
				},
			},
		},
	}
	body, _ := json.Marshal(payload)
	sig := computeSignature(cfg.WebhookSecret, now.Unix(), body)
	headers := map[string]string{
		"Stripe-Signature": "t=1760000000,v1=" + sig,
	}

	result, err := VerifyAndParseWebhook(cfg, headers, body, now)
	if err != nil {
		t.Fatalf("verify and parse webhook failed: %v", err)
	}
	if result.EventType != "checkout.session.completed" {
		t.Fatalf("unexpected event type: %s", result.EventType)
	}
	if result.PaymentID != 1001 {
		t.Fatalf("unexpected payment id: %d", result.PaymentID)
	}
	if result.ProviderRef != "cs_test_123" {
		t.Fatalf("unexpected provider ref: %s", result.ProviderRef)
	}
	if result.Status != "success" {
		t.Fatalf("unexpected status: %s", result.Status)
	}
	if result.Amount != "12.88" {
		t.Fatalf("unexpected amount: %s", result.Amount)
	}
}

func TestVerifyAndParseWebhookInvalidSignature(t *testing.T) {
	now := time.Unix(1760000000, 0)
	cfg := &Config{
		WebhookSecret:           "whsec_test_abc",
		WebhookToleranceSeconds: 300,
	}
	payload := map[string]interface{}{
		"id":   "evt_test_1",
		"type": "checkout.session.completed",
		"data": map[string]interface{}{
			"object": map[string]interface{}{
				"object": "checkout.session",
				"id":     "cs_test_123",
			},
		},
	}
	body, _ := json.Marshal(payload)
	headers := map[string]string{
		"Stripe-Signature": "t=1760000000,v1=invalid-signature",
	}

	_, err := VerifyAndParseWebhook(cfg, headers, body, now)
	if err == nil {
		t.Fatalf("expected verify error")
	}
}

func TestMapPaymentIntentStatus(t *testing.T) {
	if got := mapPaymentIntentStatus("succeeded"); got != "success" {
		t.Fatalf("expected success, got %s", got)
	}
	if got := mapPaymentIntentStatus("processing"); got != "pending" {
		t.Fatalf("expected pending, got %s", got)
	}
	if got := mapPaymentIntentStatus("canceled"); got != "failed" {
		t.Fatalf("expected failed, got %s", got)
	}
}
