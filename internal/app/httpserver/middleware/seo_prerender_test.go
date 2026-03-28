package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	categorydomain "github.com/dujiao-next/internal/modules/catalog/category/domain"
	productdomain "github.com/dujiao-next/internal/modules/catalog/product/domain"
	contentdomain "github.com/dujiao-next/internal/modules/content/domain"
	"github.com/dujiao-next/internal/shared/jsonmap"
	"github.com/dujiao-next/internal/shared/jsonslice"
)

func TestIsCrawler(t *testing.T) {
	tests := []struct {
		ua     string
		expect bool
	}{
		{"Googlebot/2.1 (+http://www.google.com/bot.html)", true},
		{"Mozilla/5.0 (compatible; Baiduspider/2.0; +http://www.baidu.com/search/spider.html)", true},
		{"Mozilla/5.0 (compatible; bingbot/2.0; +http://www.bing.com/bingbot.htm)", true},
		{"facebookexternalhit/1.1 (+http://www.facebook.com/externalhit_uatext.php)", true},
		{"Twitterbot/1.0", true},
		{"TelegramBot (like TwitterBot)", true},
		{"WhatsApp/2.21.1.14 A", true},
		{"LinkedInBot/1.0", true},
		{"Mozilla/5.0 GPTBot/1.0", true},
		{"ChatGPT-User/1.0", true},
		{"ClaudeBot/1.0", true},
		{"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36", false},
		{"Mozilla/5.0 (iPhone; CPU iPhone OS 15_0 like Mac OS X)", false},
		{"curl/7.68.0", false},
		{"", false},
	}

	for _, tt := range tests {
		got := isCrawler(tt.ua)
		if got != tt.expect {
			t.Errorf("isCrawler(%q) = %v, want %v", tt.ua, got, tt.expect)
		}
	}
}

func TestExtractLocalizedText(t *testing.T) {
	tests := []struct {
		name   string
		json   jsonmap.JSON
		locale string
		expect string
	}{
		{"zh-CN preferred", jsonmap.JSON{"zh-CN": "中文标题", "en-US": "English Title"}, "zh-CN", "中文标题"},
		{"fallback to zh-CN", jsonmap.JSON{"zh-CN": "中文标题", "en-US": "English Title"}, "ja-JP", "中文标题"},
		{"fallback to en-US", jsonmap.JSON{"en-US": "English Title"}, "ja-JP", "English Title"},
		{"nil json", nil, "zh-CN", ""},
		{"empty json", jsonmap.JSON{}, "zh-CN", ""},
		{"first available", jsonmap.JSON{"fr-FR": "Titre français"}, "zh-CN", "Titre français"},
	}

	for _, tt := range tests {
		got := extractLocalizedText(tt.json, tt.locale)
		if got != tt.expect {
			t.Errorf("extractLocalizedText(%s) = %q, want %q", tt.name, got, tt.expect)
		}
	}
}

func TestRenderSEOPage(t *testing.T) {
	p := seoPage{
		Title:        "测试商品",
		Description:  "商品描述",
		CanonicalURL: "https://example.com/products/test",
		OGType:       "product",
		OGImage:      "https://example.com/image.jpg",
		Content:      "<p>详细的商品介绍内容</p>",
		Category:     "数字商品",
		Tags:         []string{"游戏", "充值"},
		ProductsURL:  "https://example.com/products",
		JSONLD:       `{"@context":"https://schema.org","@type":"Product","name":"测试商品"}`,
	}

	result := renderSEOPage(p)

	checks := []string{
		"<title>测试商品</title>",
		`<meta name="description" content="商品描述" />`,
		`<link rel="canonical" href="https://example.com/products/test" />`,
		`<meta property="og:type" content="product" />`,
		`<meta property="og:title" content="测试商品" />`,
		`<meta property="og:description" content="商品描述" />`,
		`<meta property="og:image" content="https://example.com/image.jpg" />`,
		`<meta property="og:url" content="https://example.com/products/test" />`,
		`<meta name="twitter:card" content="summary_large_image" />`,
		`<meta name="twitter:title" content="测试商品" />`,
		`<meta name="twitter:image" content="https://example.com/image.jpg" />`,
		`application/ld+json`,
		"<h1>测试商品</h1>",
		"<h2>商品简介</h2>",
		"<h2>商品详情</h2>",
		"<article>",
		"<p>详细的商品介绍内容</p>",
		"分类：数字商品",
		"标签：游戏、充值",
		`<a href="https://example.com">首页</a>`,
		`<a href="https://example.com/products">商品列表</a>`,
		`<img src="https://example.com/image.jpg"`,
	}

	for _, check := range checks {
		if !strings.Contains(result, check) {
			t.Errorf("renderSEOPage missing: %s", check)
		}
	}

	// keywords should NOT be present
	if strings.Contains(result, `name="keywords"`) {
		t.Error("renderSEOPage should not include keywords meta")
	}
}

func TestRenderSEOPageEscaping(t *testing.T) {
	p := seoPage{
		Title:       `Test <script>alert("xss")</script>`,
		Description: `"quotes" & <tags>`,
	}

	result := renderSEOPage(p)

	if strings.Contains(result, "<script>alert") {
		t.Error("renderSEOPage should escape script tags")
	}
	if !strings.Contains(result, "&lt;script&gt;") {
		t.Error("renderSEOPage should HTML-escape special characters in title")
	}
}

