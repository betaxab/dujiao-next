package constants

// 订单状态常量
const (
	OrderStatusPendingPayment     = "pending_payment"
	OrderStatusPaid               = "paid"
	OrderStatusFulfilling         = "fulfilling"
	OrderStatusPartiallyDelivered = "partially_delivered"
	OrderStatusDelivered          = "delivered"
	OrderStatusCompleted          = "completed"
	OrderStatusCanceled           = "canceled"
)

// 交付类型与状态常量
const (
	FulfillmentTypeAuto        = "auto"
	FulfillmentTypeManual      = "manual"
	FulfillmentStatusPending   = "pending"
	FulfillmentStatusDelivered = "delivered"
)

// 支付状态常量
const (
	PaymentStatusInitiated = "initiated"
	PaymentStatusPending   = "pending"
	PaymentStatusSuccess   = "success"
	PaymentStatusFailed    = "failed"
	PaymentStatusExpired   = "expired"
)

// 支付提供方常量
const (
	PaymentProviderOfficial = "official"
	PaymentProviderEpay     = "epay"
	PaymentProviderEpusdt   = "epusdt"
	PaymentProviderWallet   = "wallet"
)

// 支付渠道类型常量
const (
	PaymentChannelTypeWechat    = "wechat"
	PaymentChannelTypeWxpay     = "wxpay"
	PaymentChannelTypeAlipay    = "alipay"
	PaymentChannelTypePaypal    = "paypal"
	PaymentChannelTypeStripe    = "stripe"
	PaymentChannelTypeQqpay     = "qqpay"
	PaymentChannelTypeUsdt      = "usdt"
	PaymentChannelTypeUsdtTrc20 = "usdt-trc20"
	PaymentChannelTypeUsdcTrc20 = "usdc-trc20"
	PaymentChannelTypeTrx       = "trx"
	PaymentChannelTypeBalance   = "balance"
)

// 支付交互方式常量
const (
	PaymentInteractionQR       = "qr"
	PaymentInteractionRedirect = "redirect"
	PaymentInteractionWAP      = "wap"
	PaymentInteractionPage     = "page"
	PaymentInteractionBalance  = "balance"
)

// 钱包交易类型常量
const (
	WalletTxnTypeRecharge    = "recharge"
	WalletTxnTypeOrderPay    = "order_pay"
	WalletTxnTypeOrderRefund = "order_refund"
	WalletTxnTypeAdminAdjust = "admin_adjust"
	WalletTxnTypeAdminRefund = "admin_refund"
	WalletTxnTypeGiftCard    = "gift_card_redeem"
)

// 钱包交易方向常量
const (
	WalletTxnDirectionIn  = "in"
	WalletTxnDirectionOut = "out"
)

// 钱包充值状态常量
const (
	WalletRechargeStatusPending = "pending"
	WalletRechargeStatusSuccess = "success"
	WalletRechargeStatusFailed  = "failed"
	WalletRechargeStatusExpired = "expired"
)

// 易支付回调常量
const (
	EpayTradeStatusSuccess = "TRADE_SUCCESS"
	EpayCallbackSuccess    = "success"
	EpayCallbackFail       = "fail"
	EpayPayTypeQRCode      = "qrcode"
)

// 支付宝回调常量
const (
	AlipayTradeStatusSuccess      = "TRADE_SUCCESS"
	AlipayTradeStatusFinished     = "TRADE_FINISHED"
	AlipayTradeStatusClosed       = "TRADE_CLOSED"
	AlipayTradeStatusWaitBuyerPay = "WAIT_BUYER_PAY"
	AlipayCallbackSuccess         = "success"
	AlipayCallbackFail            = "fail"
)

// 文章类型常量
const (
	PostTypeBlog   = "blog"
	PostTypeNotice = "notice"
)

// 商品购买身份常量
const (
	ProductPurchaseGuest  = "guest"
	ProductPurchaseMember = "member"
)

// 商品库存状态常量
const (
	ProductStockStatusUnlimited  = "unlimited"
	ProductStockStatusInStock    = "in_stock"
	ProductStockStatusLowStock   = "low_stock"
	ProductStockStatusOutOfStock = "out_of_stock"
)

// 优惠券类型常量
const (
	CouponTypeFixed   = "fixed"
	CouponTypePercent = "percent"
)

// 活动价类型常量
const (
	PromotionTypeFixed        = "fixed"
	PromotionTypePercent      = "percent"
	PromotionTypeSpecialPrice = "special_price"
)

// 适用范围常量
const (
	ScopeTypeProduct = "product"
)

