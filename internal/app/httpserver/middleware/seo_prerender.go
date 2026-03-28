package middleware

import (
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"strings"

	productapp "github.com/dujiao-next/internal/modules/catalog/product/application"
	productdomain "github.com/dujiao-next/internal/modules/catalog/product/domain"
	contentapp "github.com/dujiao-next/internal/modules/content/application"
	contentdomain "github.com/dujiao-next/internal/modules/content/domain"
	"github.com/dujiao-next/internal/shared/jsonmap"

	"github.com/gin-gonic/gin"
)

// crawlerPatterns 已知爬虫 User-Agent 关键词（全小写）
var crawlerPatterns = []string{
	"googlebot",
	"baiduspider",
	"bingbot",
	"slurp",
	"duckduckbot",
	"yandexbot",
	"sogou",
	"360spider",
	"bytespider",
	"facebookexternalhit",
	"facebot",
	"twitterbot",
	"linkedinbot",
	"whatsapp",
	"telegrambot",
	"applebot",
	"semrushbot",
	"ahrefsbot",
	"mj12bot",
	"pinterestbot",
	"discordbot",
	// AI 爬虫
	"gptbot",
	"chatgpt-user",
	"claudebot",
	"claude-web",
	"google-extended",
	"petalbot",
	"ccbot",
	"perplexitybot",
	"cohere-ai",
}

// isCrawler 判断 User-Agent 是否是已知爬虫
func isCrawler(userAgent string) bool {
	if userAgent == "" {
		return false
	}
	lower := strings.ToLower(userAgent)
	for _, pattern := range crawlerPatterns {
		if strings.Contains(lower, pattern) {
			return true
		}
	}
	return false
}

// seoLocales 本地化回退顺序
var seoLocales = []string{"zh-CN", "en-US", "zh-TW"}

// extractLocalizedText 从多语言 JSON 字段提取文本
func extractLocalizedText(j jsonmap.JSON, preferredLocale string) string {
	if j == nil || len(j) == 0 {
		return ""
	}

	if preferredLocale != "" {
		if v, ok := j[preferredLocale]; ok {
			if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
				return strings.TrimSpace(s)
			}
		}
	}

	for _, locale := range seoLocales {
		if v, ok := j[locale]; ok {
			if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
				return strings.TrimSpace(s)
			}
		}
	}

	for _, v := range j {
		if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
			return strings.TrimSpace(s)
		}
	}
	return ""
}

// extractSeoMetaField 从 SeoMetaJSON 提取单个字段（支持字符串或多语言 map）
func extractSeoMetaField(seoMeta jsonmap.JSON, field, locale string) string {
	if seoMeta == nil {
		return ""
	}
	raw, ok := seoMeta[field]
	if !ok {
		return ""
	}
	switch v := raw.(type) {
	case string:
		return strings.TrimSpace(v)
	case map[string]interface{}:
		return extractLocalizedText(jsonmap.JSON(v), locale)
	}
	return ""
}

// resolvePublicHost 从请求中获取公网主机名
func resolvePublicHost(req *http.Request) string {
	if req == nil {
		return ""
	}
	if fwdHost := strings.TrimSpace(req.Header.Get("X-Forwarded-Host")); fwdHost != "" {
		return fwdHost
	}
	return req.Host
}

// resolvePublicScheme 从请求中获取公网协议
func resolvePublicScheme(req *http.Request) string {
	if req == nil {
		return "https"
	}
	if fwdProto := strings.TrimSpace(req.Header.Get("X-Forwarded-Proto")); fwdProto != "" {
		return fwdProto
	}
	if req.TLS != nil {
		return "https"
	}
	return "https"
}

// resolvePublicBaseURL 拼接基础 URL
func resolvePublicBaseURL(req *http.Request) string {
	return resolvePublicScheme(req) + "://" + resolvePublicHost(req)
}

// resolveAbsoluteURL 将相对路径转换为绝对 URL
func resolveAbsoluteURL(path string, req *http.Request) string {
	if path == "" || req == nil {
		return path
	}
	if strings.HasPrefix(path, "http") {
		return path
	}
	return resolvePublicBaseURL(req) + path
}

// seoPage 丰富的 SEO 页面数据
type seoPage struct {
	Title        string
	Description  string
	CanonicalURL string
	OGType       string
	OGImage      string
	Content      string // 正文 HTML
	Category     string // 分类名
	Tags         []string
	ProductsURL  string // 返回商品列表链接
	JSONLD       string // JSON-LD 结构化数据
}

