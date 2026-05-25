package handlers

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"time"
	"uptimego/db"
	"uptimego/i18n"
	"uptimego/models"
	"uptimego/notifier"
	"uptimego/scheduler"

	"golang.org/x/crypto/bcrypt"
)

var Tmpl *template.Template

// PageContext carries common variables to templates
type PageContext struct {
	Lang      string
	Title     string
	User      *models.User
	Settings  *models.SystemSettings
	Data      interface{}
	Success   string
	Error     string
}

// InitTemplates compiles the embedded html templates
func InitTemplates(files io.Reader, parseFn func() *template.Template) {
	Tmpl = parseFn()
}

// GenerateRandomToken creates cryptographically secure hex string
func GenerateRandomToken() (string, error) {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

// SetupCheckMiddleware redirects to /setup if no users exist in the DB
func SetupCheckMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var count int
		err := db.DB.QueryRow("SELECT COUNT(*) FROM users").Scan(&count)
		if err != nil {
			http.Error(w, "Database error", http.StatusInternalServerError)
			return
		}

		if count == 0 && r.URL.Path != "/setup" && !strings.HasPrefix(r.URL.Path, "/api/setup") && !strings.HasPrefix(r.URL.Path, "/static/") {
			http.Redirect(w, r, "/setup", http.StatusFound)
			return
		}

		if count > 0 && r.URL.Path == "/setup" {
			http.Redirect(w, r, "/login", http.StatusFound)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// GetUserFromSession fetches user by token cookie
func GetUserFromSession(r *http.Request) (*models.User, error) {
	cookie, err := r.Cookie("session_token")
	if err != nil {
		return nil, err
	}

	var u models.User
	var expiresAt time.Time
	err = db.DB.QueryRow(`
		SELECT u.id, u.username, u.role, u.api_key, s.expires_at 
		FROM sessions s 
		JOIN users u ON s.user_id = u.id 
		WHERE s.token = ?`, cookie.Value).Scan(&u.ID, &u.Username, &u.Role, &u.APIKey, &expiresAt)
	if err != nil {
		return nil, err
	}

	if time.Now().After(expiresAt) {
		// Clean up expired session
		_, _ = db.DB.Exec("DELETE FROM sessions WHERE token = ?", cookie.Value)
		return nil, fmt.Errorf("session expired")
	}

	return &u, nil
}

// RequireAuth middleware protects routes based on RBAC roles
func RequireAuth(minRole models.Role) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user, err := GetUserFromSession(r)
			if err != nil {
				if strings.HasPrefix(r.URL.Path, "/api/") {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusUnauthorized)
					_, _ = w.Write([]byte(`{"error": "Unauthorized"}`))
					return
				}
				http.Redirect(w, r, "/login", http.StatusFound)
				return
			}

			// Validate role permission
			authorized := false
			switch minRole {
			case models.RoleSuperAdmin:
				authorized = user.Role == models.RoleSuperAdmin
			case models.RoleAdmin:
				authorized = user.Role == models.RoleSuperAdmin || user.Role == models.RoleAdmin
			case models.RoleViewer:
				authorized = user.Role == models.RoleSuperAdmin || user.Role == models.RoleAdmin || user.Role == models.RoleViewer
			}

			if !authorized {
				if strings.HasPrefix(r.URL.Path, "/api/") {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusForbidden)
					_, _ = w.Write([]byte(`{"error": "Forbidden"}`))
					return
				}
				http.Error(w, "Forbidden", http.StatusForbidden)
				return
			}

			// Add user to request context
			r = r.WithContext(insertUserInContext(r.Context(), user))
			next.ServeHTTP(w, r)
		})
	}
}

type ctxUserKey struct{}

func insertUserInContext(ctx context.Context, u *models.User) context.Context {
	return context.WithValue(ctx, ctxUserKey{}, u)
}

func GetUser(r *http.Request) *models.User {
	if u, ok := r.Context().Value(ctxUserKey{}).(*models.User); ok {
		return u
	}
	return nil
}

// Setup Handlers
func HandleSetupPage(w http.ResponseWriter, r *http.Request) {
	lang := i18n.FromContext(r.Context())
	_ = Tmpl.ExecuteTemplate(w, "setup.html", PageContext{
		Lang:  lang,
		Title: i18n.T(lang, "setup.title"),
	})
}

