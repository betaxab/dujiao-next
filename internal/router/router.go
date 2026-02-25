package router

import (
	"fmt"
	"sort"
	"strings"

	"github.com/dujiao-next/internal/authz"
	"github.com/dujiao-next/internal/cache"
	"github.com/dujiao-next/internal/config"
	adminhandlers "github.com/dujiao-next/internal/http/handlers/admin"
	publichandlers "github.com/dujiao-next/internal/http/handlers/public"
	"github.com/dujiao-next/internal/http/response"
	"github.com/dujiao-next/internal/logger"
	"github.com/dujiao-next/internal/provider"

	"github.com/gin-gonic/gin"
)

// SetupRouter 初始化路由
func SetupRouter(cfg *config.Config, c *provider.Container) *gin.Engine {
	log := logger.L
	if log == nil {
		log = logger.Init(cfg.Server.Mode, cfg.Log.ToLoggerOptions())
	}
	r := gin.New()

	// 初始化 Handler（按前台/后台分组）
	publicHandler := publichandlers.New(c)
	adminHandler := adminhandlers.New(c)
	redisPrefix := strings.TrimSpace(cfg.Redis.Prefix)
	if redisPrefix == "" {
		redisPrefix = "dj"
	}
	redisClient := cache.Client()
	loginRule := RateLimitRule{
		Prefix:        fmt.Sprintf("%s:rate:login", redisPrefix),
		WindowSeconds: cfg.Security.LoginRateLimit.WindowSeconds,
		MaxRequests:   cfg.Security.LoginRateLimit.MaxAttempts,
		BlockSeconds:  cfg.Security.LoginRateLimit.BlockSeconds,
		MessageKey:    "error.login_too_many",
	}
	adminLoginRule := RateLimitRule{
		Prefix:        fmt.Sprintf("%s:rate:admin_login", redisPrefix),
		WindowSeconds: cfg.Security.LoginRateLimit.WindowSeconds,
		MaxRequests:   cfg.Security.LoginRateLimit.MaxAttempts,
		BlockSeconds:  cfg.Security.LoginRateLimit.BlockSeconds,
		MessageKey:    "error.login_too_many",
	}

	// 中间件
	r.Use(gin.Recovery())
	r.Use(RequestIDMiddleware())
	r.Use(LoggerMiddleware(log))
	r.Use(CORSMiddleware(cfg.CORS))

	// 静态文件服务（上传的图片）- 必须放在最前面
	r.Static("/uploads", "./uploads")

	// API 路由组
	apiV1 := r.Group("/api/v1")
	{
		// 公开接口
		public := apiV1.Group("/public")
		{
			public.GET("/config", publicHandler.GetConfig)
			public.GET("/products", publicHandler.GetProducts)
			public.GET("/products/:slug", publicHandler.GetProductBySlug)
			public.GET("/posts", publicHandler.GetPosts)
			public.GET("/posts/:slug", publicHandler.GetPostBySlug)
			public.GET("/banners", publicHandler.GetPublicBanners)
			public.GET("/categories", publicHandler.GetCategories)
			public.GET("/captcha/image", publicHandler.GetImageCaptcha)
		}

		// 游客接口
		guest := apiV1.Group("/guest")
		{
			guest.POST("/orders", publicHandler.CreateGuestOrder)
			guest.POST("/orders/preview", publicHandler.PreviewGuestOrder)
			guest.GET("/orders", publicHandler.ListGuestOrders)
			guest.GET("/orders/:id", publicHandler.GetGuestOrder)
			guest.GET("/orders/by-order-no/:order_no", publicHandler.GetGuestOrderByOrderNo)
			guest.POST("/payments", publicHandler.CreateGuestPayment)
			guest.POST("/payments/:id/capture", publicHandler.CaptureGuestPayment)
			guest.GET("/payments/latest", publicHandler.GetGuestLatestPayment)
		}

		// 用户认证接口
		auth := apiV1.Group("/auth")
		{
			auth.POST("/send-verify-code", publicHandler.SendUserVerifyCode)
			auth.POST("/register", publicHandler.UserRegister)
			auth.POST("/login", RateLimitMiddleware(redisClient, loginRule, KeyByIPAndJSONField("email")), publicHandler.UserLogin)
			auth.POST("/telegram/login", RateLimitMiddleware(redisClient, loginRule, KeyByIP), publicHandler.UserTelegramLogin)
			auth.POST("/forgot-password", publicHandler.UserForgotPassword)
		}

		// 用户接口（需鉴权）
		user := apiV1.Group("")
		user.Use(UserJWTAuthMiddleware(cfg.UserJWT.SecretKey, c.UserRepo))
		{
			user.GET("/me", publicHandler.GetCurrentUser)
			user.GET("/me/login-logs", publicHandler.GetMyLoginLogs)
			user.PUT("/me/profile", publicHandler.UpdateUserProfile)
			user.PUT("/me/password", publicHandler.ChangeUserPassword)
			user.GET("/me/telegram", publicHandler.GetMyTelegramBinding)
			user.POST("/me/telegram/bind", publicHandler.BindMyTelegram)
			user.DELETE("/me/telegram/unbind", publicHandler.UnbindMyTelegram)
			user.POST("/me/email/send-verify-code", publicHandler.SendChangeEmailCode)
			user.POST("/me/email/change", publicHandler.ChangeEmail)
			user.GET("/cart", publicHandler.GetCart)
			user.POST("/cart/items", publicHandler.UpsertCartItem)
			user.DELETE("/cart/items/:product_id", publicHandler.DeleteCartItem)
			user.POST("/orders", publicHandler.CreateOrder)
			user.POST("/orders/preview", publicHandler.PreviewOrder)
			user.GET("/orders", publicHandler.ListOrders)
			user.GET("/orders/:id", publicHandler.GetOrder)
			user.GET("/orders/by-order-no/:order_no", publicHandler.GetOrderByOrderNo)
			user.POST("/orders/:id/cancel", publicHandler.CancelOrder)
			user.POST("/payments", publicHandler.CreatePayment)
			user.POST("/payments/:id/capture", publicHandler.CapturePayment)
			user.GET("/payments/latest", publicHandler.GetLatestPayment)
			user.GET("/wallet", publicHandler.GetMyWallet)
			user.GET("/wallet/transactions", publicHandler.GetMyWalletTransactions)
			user.POST("/wallet/recharge", publicHandler.RechargeWallet)
			user.GET("/wallet/recharges/:recharge_no", publicHandler.GetMyWalletRecharge)
			user.POST("/wallet/recharge/payments/:id/capture", publicHandler.CaptureMyWalletRechargePayment)
			user.POST("/gift-cards/redeem", publicHandler.RedeemGiftCard)
		}

		apiV1.POST("/payments/callback", publicHandler.PaymentCallback)
		apiV1.GET("/payments/callback", publicHandler.PaymentCallback)
		apiV1.POST("/payments/webhook/paypal", publicHandler.PaypalWebhook)
		apiV1.POST("/payments/webhook/stripe", publicHandler.StripeWebhook)

		// 管理员接口
		admin := apiV1.Group("/admin")
		{
			// 登录接口（无需鉴权）
			admin.POST("/login", RateLimitMiddleware(redisClient, adminLoginRule, KeyByIP), adminHandler.AdminLogin)

			// 需要鉴权的接口
			authorized := admin.Use(JWTAuthMiddleware(cfg.JWT.SecretKey, c.AdminRepo), AdminRBACMiddleware(c.AuthzService))
			{
				// 仪表盘
				authorized.GET("/dashboard/overview", adminHandler.GetDashboardOverview)
				authorized.GET("/dashboard/trends", adminHandler.GetDashboardTrends)
				authorized.GET("/dashboard/rankings", adminHandler.GetDashboardRankings)

				// 商品管理
				authorized.GET("/products", adminHandler.GetAdminProducts)
				authorized.GET("/products/:id", adminHandler.GetAdminProduct)
				authorized.POST("/products", adminHandler.CreateProduct)
				authorized.PUT("/products/:id", adminHandler.UpdateProduct)
				authorized.DELETE("/products/:id", adminHandler.DeleteProduct)

				// 文章管理
				authorized.GET("/posts", adminHandler.GetAdminPosts)
				authorized.POST("/posts", adminHandler.CreatePost)
				authorized.PUT("/posts/:id", adminHandler.UpdatePost)
				authorized.DELETE("/posts/:id", adminHandler.DeletePost)

				// Banner 管理
				authorized.GET("/banners", adminHandler.GetAdminBanners)
				authorized.GET("/banners/:id", adminHandler.GetAdminBanner)
				authorized.POST("/banners", adminHandler.CreateBanner)
				authorized.PUT("/banners/:id", adminHandler.UpdateBanner)
				authorized.DELETE("/banners/:id", adminHandler.DeleteBanner)

				// 分类管理
				authorized.GET("/categories", adminHandler.GetAdminCategories)
				authorized.POST("/categories", adminHandler.CreateCategory)
				authorized.PUT("/categories/:id", adminHandler.UpdateCategory)
				authorized.DELETE("/categories/:id", adminHandler.DeleteCategory)

				// 设置管理
				authorized.GET("/settings", adminHandler.GetSettings)
				authorized.PUT("/settings", adminHandler.UpdateSettings)
				authorized.GET("/settings/smtp", adminHandler.GetSMTPSettings)
				authorized.PUT("/settings/smtp", adminHandler.UpdateSMTPSettings)
				authorized.POST("/settings/smtp/test", adminHandler.TestSMTPSettings)
				authorized.GET("/settings/captcha", adminHandler.GetCaptchaSettings)
				authorized.PUT("/settings/captcha", adminHandler.UpdateCaptchaSettings)
				authorized.GET("/settings/telegram-auth", adminHandler.GetTelegramAuthSettings)
				authorized.PUT("/settings/telegram-auth", adminHandler.UpdateTelegramAuthSettings)
				authorized.PUT("/password", adminHandler.UpdateAdminPassword) // 修改密码

				// 权限管理
				authorized.GET("/authz/me", adminHandler.GetAuthzMe)
				authorized.GET("/authz/roles", adminHandler.ListAuthzRoles)
				authorized.GET("/authz/admins", adminHandler.ListAuthzAdmins)
				authorized.GET("/authz/audit-logs", adminHandler.ListAuthzAuditLogs)
				authorized.POST("/authz/admins", adminHandler.CreateAuthzAdmin)
				authorized.PUT("/authz/admins/:id", adminHandler.UpdateAuthzAdmin)
				authorized.DELETE("/authz/admins/:id", adminHandler.DeleteAuthzAdmin)
				authorized.GET("/authz/permissions/catalog", func(ctx *gin.Context) {
					response.Success(ctx, buildAdminPermissionCatalog(r))
				})
				authorized.POST("/authz/roles", adminHandler.CreateAuthzRole)
				authorized.DELETE("/authz/roles/:role", adminHandler.DeleteAuthzRole)
				authorized.GET("/authz/roles/:role/policies", adminHandler.GetAuthzRolePolicies)
				authorized.POST("/authz/policies", adminHandler.GrantAuthzPolicy)
				authorized.DELETE("/authz/policies", adminHandler.RevokeAuthzPolicy)
				authorized.GET("/authz/admins/:id/roles", adminHandler.GetAuthzAdminRoles)
				authorized.PUT("/authz/admins/:id/roles", adminHandler.SetAuthzAdminRoles)

				// 文件上传
				authorized.POST("/upload", adminHandler.UploadFile)

				// 订单管理
				authorized.GET("/orders", adminHandler.AdminListOrders)
				authorized.GET("/orders/:id", adminHandler.AdminGetOrder)
				authorized.PATCH("/orders/:id", adminHandler.AdminUpdateOrderStatus)
				authorized.POST("/orders/:id/refund-to-wallet", adminHandler.AdminRefundOrderToWallet)
				authorized.POST("/fulfillments", adminHandler.AdminCreateFulfillment)
				authorized.POST("/card-secrets/batch", adminHandler.CreateCardSecretBatch)
				authorized.POST("/card-secrets/import", adminHandler.ImportCardSecretCSV)
				authorized.GET("/card-secrets", adminHandler.GetCardSecrets)
				authorized.PUT("/card-secrets/:id", adminHandler.UpdateCardSecret)
				authorized.PATCH("/card-secrets/batch-status", adminHandler.BatchUpdateCardSecretStatus)
				authorized.POST("/card-secrets/batch-delete", adminHandler.BatchDeleteCardSecrets)
				authorized.POST("/card-secrets/export", adminHandler.ExportCardSecrets)
				authorized.GET("/card-secrets/stats", adminHandler.GetCardSecretStats)
				authorized.GET("/card-secrets/batches", adminHandler.GetCardSecretBatches)
				authorized.GET("/card-secrets/template", adminHandler.GetCardSecretTemplate)
				authorized.POST("/gift-cards/generate", adminHandler.GenerateGiftCards)
				authorized.GET("/gift-cards", adminHandler.GetGiftCards)
				authorized.PUT("/gift-cards/:id", adminHandler.UpdateGiftCard)
				authorized.DELETE("/gift-cards/:id", adminHandler.DeleteGiftCard)
				authorized.PATCH("/gift-cards/batch-status", adminHandler.BatchUpdateGiftCardStatus)
				authorized.POST("/gift-cards/export", adminHandler.ExportGiftCards)

				// 优惠券与活动价
				authorized.POST("/coupons", adminHandler.CreateCoupon)
				authorized.GET("/coupons", adminHandler.GetAdminCoupons)
				authorized.PUT("/coupons/:id", adminHandler.UpdateCoupon)
				authorized.DELETE("/coupons/:id", adminHandler.DeleteCoupon)
				authorized.POST("/promotions", adminHandler.CreatePromotion)
				authorized.GET("/promotions", adminHandler.GetAdminPromotions)
				authorized.PUT("/promotions/:id", adminHandler.UpdatePromotion)
				authorized.DELETE("/promotions/:id", adminHandler.DeletePromotion)

				// 支付渠道与支付记录
				authorized.POST("/payment-channels", adminHandler.CreatePaymentChannel)
				authorized.GET("/payment-channels", adminHandler.GetPaymentChannels)
				authorized.GET("/payment-channels/:id", adminHandler.GetPaymentChannel)
				authorized.PUT("/payment-channels/:id", adminHandler.UpdatePaymentChannel)
				authorized.DELETE("/payment-channels/:id", adminHandler.DeletePaymentChannel)
				authorized.GET("/payments", adminHandler.GetAdminPayments)
				authorized.GET("/payments/export", adminHandler.ExportAdminPayments)
				authorized.GET("/payments/:id", adminHandler.GetAdminPayment)

				// 用户管理
				authorized.GET("/users", adminHandler.GetAdminUsers)
				authorized.GET("/user-login-logs", adminHandler.GetUserLoginLogs)
				authorized.PUT("/users/batch-status", adminHandler.BatchUpdateUserStatus)
				authorized.GET("/users/:id", adminHandler.GetAdminUser)
				authorized.PUT("/users/:id", adminHandler.UpdateAdminUser)
				authorized.GET("/users/:id/coupon-usages", adminHandler.GetAdminUserCouponUsages)
				authorized.GET("/users/:id/wallet", adminHandler.GetAdminUserWallet)
				authorized.GET("/users/:id/wallet/transactions", adminHandler.GetAdminUserWalletTransactions)
				authorized.POST("/users/:id/wallet/adjust", adminHandler.AdjustAdminUserWallet)
			}
		}
	}

	// 健康检查
	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	return r
}