// renderSEOPage 生成丰富的 SEO HTML 页面
func renderSEOPage(p seoPage) string {
	esc := html.EscapeString
	var b strings.Builder
	b.Grow(4096)

	b.WriteString("<!doctype html>\n<html lang=\"zh\">\n<head>\n")
	b.WriteString("<meta charset=\"UTF-8\" />\n")
	b.WriteString("<meta name=\"viewport\" content=\"width=device-width, initial-scale=1.0\" />\n")

	// Title
	if p.Title != "" {
		b.WriteString(fmt.Sprintf("<title>%s</title>\n", esc(p.Title)))
	}
	// Description
	if p.Description != "" {
		b.WriteString(fmt.Sprintf("<meta name=\"description\" content=\"%s\" />\n", esc(p.Description)))
	}
	// Canonical
	if p.CanonicalURL != "" {
		b.WriteString(fmt.Sprintf("<link rel=\"canonical\" href=\"%s\" />\n", esc(p.CanonicalURL)))
	}

	// Open Graph
	ogType := p.OGType
	if ogType == "" {
		ogType = "website"
	}
	b.WriteString(fmt.Sprintf("<meta property=\"og:type\" content=\"%s\" />\n", esc(ogType)))
	if p.Title != "" {
		b.WriteString(fmt.Sprintf("<meta property=\"og:title\" content=\"%s\" />\n", esc(p.Title)))
	}
	if p.Description != "" {
		b.WriteString(fmt.Sprintf("<meta property=\"og:description\" content=\"%s\" />\n", esc(p.Description)))
	}
	if p.OGImage != "" {
		b.WriteString(fmt.Sprintf("<meta property=\"og:image\" content=\"%s\" />\n", esc(p.OGImage)))
	}
	if p.CanonicalURL != "" {
		b.WriteString(fmt.Sprintf("<meta property=\"og:url\" content=\"%s\" />\n", esc(p.CanonicalURL)))
	}

	// Twitter Card
	b.WriteString("<meta name=\"twitter:card\" content=\"summary_large_image\" />\n")
	if p.Title != "" {
		b.WriteString(fmt.Sprintf("<meta name=\"twitter:title\" content=\"%s\" />\n", esc(p.Title)))
	}
	if p.Description != "" {
		b.WriteString(fmt.Sprintf("<meta name=\"twitter:description\" content=\"%s\" />\n", esc(p.Description)))
	}
	if p.OGImage != "" {
		b.WriteString(fmt.Sprintf("<meta name=\"twitter:image\" content=\"%s\" />\n", esc(p.OGImage)))
	}

	// JSON-LD 结构化数据
	if p.JSONLD != "" {
		b.WriteString("<script type=\"application/ld+json\">\n")
		b.WriteString(p.JSONLD)
		b.WriteString("\n</script>\n")
	}

	b.WriteString("</head>\n<body>\n")

	// 导航 / 内链
	if p.ProductsURL != "" {
		b.WriteString("<nav>\n")
		b.WriteString(fmt.Sprintf("<a href=\"%s\">首页</a>", esc(strings.TrimSuffix(p.ProductsURL, "/products"))))
		b.WriteString(fmt.Sprintf(" / <a href=\"%s\">商品列表</a>", esc(p.ProductsURL)))
		if p.Category != "" {
			b.WriteString(fmt.Sprintf(" / <span>%s</span>", esc(p.Category)))
		}
		b.WriteString("\n</nav>\n")
	}

	// 主标题
	if p.Title != "" {
		b.WriteString(fmt.Sprintf("<h1>%s</h1>\n", esc(p.Title)))
	}

	// 分类 & 标签
	if p.Category != "" {
		b.WriteString(fmt.Sprintf("<p>分类：%s</p>\n", esc(p.Category)))
	}
	if len(p.Tags) > 0 {
		var escapedTags []string
		for _, tag := range p.Tags {
			escapedTags = append(escapedTags, esc(tag))
		}
		b.WriteString(fmt.Sprintf("<p>标签：%s</p>\n", strings.Join(escapedTags, "、")))
	}

	// 商品描述
	if p.Description != "" {
		b.WriteString("<h2>商品简介</h2>\n")
		b.WriteString(fmt.Sprintf("<p>%s</p>\n", esc(p.Description)))
	}

	// 正文内容（已是 HTML，直接输出）
	if p.Content != "" {
		b.WriteString("<article>\n<h2>商品详情</h2>\n")
		b.WriteString(p.Content)
		b.WriteString("\n</article>\n")
	}

	// OG 图片作为可见图片
	if p.OGImage != "" {
		b.WriteString(fmt.Sprintf("<img src=\"%s\" alt=\"%s\" />\n", esc(p.OGImage), esc(p.Title)))
	}

	b.WriteString("</body>\n</html>")
	return b.String()
}

// ---------- Product Handler ----------

// SEOProductHandler 商品页 SEO 预渲染
func SEOProductHandler(productService *productapp.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !isCrawler(c.GetHeader("User-Agent")) {
			c.Status(http.StatusNotFound)
			c.Abort()
			return
		}

		slug := c.Param("slug")
		if slug == "" {
			c.Status(http.StatusNotFound)
			c.Abort()
			return
		}

		product, err := productService.GetPublicBySlug(slug)
		if err != nil || product == nil {
			c.Status(http.StatusNotFound)
			c.Abort()
			return
		}

		page := buildProductSEOPage(product, c.Request)
		c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(renderSEOPage(page)))
		c.Abort()
	}
}

