package notifier

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/smtp"
	"strings"
	"time"
	"uptimego/db"
	"uptimego/models"
)

type DiscordPayload struct {
	Username  string         `json:"username"`
	AvatarURL string         `json:"avatar_url,omitempty"`
	Embeds    []DiscordEmbed `json:"embeds"`
}

type DiscordEmbed struct {
	Title       string         `json:"title"`
	Description string         `json:"description"`
	Color       int            `json:"color"` // decimal color
	Timestamp   string         `json:"timestamp"`
}

// parseTemplate replaces placeholders with actual check statistics
func parseTemplate(tmpl string, m models.Monitor, status string, timeStr string, msg string, latency int) string {
	r := strings.NewReplacer(
		"{name}", m.Name,
		"{target}", m.Target,
		"{status}", status,
		"{time}", timeStr,
		"{message}", msg,
		"{latency}", fmt.Sprintf("%d", latency),
	)
	return r.Replace(tmpl)
}

// SendAlert dispatches the UP/DOWN notifications asynchronously to enabled channels
func SendAlert(monitor models.Monitor, isUp bool, message string, latency int, isRepeat bool) {
	// Run in background goroutine to not block the scheduling loop
	go func() {
		settings, err := models.LoadSettings(db.DB)
		if err != nil {
			log.Printf("[Notifier] Error loading settings for notifications: %v", err)
			return
		}

		statusStr := "DOWN"
		colorHex := 16711680 // Red (decimal for #FF0000)
		subjectTmpl := settings.AlertSubjectDown
		bodyTmpl := settings.AlertBodyDown
		
		if isUp {
			statusStr = "UP"
			colorHex = 65280 // Green (decimal for #00FF00)
			subjectTmpl = settings.AlertSubjectUp
			bodyTmpl = settings.AlertBodyUp
		}

		timeStr := time.Now().Format("2006-01-02 15:04:05")

		// Parse templates
		parsedSubject := parseTemplate(subjectTmpl, monitor, statusStr, timeStr, message, latency)
		parsedBody := parseTemplate(bodyTmpl, monitor, statusStr, timeStr, message, latency)

		// 1. Discord Webhook Alert
		if settings.DiscordEnabled && settings.DiscordWebhook != "" {
			allowed := false
			if isUp {
				allowed = settings.DiscordAlertOnUp
			} else {
				if isRepeat {
					allowed = settings.DiscordAlertRepeat
				} else {
					allowed = settings.DiscordAlertOnDown
				}
			}
			if allowed {
				if err := sendDiscord(settings.DiscordWebhook, parsedSubject, parsedBody, colorHex); err != nil {
					log.Printf("[Notifier.Discord] Error: %v", err)
				}
			}
		}

		// 2. Telegram Bot Alert
		if settings.TelegramEnabled && settings.TelegramToken != "" && settings.TelegramChatID != "" {
			allowed := false
			if isUp {
				allowed = settings.TelegramAlertOnUp
			} else {
				if isRepeat {
					allowed = settings.TelegramAlertRepeat
				} else {
					allowed = settings.TelegramAlertOnDown
				}
			}
			if allowed {
				if err := sendTelegram(settings.TelegramToken, settings.TelegramChatID, parsedSubject, parsedBody); err != nil {
					log.Printf("[Notifier.Telegram] Error: %v", err)
				}
			}
		}

		// 3. SMTP Email Alert
		if settings.SMTPEnabled && settings.SMTPHost != "" && settings.SMTPTo != "" {
			allowed := false
			if isUp {
				allowed = settings.SMTPAlertOnUp
			} else {
				if isRepeat {
					allowed = settings.SMTPAlertRepeat
				} else {
					allowed = settings.SMTPAlertOnDown
				}
			}
			if allowed {
				if err := sendEmail(settings, parsedSubject, parsedBody, colorHex); err != nil {
					log.Printf("[Notifier.Email] Error: %v", err)
				}
			}
		}
	}()
}

func sendDiscord(url string, title string, body string, color int) error {
	payload := DiscordPayload{
		Username: "UptimeGo",
		Embeds: []DiscordEmbed{
			{
				Title:       title,
				Description: body,
				Color:       color,
				Timestamp:   time.Now().Format(time.RFC3339),
			},
		},
	}

	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("JSON encode error: %w", err)
	}

	resp, err := http.Post(url, "application/json", bytes.NewBuffer(bodyBytes))
	if err != nil {
		return fmt.Errorf("connection error: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("received bad status code: %d", resp.StatusCode)
	}
	return nil
}

func sendTelegram(token string, chatID string, title string, body string) error {
	// Combine title and body for Telegram markdown output
	messageText := fmt.Sprintf("*%s*\n\n%s", title, body)

	apiURL := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", token)
	payload := map[string]string{
		"chat_id":    chatID,
		"text":       messageText,
		"parse_mode": "Markdown",
	}

	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("JSON encode error: %w", err)
	}

	resp, err := http.Post(apiURL, "application/json", bytes.NewBuffer(bodyBytes))
	if err != nil {
		return fmt.Errorf("connection error: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("received bad status code: %d", resp.StatusCode)
	}
	return nil
}

