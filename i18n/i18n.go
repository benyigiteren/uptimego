package i18n

import (
	"context"
	"net/http"
	"strings"
)

type contextKey string

const LangContextKey contextKey = "lang"

// Translation map
var translations = map[string]map[string]string{
	"tr": {
		"common.save":                 "Kaydet",
		"common.cancel":               "İptal",
		"common.delete":               "Sil",
		"common.edit":                 "Düzenle",
		"common.actions":              "İşlemler",
		"common.status":               "Durum",
		"common.name":                 "Ad",
		"common.type":                 "Tip",
		"common.target":               "Hedef",
		"common.interval":             "Kontrol Aralığı",
		"common.timeout":              "Zaman Aşımı",
		"common.retries":              "Yeniden Deneme",
		"common.active":               "Aktif",
		"common.inactive":             "Pasif",
		"common.unknown":              "Bilinmiyor",
		"common.up":                   "Çevrimiçi",
		"common.down":                 "Çevrimdışı",
		"common.loading":              "Yükleniyor...",
		"common.seconds":              "saniye",
		"common.ms":                   "ms",

		// Setup Page
		"setup.title":                 "UptimeGo Kurulumu",
		"setup.description":           "Sistemi kullanmaya başlamak için ilk Super Admin kullanıcısını oluşturun.",
		"setup.username":              "Kullanıcı Adı",
		"setup.password":              "Şifre",
		"setup.submit":                "Kurulumu Tamamla",
		"setup.error.exists":          "Sistem zaten kurulmuş. Kurulum sayfası kapalı.",
		"setup.error.fields":          "Lütfen tüm alanları doldurun ve şifrenin en az 6 karakter olmasını sağlayın.",

		// Login Page
		"login.title":                 "Giriş Yap",
		"login.username":              "Kullanıcı Adı",
		"login.password":              "Şifre",
		"login.submit":                "Giriş Yap",
		"login.error.invalid":         "Geçersiz kullanıcı adı veya şifre",

		// Dashboard
		"dashboard.title":             "Kontrol Paneli",
		"dashboard.stats.monitors":     "Toplam Monitör",
		"dashboard.stats.up":           "Çevrimiçi",
		"dashboard.stats.down":         "Çevrimdışı",
		"dashboard.stats.avg_latency":  "Ort. Gecikme",
		"dashboard.logout":             "Çıkış Yap",
		"dashboard.settings":           "Ayarlar",
		"dashboard.public_page":        "Kamusal Sayfa",
		"dashboard.add_monitor":        "Yeni Monitör Ekle",
		"dashboard.no_monitors":        "Henüz eklenmiş monitör bulunmuyor.",
		"dashboard.no_checks":          "Henüz kontrol yapılmadı",
		"dashboard.chart.title":        "Son Yanıt Süreleri (ms)",
		"dashboard.chart.none":         "Grafik için yeterli veri yok.",
		"dashboard.ssl_expires":        "SSL Vadesi: %d gün kaldı",
		"dashboard.ssl_invalid":        "SSL Sertifikası Hatalı veya Yok",

		// Monitor Form
		"monitor.form.add_title":       "Monitör Ekle",
		"monitor.form.edit_title":      "Monitörü Düzenle",
		"monitor.form.type_http":       "HTTP(S) Kontrolü",
		"monitor.form.type_ping":       "Ping (ICMP/TCP) Kontrolü",
		"monitor.form.target_http_placeholder": "https://example.com veya http://192.168.1.1:8080",
		"monitor.form.target_ping_placeholder": "example.com, 8.8.8.8 veya 192.168.1.1:80",
		"monitor.form.keyword":         "Anahtar Kelime Kontrolü (Opsiyonel)",
		"monitor.form.keyword_desc":    "HTML içeriğinde bulunması gereken kelime",
		"monitor.form.ssl_warning":     "SSL Vadesi Yaklaşınca Uyar",
		"monitor.form.public":          "Kamusal Durum Sayfasında Göster",
		"monitor.form.interval_desc":   "Kontroller arasındaki süre",
		"monitor.form.timeout_desc":    "Zaman aşımı (milisaniye)",
		"monitor.form.retries_desc":    "Hata bildirilmeden önce yapılacak deneme sayısı",
		"monitor.form.attempts":        "deneme",

		// Settings Page
		"settings.title":              "UptimeGo Sistem Ayarları",
		"settings.tab.notifications":  "Bildirim Kanalları",
		"settings.tab.users":          "Kullanıcı Yönetimi",
		"settings.tab.general":        "Genel Ayarlar",
		"settings.tab.api":            "API & API Anahtarı",
		
		"settings.smtp.enable":         "SMTP E-Posta Bildirimini Etkinleştir",
		"settings.smtp.host":           "SMTP Host",
		"settings.smtp.port":           "SMTP Port",
		"settings.smtp.user":           "SMTP Kullanıcı",
		"settings.smtp.pass":           "SMTP Şifre",
		"settings.smtp.from":           "Gönderici E-Posta",
		"settings.smtp.to":             "Alıcı E-Posta",

		"settings.discord.enable":      "Discord Webhook Bildirimini Etkinleştir",
		"settings.discord.url":         "Discord Webhook URL",

		"settings.telegram.enable":     "Telegram Bildirimini Etkinleştir",
		"settings.telegram.token":      "Bot Token",
		"settings.telegram.chat_id":    "Chat ID",

		"settings.general.title":       "Kamusal Sayfa Başlığı",
		"settings.general.announcement": "Özel Duyuru Metni",
		"settings.general.logo":        "Logo URL",

		"settings.users.add":           "Kullanıcı Ekle",
		"settings.users.role":          "Rol",
		"settings.users.role.super_admin": "Süper Admin",
		"settings.users.role.admin":    "Admin",
		"settings.users.role.viewer":   "İzleyici",
		"settings.users.no_users":      "Diğer kullanıcılar bulunmuyor.",
		"settings.users.delete_confirm": "Bu kullanıcıyı silmek istediğinize emin misiniz?",
		"settings.tab.profile":         "Profilim",
		"settings.profile.title":        "Profil Ayarları",
		"settings.profile.username":     "Kullanıcı Adı",
		"settings.profile.password":     "Yeni Şifre (Değiştirmek istemiyorsanız boş bırakın)",
		"settings.profile.save":         "Bilgileri Kaydet",
		"settings.profile.success":      "Profil başarıyla güncellendi!",
		"settings.alert.on_down":        "Servis Çevrimdışı (DOWN) Olduğunda Bildir",
		"settings.alert.on_up":          "Servis Çevrimiçi (UP) Olduğunda Bildir",
		"settings.alert.repeat":         "Servis Çevrimdışı Kaldığı Sürece Yenilenen Alarmları Gönder",
		"monitor.form.alert_interval":  "Yenilenen Alarm",
		"monitor.form.alert_interval_desc": "saniye (0=kapalı)",

		// Public Status Page
		"status.all_operational":      "Tüm Sistemler Sorunsuz Çalışıyor",
		"status.some_issues":          "Bazı Sistemlerde Sorunlar Var",
		"status.all_down":             "Tüm Sistemler Çevrimdışı!",
		"status.operational":          "Çalışıyor",
		"status.down":                 "Hata var",
		"status.last_checked":         "Son kontrol",
		"status.latency":              "Gecikme",
		"status.uptime_24h":           "24 Saat Uptime",
		"status.uptime_7d":            "7 Gün Uptime",
		"status.uptime_30d":           "30 Gün Uptime",
		"dashboard.services":                 "Servisler",
		"dashboard.uptime_summary":           "Uptime (24s / 7g / 30g)",
		"dashboard.confirm_delete_monitor":   "Bu monitörü silmek istediğinize emin misiniz?",
		"dashboard.confirm_delete_user":      "Bu kullanıcıyı silmek istediğinize emin misiniz?",
		"dashboard.chart.label":              "Yanıt Süresi (ms)",
		"settings.smtp.title":                "E-Posta Bildirimi (SMTP)",
		"settings.discord.title":             "Discord Webhook Bildirimi",
		"settings.telegram.title":            "Telegram Bildirimi",
		"settings.general.branding":          "Kamusal Sayfa Özelleştirme",
		"settings.notice":                    "Duyuru",
		"settings.saved_success":             "Ayarlar başarıyla kaydedildi!",
		"settings.placeholder.notice":        "Örn: Pazar günü saat 02:00'de planlı bakım çalışması yapılacaktır.",
		"settings.alert.title":               "Bildirim Mesaj Şablonları",
		"settings.alert.subject_down":        "Çevrimdışı (DOWN) Konu Şablonu",
		"settings.alert.body_down":           "Çevrimdışı (DOWN) Mesaj Şablonu",
		"settings.alert.subject_up":          "Çevrimiçi (UP) Konu Şablonu",
		"settings.alert.body_up":             "Çevrimiçi (UP) Mesaj Şablonu",
		"settings.alert.placeholders_desc":   "Kullanılabilir değişkenler: {name} (Servis adı), {target} (Hedef), {status} (Durum: UP/DOWN), {time} (Zaman), {message} (Hata detayı), {latency} (Gecikme ms)",
		"settings.alert.test":                "Bağlantıyı Test Et",
		"settings.alert.testing":             "Test ediliyor...",
		"settings.alert.test_success":        "Test bildirimi başarıyla gönderildi!",
		"status.no_monitors":                 "Henüz izlenen kamusal bir servis bulunmuyor.",
		"status.powered_by":                  "Altyapı",
		"settings.api.title":                 "API Yetkilendirme",
		"settings.api.desc":                  "UptimeGo'ya harici istekleri doğrulamak için bu API Anahtarını kullanın. İsteklerinize X-API-Key üstbilgisini (header) veya ?api_key= sorgu parametresini ekleyin.",
		"settings.api.copy":                  "Kopyala",
		"settings.api.copied":                "Kopyalandı!",
		"settings.api.reset":                 "Sıfırla",
		"settings.api.reset_confirm":         "API Anahtarını yeniden oluşturmak istediğinizden emin misiniz? Mevcut anahtarı kullanan tüm dış entegrasyonlar çalışmayı durduracaktır.",
		"settings.api.docs.title":            "REST API Dokümantasyonu",
		"settings.api.docs.list":             "1. Tüm Monitörleri Listele",
		"settings.api.docs.create":           "2. Monitör Oluştur / Güncelle",
		"settings.api.docs.delete":           "3. Monitörü Sil",
	},
	"en": {
		"common.save":                 "Save",
		"common.cancel":               "Cancel",
		"common.delete":               "Delete",
		"common.edit":                 "Edit",
		"common.actions":              "Actions",
		"common.status":               "Status",
		"common.name":                 "Name",
		"common.type":                 "Type",
		"common.target":               "Target",
		"common.interval":             "Interval",
		"common.timeout":              "Timeout",
		"common.retries":              "Retries",
		"common.active":               "Active",
		"common.inactive":             "Inactive",
		"common.unknown":              "Unknown",
		"common.up":                   "Online",
		"common.down":                 "Offline",
		"common.loading":              "Loading...",
		"common.seconds":              "seconds",
		"common.ms":                   "ms",

		// Setup Page
		"setup.title":                 "UptimeGo Installation",
		"setup.description":           "Create the initial Super Admin user to start using the system.",
		"setup.username":              "Username",
		"setup.password":              "Password",
		"setup.submit":                "Complete Setup",
		"setup.error.exists":          "System is already installed. Setup page is disabled.",
		"setup.error.fields":          "Please fill in all fields and ensure the password is at least 6 characters.",

		// Login Page
		"login.title":                 "Login",
		"login.username":              "Username",
		"login.password":              "Password",
		"login.submit":                "Login",
		"login.error.invalid":         "Invalid username or password",

		// Dashboard
		"dashboard.title":             "Dashboard",
		"dashboard.stats.monitors":     "Total Monitors",
		"dashboard.stats.up":           "Online",
		"dashboard.stats.down":         "Offline",
		"dashboard.stats.avg_latency":  "Avg Latency",
		"dashboard.logout":             "Logout",
		"dashboard.settings":           "Settings",
		"dashboard.public_page":        "Public Page",
		"dashboard.add_monitor":        "Add New Monitor",
		"dashboard.no_monitors":        "No monitors found.",
		"dashboard.no_checks":          "No checks yet",
		"dashboard.chart.title":        "Recent Response Times (ms)",
		"dashboard.chart.none":         "Not enough data for chart.",
		"dashboard.ssl_expires":        "SSL: %d days remaining",
		"dashboard.ssl_invalid":        "SSL Certificate Invalid or Missing",

		// Monitor Form
		"monitor.form.add_title":       "Add Monitor",
		"monitor.form.edit_title":      "Edit Monitor",
		"monitor.form.type_http":       "HTTP(S) Check",
		"monitor.form.type_ping":       "Ping (ICMP/TCP) Check",
		"monitor.form.target_http_placeholder": "https://example.com or http://192.168.1.1:8080",
		"monitor.form.target_ping_placeholder": "example.com, 8.8.8.8 or 192.168.1.1:80",
		"monitor.form.keyword":         "Keyword Check (Optional)",
		"monitor.form.keyword_desc":    "HTML content must contain this text",
		"monitor.form.ssl_warning":     "Warn on SSL Expiry",
		"monitor.form.public":          "Show on Public Status Page",
		"monitor.form.interval_desc":   "Time between checks",
		"monitor.form.timeout_desc":    "Timeout in milliseconds",
		"monitor.form.retries_desc":    "Number of attempts before marking down",
		"monitor.form.attempts":        "attempts",

		// Settings Page
		"settings.title":              "UptimeGo Settings",
		"settings.tab.notifications":  "Notifications",
		"settings.tab.users":          "User Management",
		"settings.tab.general":        "General Settings",
		"settings.tab.api":            "API & API Key",
		
		"settings.smtp.enable":         "Enable SMTP Notifications",
		"settings.smtp.host":           "SMTP Host",
		"settings.smtp.port":           "SMTP Port",
		"settings.smtp.user":           "SMTP User",
		"settings.smtp.pass":           "SMTP Password",
		"settings.smtp.from":           "From Email",
		"settings.smtp.to":             "To Email",

		"settings.discord.enable":      "Enable Discord Notifications",
		"settings.discord.url":         "Discord Webhook URL",

		"settings.telegram.enable":     "Enable Telegram Notifications",
		"settings.telegram.token":      "Bot Token",
		"settings.telegram.chat_id":    "Chat ID",

		"settings.general.title":       "Public Page Title",
		"settings.general.announcement": "Custom Announcement",
		"settings.general.logo":        "Logo URL",

		"settings.users.add":           "Add User",
		"settings.users.role":          "Role",
		"settings.users.role.super_admin": "Super Admin",
		"settings.users.role.admin":    "Admin",
		"settings.users.role.viewer":   "Viewer",
		"settings.users.no_users":      "No other users.",
		"settings.users.delete_confirm": "Are you sure you want to delete this user?",
		"settings.tab.profile":         "My Profile",
		"settings.profile.title":        "Profile Settings",
		"settings.profile.username":     "Username",
		"settings.profile.password":     "New Password (Leave blank to keep current)",
		"settings.profile.save":         "Save Profile",
		"settings.profile.success":      "Profile updated successfully!",
		"settings.alert.on_down":        "Notify When Service Goes Offline (DOWN)",
		"settings.alert.on_up":          "Notify When Service Goes Online (UP)",
		"settings.alert.repeat":         "Send Repeat Alerts While Service Remains Offline",
		"monitor.form.alert_interval":  "Repeat Alert",
		"monitor.form.alert_interval_desc": "seconds (0=disabled)",

		// Public Status Page
		"status.all_operational":      "All Systems Operational",
		"status.some_issues":          "Some Systems Experience Issues",
		"status.all_down":             "All Systems Offline!",
		"status.operational":          "Operational",
		"status.down":                 "Outage",
		"status.last_checked":         "Last checked",
		"status.uptime_24h":           "24h Uptime",
		"status.uptime_7d":            "7d Uptime",
		"status.uptime_30d":           "30d Uptime",
		"dashboard.services":                 "Services",
		"dashboard.uptime_summary":           "Uptime (24h / 7d / 30d)",
		"dashboard.confirm_delete_monitor":   "Are you sure you want to delete this monitor?",
		"dashboard.confirm_delete_user":      "Are you sure you want to delete this user?",
		"dashboard.chart.label":              "Response Time (ms)",
		"settings.smtp.title":                "Email Notification (SMTP)",
		"settings.discord.title":             "Discord Notification",
		"settings.telegram.title":            "Telegram Notification",
		"settings.general.branding":          "Status Page Branding",
		"settings.notice":                    "Notice",
		"settings.saved_success":             "Settings saved successfully!",
		"settings.placeholder.notice":        "e.g. Planned system maintenance on Sunday at 02:00 UTC.",
		"settings.alert.title":               "Alert Message Templates",
		"settings.alert.subject_down":        "Offline (DOWN) Subject Template",
		"settings.alert.body_down":           "Offline (DOWN) Body Template",
		"settings.alert.subject_up":          "Online (UP) Subject Template",
		"settings.alert.body_up":             "Online (UP) Body Template",
		"settings.alert.placeholders_desc":   "Available variables: {name} (Service Name), {target} (Target), {status} (Status: UP/DOWN), {time} (Time), {message} (Error details), {latency} (Latency ms)",
		"settings.alert.test":                "Test Connection",
		"settings.alert.testing":             "Testing...",
		"settings.alert.test_success":        "Test notification sent successfully!",
		"status.no_monitors":                 "No public services monitored yet.",
		"status.powered_by":                  "Powered by",
		"settings.api.title":                 "API Authentication",
		"settings.api.desc":                  "Use this API Key to authenticate external requests to UptimeGo. Include it as the X-API-Key header or ?api_key= query parameter.",
		"settings.api.copy":                  "Copy",
		"settings.api.copied":                "Copied!",
		"settings.api.reset":                 "Reset",
		"settings.api.reset_confirm":         "Are you sure you want to regenerate your API Key? Any external integrations using the current key will stop working.",
		"settings.api.docs.title":            "REST API Documentation",
		"settings.api.docs.list":             "1. List All Monitors",
		"settings.api.docs.create":           "2. Create / Update Monitor",
		"settings.api.docs.delete":           "3. Delete Monitor",
	},
}

