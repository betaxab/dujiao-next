package public

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dujiao-next/internal/constants"

	"github.com/gin-gonic/gin"
)

func TestPaymentCallbackRejectUnknownPayload(t *testing.T) {
	gin.SetMode(gin.TestMode)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/payments/callback", strings.NewReader(`{"payment_id":1,"status":"success"}`))
	req.Header.Set("Content-Type", "application/json")
	c.Request = req

	h := &Handler{}
	h.PaymentCallback(c)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
	if got := strings.TrimSpace(w.Body.String()); got != constants.EpayCallbackFail {
		t.Fatalf("expected body %q, got %q", constants.EpayCallbackFail, got)
	}
}
