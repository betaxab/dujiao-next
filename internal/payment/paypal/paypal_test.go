package paypal

import "testing"

func TestValidateConfig(t *testing.T) {
	cfg := &Config{
		ClientID:     "cid",
		ClientSecret: "secret",
		BaseURL:      "https://api-m.sandbox.paypal.com",
		ReturnURL:    "https://example.com/payment?order_id=1",
		CancelURL:    "https://example.com/payment?order_id=1",
	}
	if err := ValidateConfig(cfg); err != nil {
		t.Fatalf("ValidateConfig should pass, got: %v", err)
	}
}

func TestParseConfigAndNormalize(t *testing.T) {
	raw := map[string]interface{}{
		"client_id":     " cid ",
		"client_secret": " secret ",
		"base_url":      "https://api-m.sandbox.paypal.com/",
		"return_url":    "https://example.com/return",
		"cancel_url":    "https://example.com/cancel",
	}
	cfg, err := ParseConfig(raw)
	if err != nil {
		t.Fatalf("ParseConfig error: %v", err)
	}
	if cfg.ClientID != "cid" {
		t.Fatalf("client id not normalized, got: %s", cfg.ClientID)
	}
	if cfg.BaseURL != "https://api-m.sandbox.paypal.com" {
		t.Fatalf("base url not normalized, got: %s", cfg.BaseURL)
	}
	if cfg.UserAction == "" {
		t.Fatalf("user action should have default value")
	}
}

func TestToPaymentStatus(t *testing.T) {
	status, ok := ToPaymentStatus("PAYMENT.CAPTURE.COMPLETED", "")
	if !ok || status != "success" {
		t.Fatalf("expected success status for completed event, got %s %v", status, ok)
	}
	status, ok = ToPaymentStatus("", "DECLINED")
	if !ok || status != "failed" {
		t.Fatalf("expected failed status for declined resource, got %s %v", status, ok)
	}
	status, ok = ToPaymentStatus("UNKNOWN", "UNKNOWN")
	if ok || status != "" {
		t.Fatalf("expected unsupported mapping, got %s %v", status, ok)
	}
}

func TestWebhookEventHelpers(t *testing.T) {
	event := &WebhookEvent{
		EventType: "PAYMENT.CAPTURE.COMPLETED",
		Resource: map[string]interface{}{
			"supplementary_data": map[string]interface{}{
				"related_ids": map[string]interface{}{
					"order_id": "ORDER-123",
				},
			},
			"amount": map[string]interface{}{
				"value":         "10.00",
				"currency_code": "USD",
			},
			"create_time": "2026-02-09T12:00:00Z",
			"status":      "COMPLETED",
		},
	}
	if got := event.RelatedOrderID(); got != "ORDER-123" {
		t.Fatalf("unexpected order id: %s", got)
	}
	value, currency := event.CaptureAmount()
	if value != "10.00" || currency != "USD" {
		t.Fatalf("unexpected amount info: %s %s", value, currency)
	}
	if event.PaidAt() == nil {
		t.Fatalf("PaidAt should parse time")
	}
	if status := event.ResourceStatus(); status != "COMPLETED" {
		t.Fatalf("unexpected resource status: %s", status)
	}
}