// T translates a key based on the language. If key is missing, returns the key.
func T(lang string, key string) string {
	if lang != "tr" {
		lang = "en"
	}
	if translations[lang] != nil {
		if val, ok := translations[lang][key]; ok {
			return val
		}
	}
	// Fallback to English key if TR is missing
	if lang == "tr" {
		if val, ok := translations["en"][key]; ok {
			return val
		}
	}
	return key
}

// GetLanguage detects request language.
// Priority:
// 1. "lang" Cookie (explicit user choice)
// 2. "Accept-Language" Header (Turkey/Turkish check)
// 3. Fallback to English ("en")
func GetLanguage(r *http.Request) string {
	// 1. Check Cookie
	if cookie, err := r.Cookie("lang"); err == nil {
		if cookie.Value == "tr" || cookie.Value == "en" {
			return cookie.Value
		}
	}

	// 2. Check Accept-Language header
	acceptLang := r.Header.Get("Accept-Language")
	if acceptLang != "" {
		// Look for Turkish locale
		langs := strings.Split(acceptLang, ",")
		for _, l := range langs {
			parts := strings.Split(strings.TrimSpace(l), ";")
			code := strings.ToLower(parts[0])
			if strings.HasPrefix(code, "tr") {
				return "tr"
			}
		}
	}

	// 3. Fallback
	return "en"
}

// Middleware injects language code into request context
func Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lang := GetLanguage(r)
		ctx := context.WithValue(r.Context(), LangContextKey, lang)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// FromContext extracts language from context, defaults to "en"
func FromContext(ctx context.Context) string {
	if ctx == nil {
		return "en"
	}
	if lang, ok := ctx.Value(LangContextKey).(string); ok {
		return lang
	}
	return "en"
}
