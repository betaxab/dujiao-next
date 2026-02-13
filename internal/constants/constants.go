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
)

// 支付渠道类型常量
const (
	PaymentChannelTypeWechat   = "wechat"
	PaymentChannelTypeWxpay    = "wxpay"
	PaymentChannelTypeAlipay   = "alipay"
	PaymentChannelTypePaypal   = "paypal"
	PaymentChannelTypeStripe   = "stripe"
	PaymentChannelTypeQqpay    = "qqpay"
	PaymentChannelTypeUsdt     = "usdt"
	PaymentChannelTypeUsdtTrc20 = "usdt-trc20"
	PaymentChannelTypeUsdcTrc20 = "usdc-trc20"
	PaymentChannelTypeTrx      = "trx"
)

// 支付交互方式常量
const (
	PaymentInteractionQR       = "qr"
	PaymentInteractionRedirect = "redirect"
	PaymentInteractionWAP      = "wap"
	PaymentInteractionPage     = "page"
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
	LoginLogFailReasonInternalError        = "internal_error"
)

// 登录日志来源常量
const (
	LoginLogSourceWeb = "web"
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
)

// 队列常量
const (
	QueueDefault           = "default"
	TaskOrderStatusEmail   = "order:status_email"
	TaskOrderAutoFulfill   = "order:auto_fulfill"
	TaskOrderTimeoutCancel = "order:timeout_cancel"
)

// 设置键常量
const (
	SettingKeySiteConfig             = "site_config"
	SettingKeyOrderConfig            = "order_config"
	SettingKeySMTPConfig             = "smtp_config"
	SettingKeyCaptchaConfig          = "captcha_config"
	SettingKeyDashboardConfig        = "dashboard_config"
	SettingFieldPaymentExpireMinutes = "payment_expire_minutes"
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
