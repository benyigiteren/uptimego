package models

import (
	"database/sql"
	"time"
)

type Role string

const (
	RoleSuperAdmin Role = "super_admin"
	RoleAdmin      Role = "admin"
	RoleViewer     Role = "viewer"
)

// User represents a system operator
type User struct {
	ID           int64     `json:"id"`
	Username     string    `json:"username"`
	PasswordHash string    `json:"-"`
	Role         Role      `json:"role"`
	APIKey       string    `json:"api_key,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
}

// Monitor holds target configuration
type Monitor struct {
	ID               int64     `json:"id"`
	Name             string    `json:"name"`
	Type             string    `json:"type"` // "http" or "ping"
	Target           string    `json:"target"`
	Interval         int       `json:"interval"` // seconds
	Timeout          int       `json:"timeout"`  // milliseconds
	Retries          int       `json:"retries"`
	AlertInterval    int       `json:"alert_interval"` // repeat alerts in seconds when DOWN
	Active           bool      `json:"active"`
	Keyword          string    `json:"keyword"`
	SslExpiryWarning bool      `json:"ssl_expiry_warning"`
	Public           bool      `json:"public"`
	Status           string    `json:"status"` // "up", "down", "unknown"
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`

	// Helper UI fields (not stored in monitors table directly)
	LastChecked      string   `json:"last_checked,omitempty"`
	LastResponseTime int      `json:"last_response_time,omitempty"`
	SslDaysRemaining *int     `json:"ssl_days_remaining,omitempty"`
	Uptime24h        float64  `json:"uptime_24h,omitempty"`
	Uptime7d         float64  `json:"uptime_7d,omitempty"`
	Uptime30d        float64  `json:"uptime_30d,omitempty"`
	RecentLogs       []string `json:"recent_logs,omitempty"`
}

// Log represents a check result entry
type Log struct {
	ID               int64     `json:"id"`
	MonitorID        int64     `json:"monitor_id"`
	Status           string    `json:"status"` // "up" or "down"
	ResponseTime     int       `json:"response_time"`
	StatusCode       int       `json:"status_code"`
	Message          string    `json:"message"`
	SslDaysRemaining *int      `json:"ssl_days_remaining"`
	CreatedAt        time.Time `json:"created_at"`
}

// Session matches cookies to logged-in users
type Session struct {
	Token     string    `json:"token"`
	UserID    int64     `json:"user_id"`
	ExpiresAt time.Time `json:"expires_at"`
}

// SystemSettings holds key-value settings for customizing the server and notifications
type SystemSettings struct {
	PublicTitle        string `json:"public_title"`
	PublicAnnouncement string `json:"public_announcement"`
	PublicLogo         string `json:"public_logo"`

	SMTPEnabled bool   `json:"smtp_enabled"`
	SMTPHost    string `json:"smtp_host"`
	SMTPPort    int    `json:"smtp_port"`
	SMTPUser    string `json:"smtp_user"`
	SMTPPass    string `json:"smtp_pass"`
	SMTPFrom    string `json:"smtp_from"`
	SMTPTo      string `json:"smtp_to"`

	DiscordEnabled bool   `json:"discord_enabled"`
	DiscordWebhook string `json:"discord_webhook"`

	TelegramEnabled bool   `json:"telegram_enabled"`
	TelegramToken   string `json:"telegram_token"`
	TelegramChatID  string `json:"telegram_chat_id"`

	AlertSubjectDown string `json:"alert_subject_down"`
	AlertBodyDown    string `json:"alert_body_down"`
	AlertSubjectUp   string `json:"alert_subject_up"`
	AlertBodyUp      string `json:"alert_body_up"`

	SMTPAlertOnDown      bool   `json:"smtp_alert_on_down"`
	SMTPAlertOnUp        bool   `json:"smtp_alert_on_up"`
	SMTPAlertRepeat      bool   `json:"smtp_alert_repeat"`

	DiscordAlertOnDown   bool   `json:"discord_alert_on_down"`
	DiscordAlertOnUp     bool   `json:"discord_alert_on_up"`
	DiscordAlertRepeat   bool   `json:"discord_alert_repeat"`

	TelegramAlertOnDown  bool   `json:"telegram_alert_on_down"`
	TelegramAlertOnUp    bool   `json:"telegram_alert_on_up"`
	TelegramAlertRepeat  bool   `json:"telegram_alert_repeat"`
}