func HandleSetupSubmit(w http.ResponseWriter, r *http.Request) {
	lang := i18n.FromContext(r.Context())
	w.Header().Set("Content-Type", "application/json")

	// Verify no users exist
	var count int
	_ = db.DB.QueryRow("SELECT COUNT(*) FROM users").Scan(&count)
	if count > 0 {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(fmt.Sprintf(`{"error": "%s"}`, i18n.T(lang, "setup.error.exists"))))
		return
	}

	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error": "Invalid JSON"}`))
		return
	}

	username := strings.ToLower(strings.TrimSpace(req.Username))
	password := req.Password

	if username == "" || len(password) < 6 {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(fmt.Sprintf(`{"error": "%s"}`, i18n.T(lang, "setup.error.fields"))))
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error": "Hashing error"}`))
		return
	}

	_, err = db.DB.Exec(`
		INSERT INTO users (username, password_hash, role) 
		VALUES (?, ?, ?)`, username, string(hash), string(models.RoleSuperAdmin))
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error": "Database write error"}`))
		return
	}

	// Create default settings row
	_, _ = db.DB.Exec("INSERT OR IGNORE INTO settings (key, value) VALUES ('public_title', 'UptimeGo Status')")

	_, _ = w.Write([]byte(`{"success": true}`))
}

// Login/Logout Handlers
func HandleLoginPage(w http.ResponseWriter, r *http.Request) {
	lang := i18n.FromContext(r.Context())
	_ = Tmpl.ExecuteTemplate(w, "login.html", PageContext{
		Lang:  lang,
		Title: i18n.T(lang, "login.title"),
	})
}

func HandleLoginSubmit(w http.ResponseWriter, r *http.Request) {
	lang := i18n.FromContext(r.Context())
	w.Header().Set("Content-Type", "application/json")

	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error": "Invalid request"}`))
		return
	}

	username := strings.ToLower(strings.TrimSpace(req.Username))

	var id int64
	var hash string
	var role models.Role

	err := db.DB.QueryRow("SELECT id, password_hash, role FROM users WHERE LOWER(username) = ?", username).Scan(&id, &hash, &role)
	if err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(fmt.Sprintf(`{"error": "%s"}`, i18n.T(lang, "login.error.invalid"))))
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(req.Password)); err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(fmt.Sprintf(`{"error": "%s"}`, i18n.T(lang, "login.error.invalid"))))
		return
	}

	// Generate Session Token
	token, err := GenerateRandomToken()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error": "Token generation failed"}`))
		return
	}

	expiresAt := time.Now().Add(24 * time.Hour)
	_, err = db.DB.Exec("INSERT INTO sessions (token, user_id, expires_at) VALUES (?, ?, ?)", token, id, expiresAt)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error": "Session save error"}`))
		return
	}

	// Set HttpOnly secure cookie dynamically based on TLS or proxy headers (avoid secure cookie for IP addresses to support HTTP tests)
	host := r.Host
	if shost, _, err := net.SplitHostPort(r.Host); err == nil {
		host = shost
	}
	isIP := net.ParseIP(host) != nil
	secure := (r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https") && !isIP

	http.SetCookie(w, &http.Cookie{
		Name:     "session_token",
		Value:    token,
		Expires:  expiresAt,
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
		Path:     "/",
	})

	_, _ = w.Write([]byte(`{"success": true}`))
}

func HandleLogout(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie("session_token")
	if err == nil {
		_, _ = db.DB.Exec("DELETE FROM sessions WHERE token = ?", cookie.Value)
	}

	// Clear Cookie
	http.SetCookie(w, &http.Cookie{
		Name:     "session_token",
		Value:    "",
		Expires:  time.Now().Add(-1 * time.Hour),
		HttpOnly: true,
		Path:     "/",
	})

	http.Redirect(w, r, "/login", http.StatusFound)
}

// Language Switcher Handler
func HandleLanguageSwitch(w http.ResponseWriter, r *http.Request) {
	lang := r.URL.Query().Get("lang")
	if lang == "tr" || lang == "en" {
		http.SetCookie(w, &http.Cookie{
			Name:     "lang",
			Value:    lang,
			Expires:  time.Now().Add(365 * 24 * time.Hour),
			Path:     "/",
			HttpOnly: false,
		})
	}
	// Redirect back to referrer
	ref := r.Header.Get("Referer")
	if ref == "" {
		ref = "/"
	}
	http.Redirect(w, r, ref, http.StatusFound)
}