type adminPermissionCatalogItem struct {
	Module     string `json:"module"`
	Method     string `json:"method"`
	Object     string `json:"object"`
	Permission string `json:"permission"`
}

func buildAdminPermissionCatalog(engine *gin.Engine) []adminPermissionCatalogItem {
	if engine == nil {
		return []adminPermissionCatalogItem{}
	}

	routes := engine.Routes()
	seen := make(map[string]struct{}, len(routes))
	items := make([]adminPermissionCatalogItem, 0, len(routes))

	for _, item := range routes {
		method := strings.ToUpper(strings.TrimSpace(item.Method))
		if method == "" || method == "OPTIONS" || method == "HEAD" {
			continue
		}
		if !strings.HasPrefix(item.Path, "/api/v1/admin/") {
			continue
		}
		if item.Path == "/api/v1/admin/login" {
			continue
		}
		object := authz.NormalizeObject(item.Path)
		permission := method + ":" + object
		if _, exists := seen[permission]; exists {
			continue
		}
		seen[permission] = struct{}{}
		items = append(items, adminPermissionCatalogItem{
			Module:     deriveAdminPermissionModule(object),
			Method:     method,
			Object:     object,
			Permission: permission,
		})
	}

	sort.Slice(items, func(i, j int) bool {
		if items[i].Module == items[j].Module {
			if items[i].Object == items[j].Object {
				return items[i].Method < items[j].Method
			}
			return items[i].Object < items[j].Object
		}
		return items[i].Module < items[j].Module
	})

	return items
}

func deriveAdminPermissionModule(object string) string {
	normalized := strings.TrimPrefix(strings.TrimSpace(object), "/")
	if normalized == "" {
		return "system"
	}
	segments := strings.Split(normalized, "/")
	if len(segments) <= 1 {
		return segments[0]
	}
	if segments[0] != "admin" {
		return segments[0]
	}
	if segments[1] == "authz" {
		return "authz"
	}
	return segments[1]
}