func sendEmail(s *models.SystemSettings, subjectLine string, bodyText string, colorHex int) error {
	subject := "Subject: " + subjectLine + "\n"
	mime := "MIME-version: 1.0;\nContent-Type: text/html; charset=\"UTF-8\";\n\n"

	colorStr := "#EF4444" // Red
	if colorHex == 65280 {
		colorStr = "#10B981" // Green
	}

	htmlBody := fmt.Sprintf(`
		<html>
		<body style="font-family: Arial, sans-serif; background-color: #F3F4F6; padding: 20px; margin: 0;">
			<div style="max-width: 600px; margin: 0 auto; background-color: #FFFFFF; border-radius: 8px; overflow: hidden; box-shadow: 0 4px 6px -1px rgba(0, 0, 0, 0.1);">
				<div style="background-color: %s; padding: 20px; text-align: center; color: white;">
					<h2 style="margin: 0; font-size: 22px;">%s</h2>
				</div>
				<div style="padding: 24px; color: #1F2937; line-height: 1.6; font-size: 15px;">
					%s
				</div>
				<div style="background-color: #F9FAFB; padding: 12px; text-align: center; color: #9CA3AF; font-size: 12px; border-top: 1px solid #E5E7EB;">
					Sent automatically by UptimeGo self-hosted agent.
				</div>
			</div>
		</body>
		</html>
	`, colorStr, subjectLine, strings.ReplaceAll(bodyText, "\n", "<br>"))

	msgBytes := []byte(
		"To: " + s.SMTPTo + "\n" +
			"From: " + s.SMTPFrom + "\n" +
			subject +
			mime +
			htmlBody,
	)

	auth := smtp.PlainAuth("", s.SMTPUser, s.SMTPPass, s.SMTPHost)
	addr := fmt.Sprintf("%s:%d", s.SMTPHost, s.SMTPPort)

	var err error
	if s.SMTPPort == 465 {
		// Implicit TLS for port 465
		tlsConfig := &tls.Config{
			InsecureSkipVerify: true,
			ServerName:         s.SMTPHost,
		}

		conn, dialErr := tls.Dial("tcp", addr, tlsConfig)
		if dialErr != nil {
			return fmt.Errorf("TLS dial error: %w", dialErr)
		}
		defer conn.Close()

		client, clientErr := smtp.NewClient(conn, s.SMTPHost)
		if clientErr != nil {
			return fmt.Errorf("SMTP client initialization error: %w", clientErr)
		}
		defer client.Close()

		if auth != nil {
			if ok, _ := client.Extension("AUTH"); ok {
				if err = client.Auth(auth); err != nil {
					return fmt.Errorf("SMTP Auth error: %w", err)
				}
			}
		}

		if err = client.Mail(s.SMTPFrom); err != nil {
			return fmt.Errorf("MAIL FROM command error: %w", err)
		}
		if err = client.Rcpt(s.SMTPTo); err != nil {
			return fmt.Errorf("RCPT TO command error: %w", err)
		}

		w, writeErr := client.Data()
		if writeErr != nil {
			return fmt.Errorf("DATA command writer error: %w", writeErr)
		}
		_, _ = w.Write(msgBytes)
		w.Close()

	} else {
		// Standard SMTP with STARTTLS (Port 587 / 25)
		c, dialErr := smtp.Dial(addr)
		if dialErr != nil {
			return fmt.Errorf("dial error: %w", dialErr)
		}
		defer c.Close()

		if ok, _ := c.Extension("STARTTLS"); ok {
			config := &tls.Config{ServerName: s.SMTPHost, InsecureSkipVerify: true}
			if err = c.StartTLS(config); err != nil {
				return fmt.Errorf("STARTTLS error: %w", err)
			}
		}

		if auth != nil {
			if ok, _ := c.Extension("AUTH"); ok {
				if err = c.Auth(auth); err != nil {
					return fmt.Errorf("AUTH error: %w", err)
				}
			}
		}

		if err = c.Mail(s.SMTPFrom); err != nil {
			return fmt.Errorf("MAIL FROM error: %w", err)
		}
		if err = c.Rcpt(s.SMTPTo); err != nil {
			return fmt.Errorf("RCPT TO error: %w", err)
		}

		w, writeErr := c.Data()
		if writeErr != nil {
			return fmt.Errorf("DATA error: %w", writeErr)
		}
		defer w.Close()

		_, _ = w.Write(msgBytes)
	}

	log.Printf("[Notifier.Email] Notification sent to %s", s.SMTPTo)
	return nil
}

// TestSMTP runs immediate email validation using temporary ad-hoc parameters
func TestSMTP(host string, port int, user, pass, from, to string) error {
	s := &models.SystemSettings{
		SMTPHost: host,
		SMTPPort: port,
		SMTPUser: user,
		SMTPPass: pass,
		SMTPFrom: from,
		SMTPTo:   to,
	}
	subject := "Test Notification - UptimeGo"
	body := "Your SMTP connection setup is successfully configured and working!"
	return sendEmail(s, subject, body, 65280) // Green color
}

// TestTelegram runs immediate telegram bot message test
func TestTelegram(token string, chatID string) error {
	subject := "Test Notification - UptimeGo"
	body := "Your Telegram channel integration is successfully configured and working!"
	return sendTelegram(token, chatID, subject, body)
}

// TestDiscord runs immediate discord webhook test
func TestDiscord(webhookURL string) error {
	subject := "Test Notification - UptimeGo"
	body := "Your Discord channel integration is successfully configured and working!"
	return sendDiscord(webhookURL, subject, body, 65280) // Green color
}