// Dashboard Page
func HandleDashboardPage(w http.ResponseWriter, r *http.Request) {
	lang := i18n.FromContext(r.Context())
	user := GetUser(r)

	settings, _ := models.LoadSettings(db.DB)

	// Fetch monitors
	rows, err := db.DB.Query(`
		SELECT id, name, type, target, interval, timeout, retries, alert_interval, active, keyword, ssl_expiry_warning, public, status, created_at, updated_at
		FROM monitors ORDER BY name ASC`)
	if err != nil {
		http.Error(w, "Query error", http.StatusInternalServerError)
		return
	}
	
	var monitors []models.Monitor
	for rows.Next() {
		var m models.Monitor
		err := rows.Scan(
			&m.ID, &m.Name, &m.Type, &m.Target, &m.Interval, &m.Timeout, &m.Retries, &m.AlertInterval, &m.Active, &m.Keyword, &m.SslExpiryWarning, &m.Public, &m.Status, &m.CreatedAt, &m.UpdatedAt,
		)
		if err == nil {
			monitors = append(monitors, m)
		}
	}
	rows.Close() // Close rows immediately to release connection

	totalUp, totalDown := 0, 0
	for i := range monitors {
		// Get calculated stats
		stats := calculateMonitorStats(monitors[i].ID)
		monitors[i].Uptime24h = stats.Uptime24h
		monitors[i].Uptime7d = stats.Uptime7d
		monitors[i].Uptime30d = stats.Uptime30d
		monitors[i].LastResponseTime = stats.LastLatency
		monitors[i].LastChecked = stats.LastChecked
		monitors[i].SslDaysRemaining = stats.SslDays
		monitors[i].RecentLogs = stats.RecentLogs

		if monitors[i].Active {
			if monitors[i].Status == "up" {
				totalUp++
			} else if monitors[i].Status == "down" {
				totalDown++
			}
		}
	}

	type DashboardData struct {
		Monitors   []models.Monitor
		TotalMon   int
		TotalUp    int
		TotalDown  int
		AvgLatency int
	}

	// Calculate average latency
	var avgLatency int
	_ = db.DB.QueryRow("SELECT CAST(AVG(response_time) AS INTEGER) FROM monitor_logs WHERE status = 'up' AND created_at > datetime('now', '-24 hours')").Scan(&avgLatency)

	_ = Tmpl.ExecuteTemplate(w, "dashboard.html", PageContext{
		Lang:     lang,
		Title:    i18n.T(lang, "dashboard.title"),
		User:     user,
		Settings: settings,
		Data: DashboardData{
			Monitors:   monitors,
			TotalMon:   len(monitors),
			TotalUp:    totalUp,
			TotalDown:  totalDown,
			AvgLatency: avgLatency,
		},
	})
}

// API - Monitors CRUD
func HandleAPIMonitorList(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	rows, err := db.DB.Query(`SELECT id, name, type, target, active, status FROM monitors`)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error": "DB query failed"}`))
		return
	}
	defer rows.Close()

	var list []map[string]interface{}
	for rows.Next() {
		var id int64
		var name, mType, target, status string
		var active bool
		if err := rows.Scan(&id, &name, &mType, &target, &active, &status); err == nil {
			list = append(list, map[string]interface{}{
				"id":     id,
				"name":   name,
				"type":   mType,
				"target": target,
				"active": active,
				"status": status,
			})
		}
	}
	_ = json.NewEncoder(w).Encode(list)
}