// GetSystemSettings queries settings key from DB and unmarshals or returns default
func LoadSettings(dbConn *sql.DB) (*SystemSettings, error) {
	s := &SystemSettings{
		PublicTitle:         "UptimeGo Status Page",
		SMTPAlertOnDown:     true,
		SMTPAlertOnUp:       true,
		SMTPAlertRepeat:     true,
		DiscordAlertOnDown:   true,
		DiscordAlertOnUp:     true,
		DiscordAlertRepeat:   true,
		TelegramAlertOnDown:  true,
		TelegramAlertOnUp:    true,
		TelegramAlertRepeat:  true,
	}

	rows, err := dbConn.Query("SELECT key, value FROM settings")
	if err != nil {
		return s, err
	}
	defer rows.Close()

	for rows.Next() {
		var key, val string
		if err := rows.Scan(&key, &val); err != nil {
			continue
		}
		switch key {
		case "public_title":
			s.PublicTitle = val
		case "public_announcement":
			s.PublicAnnouncement = val
		case "public_logo":
			s.PublicLogo = val
		case "smtp_enabled":
			s.SMTPEnabled = val == "true"
		case "smtp_host":
			s.SMTPHost = val
		case "smtp_port":
			var port int
			fmtSscanf(val, "%d", &port)
			s.SMTPPort = port
		case "smtp_user":
			s.SMTPUser = val
		case "smtp_pass":
			s.SMTPPass = val
		case "smtp_from":
			s.SMTPFrom = val
		case "smtp_to":
			s.SMTPTo = val
		case "discord_enabled":
			s.DiscordEnabled = val == "true"
		case "discord_webhook":
			s.DiscordWebhook = val
		case "telegram_enabled":
			s.TelegramEnabled = val == "true"
		case "telegram_token":
			s.TelegramToken = val
		case "telegram_chat_id":
			s.TelegramChatID = val
		case "alert_subject_down":
			s.AlertSubjectDown = val
		case "alert_body_down":
			s.AlertBodyDown = val
		case "alert_subject_up":
			s.AlertSubjectUp = val
		case "alert_body_up":
			s.AlertBodyUp = val
		case "smtp_alert_on_down":
			s.SMTPAlertOnDown = val != "false"
		case "smtp_alert_on_up":
			s.SMTPAlertOnUp = val != "false"
		case "smtp_alert_repeat":
			s.SMTPAlertRepeat = val != "false"
		case "discord_alert_on_down":
			s.DiscordAlertOnDown = val != "false"
		case "discord_alert_on_up":
			s.DiscordAlertOnUp = val != "false"
		case "discord_alert_repeat":
			s.DiscordAlertRepeat = val != "false"
		case "telegram_alert_on_down":
			s.TelegramAlertOnDown = val != "false"
		case "telegram_alert_on_up":
			s.TelegramAlertOnUp = val != "false"
		case "telegram_alert_repeat":
			s.TelegramAlertRepeat = val != "false"
		}
	}

	// Apply default messages if none are defined
	if s.AlertSubjectDown == "" {
		s.AlertSubjectDown = "🔴 Servis Çevrimdışı: {name}"
	}
	if s.AlertBodyDown == "" {
		s.AlertBodyDown = "⚠️ **{name}** ({target}) servisi ÇEVRİMDışı durumuna geçti.\n⏱️ Gecikme: {latency}ms\n🔍 Hata Detayı: {message}"
	}
	if s.AlertSubjectUp == "" {
		s.AlertSubjectUp = "🟢 Servis Çevrimiçi: {name}"
	}
	if s.AlertBodyUp == "" {
		s.AlertBodyUp = "✅ **{name}** ({target}) servisi tekrar çevrimiçi.\n⏱️ Gecikme: {latency}ms"
	}

	return s, nil
}

func fmtSscanf(str string, format string, a ...interface{}) {
	// A safe sscanf helper
	_, _ = fmtSscanfHelper(str, format, a...)
}

func fmtSscanfHelper(str string, format string, a ...interface{}) (int, error) {
	// Helper to avoid circular package issues or heavy code
	var val int
	_, err := scanNumber(str, &val)
	if err == nil && len(a) > 0 {
		if p, ok := a[0].(*int); ok {
			*p = val
		}
	}
	return 1, nil
}

func scanNumber(str string, val *int) (int, error) {
	res := 0
	for i := 0; i < len(str); i++ {
		if str[i] >= '0' && str[i] <= '9' {
			res = res*10 + int(str[i]-'0')
		} else {
			break
		}
	}
	*val = res
	return res, nil
}
