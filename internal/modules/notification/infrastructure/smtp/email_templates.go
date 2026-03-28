package smtp

import (
	"bytes"
	"embed"
	"html/template"
	"strings"

	"github.com/dujiao-next/internal/i18n"
)

//go:embed emailtemplates/*.html
var emailTemplateFiles embed.FS

var emailTemplates = template.Must(template.ParseFS(emailTemplateFiles, "emailtemplates/*.html"))

// emailLayoutData 邮件公共布局数据
type emailLayoutData struct {
	Lang     string
	Title    string
	SiteName string
	Content  template.HTML
}

// emailInfoData 邮件站点与支持信息数据
type emailInfoData struct {
	SiteName    string
	SiteURL     string
	SupportText string
	SiteLabel   string
	ShowInfo    bool
}

// verifyCodeEmailData 验证码邮件模板数据
type verifyCodeEmailData struct {
	emailInfoData
	Greeting     string
	Thanks       string
	Description  string
	Code         string
	ExpirePrefix string
	ExpireStrong string
	ExpireSuffix string
	IgnoreText   string
}

// orderStatusEmailData 订单状态邮件模板数据
type orderStatusEmailData struct {
	emailInfoData
	ShowSummary     bool
	Greeting        string
	Intro           string
	OrderNoLabel    string
	OrderNo         string
	StatusLabelText string
	StatusLabel     string
	AmountLabel     string
	Amount          string
	Paragraphs      []string
	DeliveryLabel   string
	FulfillmentInfo string
	GuestTipLabel   string
	GuestTip        string
}

// genericEmailData 通用邮件模板数据
type genericEmailData struct {
	Paragraphs []string
}

// renderEmbeddedEmail 渲染邮件内容模板并嵌入公共布局
func renderEmbeddedEmail(contentTemplate string, contentData any, layout emailLayoutData) string {
	var content bytes.Buffer
	if err := emailTemplates.ExecuteTemplate(&content, contentTemplate, contentData); err != nil {
		panic(err)
	}
	layout.Content = template.HTML(content.String())

	var result bytes.Buffer
	if err := emailTemplates.ExecuteTemplate(&result, "layout", layout); err != nil {
		panic(err)
	}
	return result.String()
}

// emailLangCode 将系统语言代码转换为 HTML lang 属性值
func emailLangCode(locale string) string {
	switch normalizeLocale(locale) {
	case i18n.LocaleTW:
		return "zh-TW"
	case i18n.LocaleEN:
		return "en"
	default:
		return "zh-CN"
	}
}

// splitEmailParagraphs 将纯文本按行拆分为非空邮件段落
func splitEmailParagraphs(body string) []string {
	lines := strings.Split(strings.TrimSpace(body), "\n")
	paragraphs := make([]string, 0, len(lines))
	for _, line := range lines {
		if text := strings.TrimSpace(line); text != "" {
			paragraphs = append(paragraphs, text)
		}
	}
	return paragraphs
}