func HandleAPIMonitorSave(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	
	var req struct {
		ID               int64  `json:"id"`
		Name             string `json:"name"`
		Type             string `json:"type"`
		Target           string `json:"target"`
		Interval         int    `json:"interval"`
		Timeout          int    `json:"timeout"`
		Retries          int    `json:"retries"`
		AlertInterval    int    `json:"alert_interval"`
		Active           bool   `json:"active"`
		Keyword          string `json:"keyword"`
		SslExpiryWarning bool   `json:"ssl_expiry_warning"`
		Public           bool   `json:"public"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error": "Invalid request"}`))
		return
	}

	// Auto-prefix URL protocol for HTTP(S) monitors
	if req.Type == "http" {
		target := strings.TrimSpace(req.Target)
		if target != "" && !strings.Contains(target, "://") {
			target = "http://" + target
		}
		req.Target = target
	} else if req.Type == "ping" {
		req.Target = strings.TrimSpace(req.Target)
	}

	if req.Name == "" || req.Target == "" || req.Interval < 5 || req.Timeout < 100 {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error": "Validation failed: Check intervals (min 5s), timeouts (min 100ms) and required fields"}`))
		return
	}

	activeInt := 0
	if req.Active {
		activeInt = 1
	}
	sslInt := 0
	if req.SslExpiryWarning {
		sslInt = 1
	}
	pubInt := 0
	if req.Public {
		pubInt = 1
	}

	var err error
	var monitorID int64

	if req.ID > 0 {
		// Update
		monitorID = req.ID
		_, err = db.DB.Exec(`
			UPDATE monitors 
			SET name=?, type=?, target=?, interval=?, timeout=?, retries=?, alert_interval=?, active=?, keyword=?, ssl_expiry_warning=?, public=?, updated_at=CURRENT_TIMESTAMP 
			WHERE id=?`,
			req.Name, req.Type, req.Target, req.Interval, req.Timeout, req.Retries, req.AlertInterval, activeInt, req.Keyword, sslInt, pubInt, req.ID,
		)
	} else {
		// Insert
		res, insErr := db.DB.Exec(`
			INSERT INTO monitors (name, type, target, interval, timeout, retries, alert_interval, active, keyword, ssl_expiry_warning, public, status) 
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'unknown')`,
			req.Name, req.Type, req.Target, req.Interval, req.Timeout, req.Retries, req.AlertInterval, activeInt, req.Keyword, sslInt, pubInt,
		)
		if insErr == nil {
			monitorID, _ = res.LastInsertId()
		}
		err = insErr
	}

	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(fmt.Sprintf(`{"error": "Failed to save monitor: %v"}`, err)))
		return
	}

	// Update Scheduler worker
	if req.Active {
		scheduler.StartMonitor(monitorID)
	} else {
		scheduler.StopMonitor(monitorID)
		_, _ = db.DB.Exec("UPDATE monitors SET status = 'unknown' WHERE id = ?", monitorID)
	}

	_, _ = w.Write([]byte(`{"success": true}`))
}

func HandleAPIMonitorDelete(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	idStr := r.URL.Query().Get("id")
	id, _ := strconvParseInt64(idStr)

	if id <= 0 {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error": "Missing ID"}`))
		return
	}

	scheduler.StopMonitor(id)
	_, err := db.DB.Exec("DELETE FROM monitors WHERE id = ?", id)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error": "Delete failed"}`))
		return
	}

	_, _ = w.Write([]byte(`{"success": true}`))
}

func HandleAPIMonitorToggle(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	idStr := r.URL.Query().Get("id")
	id, _ := strconvParseInt64(idStr)

	var active int
	err := db.DB.QueryRow("SELECT active FROM monitors WHERE id = ?", id).Scan(&active)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	newActive := 1 - active
	_, _ = db.DB.Exec("UPDATE monitors SET active = ?, status = 'unknown' WHERE id = ?", newActive, id)

	if newActive == 1 {
		scheduler.StartMonitor(id)
	} else {
		scheduler.StopMonitor(id)
	}

	_, _ = w.Write([]byte(`{"success": true}`))
}

// API - Monitor Latency history for Chart.js
func HandleAPIMonitorHistory(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	idStr := r.URL.Query().Get("id")
	id, _ := strconvParseInt64(idStr)

	// Fetch last 30 logs
	rows, err := db.DB.Query(`
		SELECT response_time, created_at 
		FROM monitor_logs 
		WHERE monitor_id = ? 
		ORDER BY created_at DESC LIMIT 30`, id)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	type HistoryPoint struct {
		Time    string `json:"time"`
		Latency int    `json:"latency"`
	}

	var points []HistoryPoint
	for rows.Next() {
		var l int
		var t time.Time
		if err := rows.Scan(&l, &t); err == nil {
			// Format time to show hours and minutes local
			timeStr := t.Local().Format("15:04")
			points = append([]HistoryPoint{{Time: timeStr, Latency: l}}, points...) // prepend to keep chronological
		}
	}
	_ = json.NewEncoder(w).Encode(points)
}

