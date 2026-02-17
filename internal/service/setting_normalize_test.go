package service

import (
	"testing"

	"github.com/dujiao-next/internal/constants"
)

func TestUpdateOrderSettingNormalized(t *testing.T) {
	repo := newMockSettingRepo()
	svc := NewSettingService(repo)

	result, err := svc.Update(constants.SettingKeyOrderConfig, map[string]interface{}{
		constants.SettingFieldPaymentExpireMinutes: "20000",
		"extra": "keep",
	})
	if err != nil {
		t.Fatalf("update order config failed: %v", err)
	}

	minutes, err := parseSettingInt(result[constants.SettingFieldPaymentExpireMinutes])
	if err != nil {
		t.Fatalf("parse payment_expire_minutes failed: %v", err)
	}
	if minutes != 10080 {
		t.Fatalf("unexpected payment_expire_minutes, expected 10080 got %d", minutes)
	}
	if result["extra"] != "keep" {
		t.Fatalf("unexpected extra field: %v", result["extra"])
	}
}

func TestUpdateSiteSettingNormalized(t *testing.T) {
	repo := newMockSettingRepo()
	svc := NewSettingService(repo)

	result, err := svc.Update(constants.SettingKeySiteConfig, map[string]interface{}{
		"brand": map[string]interface{}{
			"site_name": 123,
		},
		"contact": map[string]interface{}{
			"telegram": "  https://t.me/demo  ",
			"whatsapp": 123,
		},
		"seo": map[string]interface{}{
			"title": map[string]interface{}{
				"zh-CN": "  标题  ",
				"en-US": "  Title  ",
			},
		},
		"about": map[string]interface{}{
			"hero": map[string]interface{}{
				"title": map[string]interface{}{
					"zh-CN": "  关于我们  ",
					"en-US": "  About Us  ",
				},
				"subtitle": map[string]interface{}{
					"zh-CN": "  欢迎来到独角工作室  ",
				},
			},
			"introduction": map[string]interface{}{
				"zh-CN": "  我们致力于为用户提供可靠服务  ",
				"zh-TW": 123,
			},
			"services": map[string]interface{}{
				"title": map[string]interface{}{
					"zh-CN": "  我们的服务  ",
				},
				"items": []interface{}{
					map[string]interface{}{
						"zh-CN": "  服务项一  ",
						"en-US": "  Service One  ",
					},
					map[string]interface{}{
						"zh-CN": "",
						"zh-TW": "",
						"en-US": "",
					},
					"invalid",
				},
			},
			"contact": map[string]interface{}{
				"title": map[string]interface{}{
					"zh-CN": "  联系我们  ",
				},
				"text": map[string]interface{}{
					"zh-CN": "  通过以下方式联系我们  ",
					"en-US": "  Contact us via channels below  ",
				},
			},
		},
		"scripts": []interface{}{
			map[string]interface{}{
				"name":     "  Google Analytics  ",
				"enabled":  "true",
				"position": "head",
				"code":     "  window.dataLayer = window.dataLayer || [];  ",
			},
			map[string]interface{}{
				"name":     "Footer Tracker",
				"enabled":  1,
				"position": "body_end",
				"code":     "window.__footerTracker = true;",
			},
			map[string]interface{}{
				"name":     "Fallback Position",
				"enabled":  false,
				"position": "invalid",
				"code":     "console.log('fallback');",
			},
			map[string]interface{}{
				"name":     "Skip Empty",
				"enabled":  true,
				"position": "head",
				"code":     "   ",
			},
			"invalid",
		},
		"languages": []interface{}{" zh-CN ", "en-US", "", "en-US"},
	})
	if err != nil {
		t.Fatalf("update site config failed: %v", err)
	}

	brand, ok := result["brand"].(map[string]interface{})
	if !ok {
		t.Fatalf("invalid brand payload type: %T", result["brand"])
	}
	if brand["site_name"] != "" {
		t.Fatalf("unexpected brand.site_name: %v", brand["site_name"])
	}

	contact, ok := result["contact"].(map[string]interface{})
	if !ok {
		t.Fatalf("invalid contact payload type: %T", result["contact"])
	}
	if contact["telegram"] != "https://t.me/demo" {
		t.Fatalf("unexpected telegram: %v", contact["telegram"])
	}
	if contact["whatsapp"] != "" {
		t.Fatalf("unexpected whatsapp: %v", contact["whatsapp"])
	}

	seo, ok := result["seo"].(map[string]interface{})
	if !ok {
		t.Fatalf("invalid seo payload type: %T", result["seo"])
	}
	title, ok := seo["title"].(map[string]interface{})
	if !ok {
		t.Fatalf("invalid seo.title payload type: %T", seo["title"])
	}
	if title["zh-CN"] != "标题" {
		t.Fatalf("unexpected seo.title.zh-CN: %v", title["zh-CN"])
	}
	if title["en-US"] != "Title" {
		t.Fatalf("unexpected seo.title.en-US: %v", title["en-US"])
	}
	if title["zh-TW"] != "" {
		t.Fatalf("unexpected seo.title.zh-TW: %v", title["zh-TW"])
	}

	legal, ok := result["legal"].(map[string]interface{})
	if !ok {
		t.Fatalf("invalid legal payload type: %T", result["legal"])
	}
	privacy, ok := legal["privacy"].(map[string]interface{})
	if !ok {
		t.Fatalf("invalid legal.privacy payload type: %T", legal["privacy"])
	}
	if privacy["zh-CN"] != "" || privacy["zh-TW"] != "" || privacy["en-US"] != "" {
		t.Fatalf("unexpected legal.privacy payload: %+v", privacy)
	}

	about, ok := result["about"].(map[string]interface{})
	if !ok {
		t.Fatalf("invalid about payload type: %T", result["about"])
	}

	hero, ok := about["hero"].(map[string]interface{})
	if !ok {
		t.Fatalf("invalid about.hero payload type: %T", about["hero"])
	}
	heroTitle, ok := hero["title"].(map[string]interface{})
	if !ok {
		t.Fatalf("invalid about.hero.title payload type: %T", hero["title"])
	}
	if heroTitle["zh-CN"] != "关于我们" || heroTitle["en-US"] != "About Us" || heroTitle["zh-TW"] != "" {
		t.Fatalf("unexpected about.hero.title payload: %+v", heroTitle)
	}

	introduction, ok := about["introduction"].(map[string]interface{})
	if !ok {
		t.Fatalf("invalid about.introduction payload type: %T", about["introduction"])
	}
	if introduction["zh-CN"] != "我们致力于为用户提供可靠服务" || introduction["zh-TW"] != "" {
		t.Fatalf("unexpected about.introduction payload: %+v", introduction)
	}

	services, ok := about["services"].(map[string]interface{})
	if !ok {
		t.Fatalf("invalid about.services payload type: %T", about["services"])
	}
	serviceItems, ok := services["items"].([]interface{})
	if !ok {
		t.Fatalf("invalid about.services.items payload type: %T", services["items"])
	}
	if len(serviceItems) != 1 {
		t.Fatalf("unexpected about.services.items size: %d", len(serviceItems))
	}
	serviceItem, ok := serviceItems[0].(map[string]interface{})
	if !ok {
		t.Fatalf("invalid about.services.items[0] payload type: %T", serviceItems[0])
	}
	if serviceItem["zh-CN"] != "服务项一" || serviceItem["en-US"] != "Service One" || serviceItem["zh-TW"] != "" {
		t.Fatalf("unexpected about.services.items[0] payload: %+v", serviceItem)
	}

	aboutContact, ok := about["contact"].(map[string]interface{})
	if !ok {
		t.Fatalf("invalid about.contact payload type: %T", about["contact"])
	}
	contactText, ok := aboutContact["text"].(map[string]interface{})
	if !ok {
		t.Fatalf("invalid about.contact.text payload type: %T", aboutContact["text"])
	}
	if contactText["zh-CN"] != "通过以下方式联系我们" || contactText["en-US"] != "Contact us via channels below" {
		t.Fatalf("unexpected about.contact.text payload: %+v", contactText)
	}

	languages, ok := result["languages"].([]string)
	if !ok {
		t.Fatalf("invalid languages payload type: %T", result["languages"])
	}
	if len(languages) != 2 || languages[0] != "zh-CN" || languages[1] != "en-US" {
		t.Fatalf("unexpected languages: %+v", languages)
	}

	scripts, ok := result["scripts"].([]interface{})
	if !ok {
		t.Fatalf("invalid scripts payload type: %T", result["scripts"])
	}
	if len(scripts) != 3 {
		t.Fatalf("unexpected scripts size: %d", len(scripts))
	}
	firstScript, ok := scripts[0].(map[string]interface{})
	if !ok {
		t.Fatalf("invalid scripts[0] payload type: %T", scripts[0])
	}
	if firstScript["name"] != "Google Analytics" || firstScript["enabled"] != true || firstScript["position"] != "head" || firstScript["code"] != "window.dataLayer = window.dataLayer || [];" {
		t.Fatalf("unexpected scripts[0]: %+v", firstScript)
	}
	secondScript, ok := scripts[1].(map[string]interface{})
	if !ok {
		t.Fatalf("invalid scripts[1] payload type: %T", scripts[1])
	}
	if secondScript["position"] != "body_end" || secondScript["enabled"] != true {
		t.Fatalf("unexpected scripts[1]: %+v", secondScript)
	}
	thirdScript, ok := scripts[2].(map[string]interface{})
	if !ok {
		t.Fatalf("invalid scripts[2] payload type: %T", scripts[2])
	}
	if thirdScript["position"] != "head" || thirdScript["enabled"] != false {
		t.Fatalf("unexpected scripts[2]: %+v", thirdScript)
	}
}