// buildProductSEOPage 从商品构建完整 SEO 页面数据
func buildProductSEOPage(product *productdomain.Product, req *http.Request) seoPage {
	locale := "zh-CN"
	p := seoPage{
		OGType: "product",
	}

	// 标题：优先 seo_meta.title → 商品标题
	seoTitle := extractSeoMetaField(product.SeoMetaJSON, "title", locale)
	if seoTitle != "" {
		p.Title = seoTitle
	} else {
		p.Title = extractLocalizedText(product.TitleJSON, locale)
	}

	// 描述：优先 seo_meta.description → 商品描述
	seoDesc := extractSeoMetaField(product.SeoMetaJSON, "description", locale)
	if seoDesc != "" {
		p.Description = seoDesc
	} else {
		p.Description = extractLocalizedText(product.DescriptionJSON, locale)
	}

	// Canonical URL
	if req != nil {
		p.CanonicalURL = resolvePublicBaseURL(req) + "/products/" + product.Slug
		p.ProductsURL = resolvePublicBaseURL(req) + "/products"
	}

	// 图片
	if len(product.Images) > 0 {
		img := strings.TrimSpace(product.Images[0])
		if img != "" {
			p.OGImage = resolveAbsoluteURL(img, req)
		}
	}

	// 正文内容
	p.Content = extractLocalizedText(product.ContentJSON, locale)

	// 分类
	if product.Category.NameJSON != nil {
		p.Category = extractLocalizedText(product.Category.NameJSON, locale)
	}

	// 标签
	if len(product.Tags) > 0 {
		p.Tags = []string(product.Tags)
	}

	// JSON-LD Product 结构化数据
	p.JSONLD = buildProductJSONLD(product, p, req)

	return p
}

// buildProductJSONLD 生成 Product JSON-LD 结构化数据
func buildProductJSONLD(product *productdomain.Product, p seoPage, req *http.Request) string {
	ld := map[string]interface{}{
		"@context": "https://schema.org",
		"@type":    "Product",
		"name":     p.Title,
		"url":      p.CanonicalURL,
	}

	if p.Description != "" {
		ld["description"] = p.Description
	}
	if p.OGImage != "" {
		ld["image"] = p.OGImage
	}
	if p.Category != "" {
		ld["category"] = p.Category
	}

	// 价格
	priceStr := product.PriceAmount.String()
	if priceStr != "" && priceStr != "0" && priceStr != "0.00" {
		ld["offers"] = map[string]interface{}{
			"@type":         "Offer",
			"price":         priceStr,
			"priceCurrency": "CNY",
			"availability":  "https://schema.org/InStock",
			"url":           p.CanonicalURL,
		}
	}

	data, err := json.Marshal(ld)
	if err != nil {
		return ""
	}
	return string(data)
}

// ---------- Blog Handler ----------

// SEOBlogHandler 博客页 SEO 预渲染
func SEOBlogHandler(postService *contentapp.PostService) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !isCrawler(c.GetHeader("User-Agent")) {
			c.Status(http.StatusNotFound)
			c.Abort()
			return
		}

		slug := c.Param("slug")
		if slug == "" {
			c.Status(http.StatusNotFound)
			c.Abort()
			return
		}

		post, err := postService.GetPublicBySlug(c.Request.Context(), slug)
		if err != nil || post == nil {
			c.Status(http.StatusNotFound)
			c.Abort()
			return
		}

		page := buildPostSEOPage(post, c.Request)
		c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(renderSEOPage(page)))
		c.Abort()
	}
}

// buildPostSEOPage 从文章构建完整 SEO 页面数据
func buildPostSEOPage(post *contentdomain.Post, req *http.Request) seoPage {
	locale := "zh-CN"
	p := seoPage{
		OGType: "article",
	}

	p.Title = extractLocalizedText(post.TitleJSON, locale)
	p.Description = extractLocalizedText(post.SummaryJSON, locale)
	p.Content = extractLocalizedText(post.ContentJSON, locale)

	if req != nil {
		p.CanonicalURL = resolvePublicBaseURL(req) + "/blog/" + post.Slug
	}

	if thumb := strings.TrimSpace(post.Thumbnail); thumb != "" {
		p.OGImage = resolveAbsoluteURL(thumb, req)
	}

	// JSON-LD Article 结构化数据
	p.JSONLD = buildArticleJSONLD(post, p)

	return p
}

// buildArticleJSONLD 生成 Article JSON-LD 结构化数据
func buildArticleJSONLD(post *contentdomain.Post, p seoPage) string {
	ld := map[string]interface{}{
		"@context": "https://schema.org",
		"@type":    "Article",
		"headline": p.Title,
		"url":      p.CanonicalURL,
	}

	if p.Description != "" {
		ld["description"] = p.Description
	}
	if p.OGImage != "" {
		ld["image"] = p.OGImage
	}
	if post.PublishedAt != nil {
		ld["datePublished"] = post.PublishedAt.Format("2006-01-02T15:04:05Z07:00")
	}

	data, err := json.Marshal(ld)
	if err != nil {
		return ""
	}
	return string(data)
}