// Settings Page
func HandleSettingsPage(w http.ResponseWriter, r *http.Request) {
	lang := i18n.FromContext(r.Context())
	user := GetUser(r)
	sysSettings, _ := models.LoadSettings(db.DB)

	// Fetch users list (for Super Admin only)
	var listUsers []models.User
	if user.Role == models.RoleSuperAdmin {
		rows, err := db.DB.Query("SELECT id, username, role, created_at FROM users WHERE id != ? ORDER BY id ASC", user.ID)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var u models.User
				if err := rows.Scan(&u.ID, &u.Username, &u.Role, &u.CreatedAt); err == nil {
					listUsers = append(listUsers, u)
				}
			}
		}
	}

	type SettingsPageData struct {
		SysSettings *models.SystemSettings
		Users       []models.User
	}

	err := Tmpl.ExecuteTemplate(w, "settings.html", PageContext{
		Lang:     lang,
		Title:    i18n.T(lang, "settings.title"),
		User:     user,
		Settings: sysSettings,
		Data: SettingsPageData{
			SysSettings: sysSettings,
			Users:       listUsers,
		},
	})
	if err != nil {
		log.Printf("[Error] Failed to render settings.html: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func HandleAPISettingsSave(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	
	var req struct {
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

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error": "Invalid request"}`))
		return
	}

	// Update DB transactions
	saveKeyVal := func(key string, val string) {
		_, _ = db.DB.Exec("INSERT OR REPLACE INTO settings (key, value) VALUES (?, ?)", key, val)
	}
	saveBool := func(key string, val bool) {
		str := "false"
		if val {
			str = "true"
		}
		saveKeyVal(key, str)
	}

	saveKeyVal("public_title", req.PublicTitle)
	saveKeyVal("public_announcement", req.PublicAnnouncement)
	saveKeyVal("public_logo", req.PublicLogo)

	smtpEnabledStr := "false"
	if req.SMTPEnabled {
		smtpEnabledStr = "true"
	}
	saveKeyVal("smtp_enabled", smtpEnabledStr)
	saveKeyVal("smtp_host", req.SMTPHost)
	saveKeyVal("smtp_port", fmt.Sprintf("%d", req.SMTPPort))
	saveKeyVal("smtp_user", req.SMTPUser)
	// Don't overwrite password if sent empty (helps keep it masked)
	if req.SMTPPass != "" {
		saveKeyVal("smtp_pass", req.SMTPPass)
	}
	saveKeyVal("smtp_from", req.SMTPFrom)
	saveKeyVal("smtp_to", req.SMTPTo)

	discordEnabledStr := "false"
	if req.DiscordEnabled {
		discordEnabledStr = "true"
	}
	saveKeyVal("discord_enabled", discordEnabledStr)
	saveKeyVal("discord_webhook", req.DiscordWebhook)

	telegramEnabledStr := "false"
	if req.TelegramEnabled {
		telegramEnabledStr = "true"
	}
	saveKeyVal("telegram_enabled", telegramEnabledStr)
	saveKeyVal("telegram_token", req.TelegramToken)
	saveKeyVal("telegram_chat_id", req.TelegramChatID)

	saveKeyVal("alert_subject_down", req.AlertSubjectDown)
	saveKeyVal("alert_body_down", req.AlertBodyDown)
	saveKeyVal("alert_subject_up", req.AlertSubjectUp)
	saveKeyVal("alert_body_up", req.AlertBodyUp)

	saveBool("smtp_alert_on_down", req.SMTPAlertOnDown)
	saveBool("smtp_alert_on_up", req.SMTPAlertOnUp)
	saveBool("smtp_alert_repeat", req.SMTPAlertRepeat)

	saveBool("discord_alert_on_down", req.DiscordAlertOnDown)
	saveBool("discord_alert_on_up", req.DiscordAlertOnUp)
	saveBool("discord_alert_repeat", req.DiscordAlertRepeat)

	saveBool("telegram_alert_on_down", req.TelegramAlertOnDown)
	saveBool("telegram_alert_on_up", req.TelegramAlertOnUp)
	saveBool("telegram_alert_repeat", req.TelegramAlertRepeat)

	_, _ = w.Write([]byte(`{"success": true}`))
}

func HandleAPISettingsTestAlert(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	var req struct {
		Channel string `json:"channel"`

		SMTPHost string `json:"smtp_host"`
		SMTPPort int    `json:"smtp_port"`
		SMTPUser string `json:"smtp_user"`
		SMTPPass string `json:"smtp_pass"`
		SMTPFrom string `json:"smtp_from"`
		SMTPTo   string `json:"smtp_to"`

		TelegramToken  string `json:"telegram_token"`
		TelegramChatID string `json:"telegram_chat_id"`

		DiscordWebhook string `json:"discord_webhook"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error": "Invalid request"}`))
		return
	}

	var testErr error
	switch req.Channel {
	case "smtp":
		password := req.SMTPPass
		if password == "" {
			var savedPass string
			_ = db.DB.QueryRow("SELECT value FROM settings WHERE key = 'smtp_pass'").Scan(&savedPass)
			password = savedPass
		}
		testErr = notifier.TestSMTP(req.SMTPHost, req.SMTPPort, req.SMTPUser, password, req.SMTPFrom, req.SMTPTo)
	case "telegram":
		testErr = notifier.TestTelegram(req.TelegramToken, req.TelegramChatID)
	case "discord":
		testErr = notifier.TestDiscord(req.DiscordWebhook)
	default:
		testErr = fmt.Errorf("unknown channel: %s", req.Channel)
	}

	if testErr != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": testErr.Error()})
		return
	}

	_, _ = w.Write([]byte(`{"success": true}`))
}