func TestUpdateSiteSettingNormalizedDefaultAbout(t *testing.T) {
	repo := newMockSettingRepo()
	svc := NewSettingService(repo)

	result, err := svc.Update(constants.SettingKeySiteConfig, map[string]interface{}{})
	if err != nil {
		t.Fatalf("update site config failed: %v", err)
	}

	brand, ok := result["brand"].(map[string]interface{})
	if !ok {
		t.Fatalf("invalid brand payload type: %T", result["brand"])
	}
	if brand["site_name"] != "" {
		t.Fatalf("unexpected default brand payload: %+v", brand)
	}
	scripts, ok := result["scripts"].([]interface{})
	if !ok {
		t.Fatalf("invalid scripts payload type: %T", result["scripts"])
	}
	if len(scripts) != 0 {
		t.Fatalf("unexpected default scripts payload: %+v", scripts)
	}

	about, ok := result["about"].(map[string]interface{})
	if !ok {
		t.Fatalf("invalid about payload type: %T", result["about"])
	}

	hero, ok := about["hero"].(map[string]interface{})
	if !ok {
		t.Fatalf("invalid about.hero payload type: %T", about["hero"])
	}
	heroTitle, ok := hero["title"].(map[string]interface{})
	if !ok {
		t.Fatalf("invalid about.hero.title payload type: %T", hero["title"])
	}
	if heroTitle["zh-CN"] != "" || heroTitle["zh-TW"] != "" || heroTitle["en-US"] != "" {
		t.Fatalf("unexpected default about.hero.title: %+v", heroTitle)
	}

	services, ok := about["services"].(map[string]interface{})
	if !ok {
		t.Fatalf("invalid about.services payload type: %T", about["services"])
	}
	serviceItems, ok := services["items"].([]interface{})
	if !ok {
		t.Fatalf("invalid about.services.items payload type: %T", services["items"])
	}
	if len(serviceItems) != 0 {
		t.Fatalf("unexpected default about.services.items size: %d", len(serviceItems))
	}
}

func TestUpdateSiteSettingNormalizedScriptsLimit(t *testing.T) {
	repo := newMockSettingRepo()
	svc := NewSettingService(repo)

	scripts := make([]interface{}, 0, 25)
	for i := 0; i < 25; i++ {
		scripts = append(scripts, map[string]interface{}{
			"name":     "script",
			"enabled":  true,
			"position": "head",
			"code":     "console.log('ok');",
		})
	}

	result, err := svc.Update(constants.SettingKeySiteConfig, map[string]interface{}{
		"scripts": scripts,
	})
	if err != nil {
		t.Fatalf("update site config failed: %v", err)
	}

	normalizedScripts, ok := result["scripts"].([]interface{})
	if !ok {
		t.Fatalf("invalid scripts payload type: %T", result["scripts"])
	}
	if len(normalizedScripts) != settingSiteScriptsMaxCount {
		t.Fatalf("unexpected scripts size: %d", len(normalizedScripts))
	}
}