func TestBuildProductSEOPage(t *testing.T) {
	product := &productdomain.Product{
		Slug:            "test-product",
		TitleJSON:       jsonmap.JSON{"zh-CN": "测试商品"},
		DescriptionJSON: jsonmap.JSON{"zh-CN": "这是一个测试商品"},
		ContentJSON:     jsonmap.JSON{"zh-CN": "<p>商品详情内容</p>"},
		SeoMetaJSON: jsonmap.JSON{
			"title":       map[string]interface{}{"zh-CN": "SEO标题"},
			"description": map[string]interface{}{"zh-CN": "SEO描述"},
		},
		Images: jsonslice.Strings{"/uploads/test.jpg"},
		Tags:   jsonslice.Strings{"标签1", "标签2"},
	}
	product.Category = categorydomain.Category{NameJSON: jsonmap.JSON{"zh-CN": "测试分类"}}

	req := httptest.NewRequest(http.MethodGet, "https://example.com/seo/products/test-product", nil)
	req.Host = "example.com"

	page := buildProductSEOPage(product, req)

	if page.Title != "SEO标题" {
		t.Errorf("title want SEO标题, got %s", page.Title)
	}
	if page.Description != "SEO描述" {
		t.Errorf("description want SEO描述, got %s", page.Description)
	}
	if page.OGType != "product" {
		t.Errorf("og:type want product, got %s", page.OGType)
	}
	if !strings.Contains(page.CanonicalURL, "/products/test-product") {
		t.Errorf("canonical should contain /products/test-product, got %s", page.CanonicalURL)
	}
	if page.Content != "<p>商品详情内容</p>" {
		t.Errorf("content want <p>商品详情内容</p>, got %s", page.Content)
	}
	if page.Category != "测试分类" {
		t.Errorf("category want 测试分类, got %s", page.Category)
	}
	if len(page.Tags) != 2 {
		t.Errorf("tags want 2, got %d", len(page.Tags))
	}
	if page.JSONLD == "" {
		t.Error("JSONLD should not be empty")
	}
	if !strings.Contains(page.JSONLD, "Product") {
		t.Error("JSONLD should contain Product type")
	}
}

func TestBuildProductSEOPageFallback(t *testing.T) {
	product := &productdomain.Product{
		Slug:            "test-product",
		TitleJSON:       jsonmap.JSON{"zh-CN": "商品标题"},
		DescriptionJSON: jsonmap.JSON{"zh-CN": "商品描述"},
		Images:          jsonslice.Strings{"https://cdn.example.com/image.jpg"},
	}

	req := httptest.NewRequest(http.MethodGet, "/seo/products/test-product", nil)
	req.Host = "example.com"

	page := buildProductSEOPage(product, req)

	if page.Title != "商品标题" {
		t.Errorf("fallback title want 商品标题, got %s", page.Title)
	}
	if page.Description != "商品描述" {
		t.Errorf("fallback description want 商品描述, got %s", page.Description)
	}
	if page.OGImage != "https://cdn.example.com/image.jpg" {
		t.Errorf("og:image want https://cdn.example.com/image.jpg, got %s", page.OGImage)
	}
}

func TestBuildPostSEOPage(t *testing.T) {
	post := &contentdomain.Post{
		Slug:        "test-post",
		TitleJSON:   jsonmap.JSON{"zh-CN": "测试文章"},
		SummaryJSON: jsonmap.JSON{"zh-CN": "文章摘要"},
		ContentJSON: jsonmap.JSON{"zh-CN": "<p>文章正文</p>"},
		Thumbnail:   "/uploads/thumb.jpg",
	}

	req := httptest.NewRequest(http.MethodGet, "/seo/blog/test-post", nil)
	req.Host = "example.com"

	page := buildPostSEOPage(post, req)

	if page.Title != "测试文章" {
		t.Errorf("title want 测试文章, got %s", page.Title)
	}
	if page.Description != "文章摘要" {
		t.Errorf("description want 文章摘要, got %s", page.Description)
	}
	if page.OGType != "article" {
		t.Errorf("og:type want article, got %s", page.OGType)
	}
	if page.Content != "<p>文章正文</p>" {
		t.Errorf("content want <p>文章正文</p>, got %s", page.Content)
	}
	if !strings.Contains(page.CanonicalURL, "/blog/test-post") {
		t.Errorf("canonical should contain /blog/test-post, got %s", page.CanonicalURL)
	}
	if page.JSONLD == "" {
		t.Error("JSONLD should not be empty")
	}
	if !strings.Contains(page.JSONLD, "Article") {
		t.Error("JSONLD should contain Article type")
	}
}

func TestResolvePublicHost(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Host = "127.0.0.1:8080"
	req.Header.Set("X-Forwarded-Host", "example.com")

	got := resolvePublicHost(req)
	if got != "example.com" {
		t.Errorf("resolvePublicHost want example.com, got %s", got)
	}

	// Without X-Forwarded-Host, should fall back to req.Host
	req2 := httptest.NewRequest(http.MethodGet, "/test", nil)
	req2.Host = "localhost:8080"

	got2 := resolvePublicHost(req2)
	if got2 != "localhost:8080" {
		t.Errorf("resolvePublicHost fallback want localhost:8080, got %s", got2)
	}
}