// User CRUD (Super Admin only)
func HandleAPIUserAdd(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var req struct {
		Username string      `json:"username"`
		Password string      `json:"password"`
		Role     models.Role `json:"role"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	username := strings.ToLower(strings.TrimSpace(req.Username))
	if username == "" || len(req.Password) < 6 || req.Role != models.RoleAdmin {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error": "Invalid username, password (min 6 characters), or role choice (must be admin)"}`))
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// Generate a secure random API key for the new user immediately
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	apiKey := hex.EncodeToString(b)

	_, err = db.DB.Exec("INSERT INTO users (username, password_hash, role, api_key) VALUES (?, ?, ?, ?)", username, string(hash), string(req.Role), apiKey)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error": "Username already exists"}`))
		return
	}

	_, _ = w.Write([]byte(`{"success": true}`))
}

func HandleAPIUserDelete(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	idStr := r.URL.Query().Get("id")
	id, _ := strconvParseInt64(idStr)

	if id <= 0 {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Double check user doesn't delete themselves
	currentAdmin := GetUser(r)
	if currentAdmin.ID == id {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error": "You cannot delete your own user account!"}`))
		return
	}

	_, err := db.DB.Exec("DELETE FROM users WHERE id = ?", id)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	_, _ = w.Write([]byte(`{"success": true}`))
}

func HandleAPIUserProfileSave(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	user := GetUser(r)
	if user == nil {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error": "Unauthorized"}`))
		return
	}

	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error": "Invalid request"}`))
		return
	}

	username := strings.ToLower(strings.TrimSpace(req.Username))
	if username == "" {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error": "Username cannot be empty"}`))
		return
	}

	// Check if the username is taken by someone else
	var count int
	err := db.DB.QueryRow("SELECT COUNT(*) FROM users WHERE LOWER(username) = ? AND id != ?", username, user.ID).Scan(&count)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error": "Database error"}`))
		return
	}
	if count > 0 {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error": "Username already exists"}`))
		return
	}

	if req.Password != "" && len(req.Password) < 6 {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error": "Password must be at least 6 characters long"}`))
		return
	}

	if req.Password != "" {
		hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error": "Hashing failed"}`))
			return
		}
		_, err = db.DB.Exec("UPDATE users SET username = ?, password_hash = ? WHERE id = ?", username, string(hash), user.ID)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error": "Database update failed"}`))
			return
		}
	} else {
		_, err = db.DB.Exec("UPDATE users SET username = ? WHERE id = ?", username, user.ID)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error": "Database update failed"}`))
			return
		}
	}

	_, _ = w.Write([]byte(`{"success": true}`))
}