// 用户状态常量
const (
	UserStatusActive   = "active"
	UserStatusDisabled = "disabled"
)

// 第三方登录提供方常量
const (
	UserOAuthProviderTelegram = "telegram"
)

// 登录日志状态常量
const (
	LoginLogStatusSuccess = "success"
	LoginLogStatusFailed  = "failed"
)

// 登录日志失败原因常量
const (
	LoginLogFailReasonBadRequest           = "bad_request"
	LoginLogFailReasonCaptchaRequired      = "captcha_required"
	LoginLogFailReasonCaptchaInvalid       = "captcha_invalid"
	LoginLogFailReasonCaptchaConfigInvalid = "captcha_config_invalid"
	LoginLogFailReasonCaptchaVerifyFailed  = "captcha_verify_failed"
	LoginLogFailReasonInvalidEmail         = "invalid_email"
	LoginLogFailReasonInvalidCredentials   = "invalid_credentials"
	LoginLogFailReasonEmailNotVerified     = "email_not_verified"
	LoginLogFailReasonUserDisabled         = "user_disabled"
	LoginLogFailReasonTelegramInvalid      = "telegram_invalid"
	LoginLogFailReasonTelegramExpired      = "telegram_expired"
	LoginLogFailReasonTelegramReplayed     = "telegram_replayed"
	LoginLogFailReasonTelegramConfig       = "telegram_config_invalid"
	LoginLogFailReasonInternalError        = "internal_error"
)

// 登录日志来源常量
const (
	LoginLogSourceWeb      = "web"
	LoginLogSourceTelegram = "telegram"
)

// 验证码用途常量
const (
	VerifyPurposeRegister       = "register"
	VerifyPurposeReset          = "reset"
	VerifyPurposeChangeEmailOld = "change_email_old"
	VerifyPurposeChangeEmailNew = "change_email_new"
)

// 验证码提供方常量
const (
	CaptchaProviderNone      = "none"
	CaptchaProviderImage     = "image"
	CaptchaProviderTurnstile = "turnstile"
)

// 验证码校验场景常量
const (
	CaptchaSceneLogin            = "login"
	CaptchaSceneRegisterSendCode = "register_send_code"
	CaptchaSceneResetSendCode    = "reset_send_code"
	CaptchaSceneGuestCreateOrder = "guest_create_order"
	CaptchaSceneGiftCardRedeem   = "gift_card_redeem"
)

// 通知中心事件常量
const (
	NotificationEventWalletRechargeSuccess    = "wallet_recharge_success"
	NotificationEventOrderPaidSuccess         = "order_paid_success"
	NotificationEventManualFulfillmentPending = "manual_fulfillment_pending"
	NotificationEventExceptionAlert           = "exception_alert"
	NotificationEventExceptionAlertCheck      = "exception_alert_check"
)

// 通知中心异常阈值类型常量
const (
	NotificationAlertTypeOutOfStockProducts = "out_of_stock_products"
	NotificationAlertTypeLowStockProducts   = "low_stock_products"
	NotificationAlertTypePendingOrders      = "pending_payment_orders"
	NotificationAlertTypePaymentsFailed     = "payments_failed"
)

// 队列常量
const (
	QueueDefault             = "default"
	TaskOrderStatusEmail     = "order:status_email"
	TaskOrderAutoFulfill     = "order:auto_fulfill"
	TaskOrderTimeoutCancel   = "order:timeout_cancel"
	TaskNotificationDispatch = "notification:dispatch"
)

// 设置键常量
const (
	SettingKeySiteConfig               = "site_config"
	SettingKeyOrderConfig              = "order_config"
	SettingKeySMTPConfig               = "smtp_config"
	SettingKeyCaptchaConfig            = "captcha_config"
	SettingKeyTelegramAuthConfig       = "telegram_auth_config"
	SettingKeyDashboardConfig          = "dashboard_config"
	SettingKeyNotificationCenterConfig = "notification_center_config"
	SettingFieldSiteCurrency           = "currency"
	SettingFieldPaymentExpireMinutes   = "payment_expire_minutes"
)

// 币种常量
const (
	SiteCurrencyDefault = "CNY"
)

// 卡密批次来源常量
const (
	CardSecretSourceManual = "manual"
	CardSecretSourceCSV    = "csv"
)

// Banner 位置常量
const (
	BannerPositionHomeHero = "home_hero"
)

// Banner 跳转类型常量
const (
	BannerLinkTypeNone     = "none"
	BannerLinkTypeInternal = "internal"
	BannerLinkTypeExternal = "external"
)
