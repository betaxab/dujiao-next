package service

import (
	"context"

	paymentcommon "github.com/dujiao-next/internal/payment/common"
)

// detachPaymentGatewayContext 将支付网关请求从上游 HTTP 连接生命周期中解耦，
// 避免浏览器断开、页面跳转或 Bot 客户端超时直接取消下游支付创建请求。
func detachPaymentGatewayContext(parent context.Context) (context.Context, context.CancelFunc) {
	if parent == nil {
		return paymentcommon.WithDefaultTimeout(context.Background())
	}
	return paymentcommon.WithDefaultTimeout(context.WithoutCancel(parent))
}