// Public Status Page Handler
func HandlePublicStatusPage(w http.ResponseWriter, r *http.Request) {
	lang := i18n.FromContext(r.Context())
	settings, _ := models.LoadSettings(db.DB)

	rows, err := db.DB.Query(`
		SELECT id, name, type, target, active, status, created_at, updated_at
		FROM monitors WHERE public = 1 ORDER BY name ASC`)
	if err != nil {
		http.Error(w, "Query error", http.StatusInternalServerError)
		return
	}
	
	var publicMonitors []models.Monitor
	for rows.Next() {
		var m models.Monitor
		err := rows.Scan(
			&m.ID, &m.Name, &m.Type, &m.Target, &m.Active, &m.Status, &m.CreatedAt, &m.UpdatedAt,
		)
		if err == nil {
			publicMonitors = append(publicMonitors, m)
		}
	}
	rows.Close() // Close rows immediately to release connection

	allUp := true
	anyDown := false
	anyActive := len(publicMonitors) > 0

	for i := range publicMonitors {
		stats := calculateMonitorStats(publicMonitors[i].ID)
		publicMonitors[i].Uptime24h = stats.Uptime24h
		publicMonitors[i].Uptime7d = stats.Uptime7d
		publicMonitors[i].Uptime30d = stats.Uptime30d
		publicMonitors[i].LastResponseTime = stats.LastLatency
		publicMonitors[i].LastChecked = stats.LastChecked
		publicMonitors[i].SslDaysRemaining = stats.SslDays
		publicMonitors[i].RecentLogs = stats.RecentLogs

		if publicMonitors[i].Active {
			if publicMonitors[i].Status == "down" {
				anyDown = true
				allUp = false
			} else if publicMonitors[i].Status == "unknown" {
				allUp = false
			}
		}
	}

	statusSummary := "all_operational"
	if !anyActive {
		statusSummary = "unknown"
	} else if anyDown {
		if allUp {
			statusSummary = "some_issues"
		} else {
			// All down? Or some down
			// Check if ALL are down
			allDownFlag := true
			for _, m := range publicMonitors {
				if m.Active && m.Status == "up" {
					allDownFlag = false
					break
				}
			}
			if allDownFlag {
				statusSummary = "all_down"
			} else {
				statusSummary = "some_issues"
			}
		}
	}

	type StatusPageData struct {
		Monitors []models.Monitor
		Summary  string
	}

	_ = Tmpl.ExecuteTemplate(w, "status.html", PageContext{
		Lang:     lang,
		Title:    settings.PublicTitle,
		Settings: settings,
		Data: StatusPageData{
			Monitors: publicMonitors,
			Summary:  statusSummary,
		},
	})
}

// Stats calculation helper structures
type monitorStats struct {
	Uptime24h   float64
	Uptime7d    float64
	Uptime30d   float64
	LastLatency int
	LastChecked string
	SslDays     *int
	RecentLogs  []string
}

func calculateMonitorStats(monitorID int64) monitorStats {
	stats := monitorStats{Uptime24h: 100.0, Uptime7d: 100.0, Uptime30d: 100.0}

	// 1. Fetch last response time and check time
	var lastTime time.Time
	err := db.DB.QueryRow(`
		SELECT response_time, ssl_days_remaining, created_at 
		FROM monitor_logs 
		WHERE monitor_id = ? 
		ORDER BY created_at DESC LIMIT 1`, monitorID).Scan(&stats.LastLatency, &stats.SslDays, &lastTime)
	
	if err == nil {
		stats.LastChecked = lastTime.Local().Format("15:04:05")
	}

	// 2. Fetch Uptime percentages
	uptimeQuery := func(days int) float64 {
		var total, up int
		err := db.DB.QueryRow(`
			SELECT 
				COUNT(*),
				SUM(CASE WHEN status = 'up' THEN 1 ELSE 0 END) 
			FROM monitor_logs 
			WHERE monitor_id = ? AND created_at > datetime('now', ?)`,
			monitorID, fmt.Sprintf("-%d days", days),
		).Scan(&total, &up)

		if err != nil || total == 0 {
			return 100.0
		}
		return (float64(up) / float64(total)) * 100.0
	}

	stats.Uptime24h = uptimeQuery(1)
	stats.Uptime7d = uptimeQuery(7)
	stats.Uptime30d = uptimeQuery(30)

	// 3. Fetch last 30 logs for the dot grid
	logRows, err := db.DB.Query(`
		SELECT status FROM monitor_logs 
		WHERE monitor_id = ? 
		ORDER BY created_at DESC LIMIT 30`, monitorID)
	if err == nil {
		var recent []string
		for logRows.Next() {
			var status string
			if err := logRows.Scan(&status); err == nil {
				recent = append(recent, status)
			}
		}
		logRows.Close()

		// Reverse it so chronological order is left-to-right (oldest to newest)
		for i, j := 0, len(recent)-1; i < j; i, j = i+1, j-1 {
			recent[i], recent[j] = recent[j], recent[i]
		}
		stats.RecentLogs = recent
	}

	return stats
}

// Helpers
func strconvParseInt64(s string) (int64, error) {
	var val int64
	_, err := fmt.Sscanf(s, "%d", &val)
	return val, err
}

func APIKeyAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		apiKey := r.Header.Get("X-API-Key")
		if apiKey == "" {
			apiKey = r.URL.Query().Get("api_key")
		}

		if apiKey == "" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error": "API Key is missing"}`))
			return
		}

		var u models.User
		err := db.DB.QueryRow("SELECT id, username, role FROM users WHERE api_key = ?", apiKey).Scan(&u.ID, &u.Username, &u.Role)
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error": "Invalid API Key"}`))
			return
		}

		r = r.WithContext(insertUserInContext(r.Context(), &u))
		next.ServeHTTP(w, r)
	})
}

func HandleAPIAPIKeyReset(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	user := GetUser(r)
	if user == nil {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error": "Unauthorized"}`))
		return
	}

	b := make([]byte, 16)
	_, _ = rand.Read(b)
	newKey := hex.EncodeToString(b)

	_, err := db.DB.Exec("UPDATE users SET api_key = ? WHERE id = ?", newKey, user.ID)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(fmt.Sprintf(`{"error": "Database error: %v"}`, err)))
		return
	}

	_ = json.NewEncoder(w).Encode(map[string]string{"api_key": newKey})
}

func HandleAPIV1MonitorList(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	rows, err := db.DB.Query(`
		SELECT id, name, type, target, interval, timeout, retries, alert_interval, active, keyword, ssl_expiry_warning, public, status 
		FROM monitors ORDER BY name ASC`)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error": "Query failed"}`))
		return
	}
	defer rows.Close()

	var list []models.Monitor
	for rows.Next() {
		var m models.Monitor
		err := rows.Scan(
			&m.ID, &m.Name, &m.Type, &m.Target, &m.Interval, &m.Timeout, &m.Retries, &m.AlertInterval, &m.Active, &m.Keyword, &m.SslExpiryWarning, &m.Public, &m.Status,
		)
		if err == nil {
			list = append(list, m)
		}
	}

	if list == nil {
		list = []models.Monitor{}
	}
	_ = json.NewEncoder(w).Encode(list)
}

func HandleAPIV1MonitorSave(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	user := GetUser(r)
	if user.Role == models.RoleViewer {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error": "Viewer role cannot write monitors"}`))
		return
	}
	HandleAPIMonitorSave(w, r)
}

func HandleAPIV1MonitorDelete(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	user := GetUser(r)
	if user.Role == models.RoleViewer {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error": "Viewer role cannot delete monitors"}`))
		return
	}
	HandleAPIMonitorDelete(w, r)
}

// LoggingMiddleware wraps http.ResponseWriter to capture status code and log HTTP request details
func LoggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		
		// Custom response writer to capture status code
		lrw := &loggingResponseWriter{ResponseWriter: w, statusCode: http.StatusOK}
		
		next.ServeHTTP(lrw, r)
		
		log.Printf("[HTTP] %s %s - Status: %d - Duration: %v", r.Method, r.URL.Path, lrw.statusCode, time.Since(start))
	})
}

type loggingResponseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (lrw *loggingResponseWriter) WriteHeader(code int) {
	lrw.statusCode = code
	lrw.ResponseWriter.WriteHeader(code)
}
