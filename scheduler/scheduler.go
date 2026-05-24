package scheduler

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os/exec"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
	"uptimego/db"
	"uptimego/models"
	"uptimego/notifier"
)

var (
	registry = make(map[int64]context.CancelFunc)
	regMu    sync.RWMutex
)

// StartAll retrieves all active monitors and starts their checking loops
func StartAll() {
	rows, err := db.DB.Query("SELECT id FROM monitors WHERE active = 1")
	if err != nil {
		log.Printf("[Scheduler] Error loading monitors on startup: %v", err)
		return
	}
	defer rows.Close()

	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err == nil {
			StartMonitor(id)
		}
	}
	log.Printf("[Scheduler] Started monitoring loops for all active services.")
}

// StopAll stops all active monitoring loops
func StopAll() {
	regMu.Lock()
	defer regMu.Unlock()

	for id, cancel := range registry {
		cancel()
		delete(registry, id)
	}
	log.Printf("[Scheduler] Stopped all monitoring loops.")
}

// StartMonitor spins up a check loop for a specific monitor
func StartMonitor(id int64) {
	StopMonitor(id) // Ensure we cancel any existing loop first

	regMu.Lock()
	defer regMu.Unlock()

	ctx, cancel := context.WithCancel(context.Background())
	registry[id] = cancel

	go monitorLoop(ctx, id)
}

// StopMonitor cancels the check loop for a monitor
func StopMonitor(id int64) {
	regMu.Lock()
	defer regMu.Unlock()

	if cancel, ok := registry[id]; ok {
		cancel()
		delete(registry, id)
	}
}

func monitorLoop(ctx context.Context, monitorID int64) {
	// Fetch monitor config
	var m models.Monitor
	err := db.DB.QueryRow(`
		SELECT id, name, type, target, interval, timeout, retries, alert_interval, active, keyword, ssl_expiry_warning, public, status
		FROM monitors WHERE id = ?`, monitorID).Scan(
		&m.ID, &m.Name, &m.Type, &m.Target, &m.Interval, &m.Timeout, &m.Retries, &m.AlertInterval, &m.Active, &m.Keyword, &m.SslExpiryWarning, &m.Public, &m.Status,
	)
	if err != nil {
		log.Printf("[Scheduler.Loop] Failed to find monitor %d: %v", monitorID, err)
		return
	}

	ticker := time.NewTicker(time.Duration(m.Interval) * time.Second)
	defer ticker.Stop()

	// Track sequential failure retries and alert timers
	consecutiveFailures := 0
	lastAlertedTime := time.Time{}

	// Run initial check immediately
	runCheck(ctx, &m, &consecutiveFailures, &lastAlertedTime)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// Fetch config again in case it was updated on the fly without restarting goroutine
			err = db.DB.QueryRow(`
				SELECT id, name, type, target, interval, timeout, retries, alert_interval, active, keyword, ssl_expiry_warning, public, status
				FROM monitors WHERE id = ?`, monitorID).Scan(
				&m.ID, &m.Name, &m.Type, &m.Target, &m.Interval, &m.Timeout, &m.Retries, &m.AlertInterval, &m.Active, &m.Keyword, &m.SslExpiryWarning, &m.Public, &m.Status,
			)
			if err != nil {
				// Monitor was probably deleted
				return
			}

			if !m.Active {
				return
			}

			runCheck(ctx, &m, &consecutiveFailures, &lastAlertedTime)
		}
	}
}

func runCheck(ctx context.Context, m *models.Monitor, consecutiveFailures *int, lastAlertedTime *time.Time) {
	var isUp bool
	var latency int // ms
	var statusCode int
	var message string
	var sslDays *int

	timeout := time.Duration(m.Timeout) * time.Millisecond

	switch m.Type {
	case "http":
		isUp, latency, statusCode, message, sslDays = performHTTPCheck(ctx, m.Target, timeout, m.Keyword)
	case "ping":
		isUp, latency, message = performPingCheck(m.Target, timeout)
	default:
		isUp = false
		message = "Unknown monitor type"
	}

	// Retry logic:
	// If it fails, only mark it DOWN if failures exceed MaxRetries
	targetStatus := "up"
	if !isUp {
		*consecutiveFailures++
		if *consecutiveFailures >= m.Retries {
			targetStatus = "down"
		} else {
			// Keep previous status while retrying
			targetStatus = m.Status
		}
	} else {
		*consecutiveFailures = 0
		targetStatus = "up"
	}

	// If status changed, update DB and trigger notifications
	statusChanged := m.Status != targetStatus && targetStatus != "unknown"

	shouldRepeatAlert := false
	if targetStatus == "down" && m.AlertInterval > 0 {
		if !lastAlertedTime.IsZero() && time.Since(*lastAlertedTime) >= time.Duration(m.AlertInterval)*time.Second {
			shouldRepeatAlert = true
		}
	}

	// Insert check log to SQLite
	logStatus := "up"
	if !isUp {
		logStatus = "down"
	}

	_, err := db.DB.Exec(`
		INSERT INTO monitor_logs (monitor_id, status, response_time, status_code, message, ssl_days_remaining)
		VALUES (?, ?, ?, ?, ?, ?)`,
		m.ID, logStatus, latency, statusCode, message, sslDays,
	)
	if err != nil {
		log.Printf("[Scheduler] Error saving check log: %v", err)
	}

	if statusChanged || shouldRepeatAlert {
		*lastAlertedTime = time.Now()

		if statusChanged {
			m.Status = targetStatus
			_, err = db.DB.Exec("UPDATE monitors SET status = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?", targetStatus, m.ID)
			if err != nil {
				log.Printf("[Scheduler] Error updating status in DB: %v", err)
			}
		}

		// Dispatch notifications
		alertMessage := message
		if targetStatus == "up" {
			alertMessage = "Service is back online."
		}
		notifier.SendAlert(*m, targetStatus == "up", alertMessage, latency, shouldRepeatAlert)
	}
}

func performHTTPCheck(ctx context.Context, target string, timeout time.Duration, keyword string) (bool, int, int, string, *int) {
	start := time.Now()

	// Prepare request
	req, err := http.NewRequestWithContext(ctx, "GET", target, nil)
	if err != nil {
		return false, 0, 0, fmt.Sprintf("Failed to create request: %v", err), nil
	}
	req.Header.Set("User-Agent", "UptimeGo/1.0 Uptime Monitor")

	// Custom client with redirect prevention and custom TLS skip verification
	// inside transport, but let's check certificates manually or use a standard transport.
	// We want to verify SSL certificates, but if SSL is invalid, we want to capture the error.
	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(address)
			if err != nil {
				host = address
				port = "80"
			}

			if net.ParseIP(host) != nil {
				d := net.Dialer{Timeout: timeout}
				return d.DialContext(ctx, network, address)
			}

			ips, err := resolveHost(ctx, host)
			if err != nil {
				return nil, fmt.Errorf("DNS resolution failed: %w", err)
			}

			var dialErr error
			for _, ip := range ips {
				d := net.Dialer{Timeout: timeout}
				conn, err := d.DialContext(ctx, network, net.JoinHostPort(ip, port))
				if err == nil {
					return conn, nil
				}
				dialErr = err
			}
			if dialErr == nil {
				dialErr = fmt.Errorf("no dialable IP found")
			}
			return nil, dialErr
		},
		TLSHandshakeTimeout: timeout,
		TLSClientConfig: &tls.Config{InsecureSkipVerify: false},
	}

	client := &http.Client{
		Timeout:   timeout,
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			// Don't follow redirects automatically, count 3xx as UP depending on status code
			return http.ErrUseLastResponse
		},
	}

	resp, err := client.Do(req)
	latency := int(time.Since(start).Milliseconds())

	if err != nil {
		// If request failed, check if it's an SSL/TLS error
		errMsg := err.Error()
		var sslDaysRemaining *int
		
		if strings.Contains(errMsg, "x509") || strings.Contains(errMsg, "certificate") {
			// Try to retrieve certificate details by bypassing verification
			sslDaysRemaining = checkSSLDaysDirect(target)
			return false, latency, 0, fmt.Sprintf("SSL/TLS Certificate Error: %v", err), sslDaysRemaining
		}
		return false, latency, 0, errMsg, nil
	}
	defer resp.Body.Close()

	// Analyze TLS certificate if HTTPS
	var sslDaysRemaining *int
	if resp.TLS != nil && len(resp.TLS.PeerCertificates) > 0 {
		cert := resp.TLS.PeerCertificates[0]
		days := int(time.Until(cert.NotAfter).Hours() / 24)
		sslDaysRemaining = &days
	}

	// Read body if keyword check is required
	if keyword != "" {
		bodyBytes, err := io.ReadAll(io.LimitReader(resp.Body, 1024*1024)) // limit to 1MB
		if err != nil {
			return false, latency, resp.StatusCode, "Failed to read body for keyword check", sslDaysRemaining
		}
		if !strings.Contains(string(bodyBytes), keyword) {
			return false, latency, resp.StatusCode, fmt.Sprintf("Keyword '%s' not found in response", keyword), sslDaysRemaining
		}
	}

	// Status code check: 2xx or 3xx are considered success
	if resp.StatusCode >= 200 && resp.StatusCode < 400 {
		return true, latency, resp.StatusCode, "OK", sslDaysRemaining
	}

	return false, latency, resp.StatusCode, fmt.Sprintf("HTTP Status Code: %d", resp.StatusCode), sslDaysRemaining
}

// checkSSLDaysDirect establishes a raw TLS connection to extract certificate expiry days even if cert is invalid
func checkSSLDaysDirect(targetURL string) *int {
	host := targetURL
	if strings.Contains(host, "://") {
		parts := strings.Split(host, "://")
		if len(parts) > 1 {
			host = parts[1]
		}
	}
	if idx := strings.Index(host, "/"); idx != -1 {
		host = host[:idx]
	}
	
	port := "443"
	if strings.Contains(host, ":") {
		h, p, err := net.SplitHostPort(host)
		if err == nil {
			host = h
			port = p
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ips, err := resolveHost(ctx, host)
	if err != nil {
		return nil
	}

	var conn *tls.Conn
	var dialErr error
	for _, ip := range ips {
		dialer := &net.Dialer{Timeout: 5 * time.Second}
		conn, dialErr = tls.DialWithDialer(dialer, "tcp", net.JoinHostPort(ip, port), &tls.Config{
			InsecureSkipVerify: true,
			ServerName:         host,
		})
		if dialErr == nil {
			break
		}
	}
	if dialErr != nil {
		return nil
	}
	defer conn.Close()

	state := conn.ConnectionState()
	if len(state.PeerCertificates) > 0 {
		cert := state.PeerCertificates[0]
		days := int(time.Until(cert.NotAfter).Hours() / 24)
		return &days
	}
	return nil
}

// performPingCheck performs a ping by either:
// 1. TCP dial (if port is specified, e.g. 192.168.1.1:80)
// 2. Running system's ping command (ICMP)
func performPingCheck(target string, timeout time.Duration) (bool, int, string) {
	// If target has a port, perform TCP Ping instead
	if strings.Contains(target, ":") {
		start := time.Now()
		host, port, err := net.SplitHostPort(target)
		if err != nil {
			return false, 0, fmt.Sprintf("Invalid target format: %v", err)
		}

		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()

		ips, err := resolveHost(ctx, host)
		if err != nil {
			return false, int(time.Since(start).Milliseconds()), fmt.Sprintf("DNS resolution failed: %v", err)
		}

		var conn net.Conn
		var dialErr error
		for _, ip := range ips {
			dialer := &net.Dialer{Timeout: timeout}
			conn, dialErr = dialer.DialContext(ctx, "tcp", net.JoinHostPort(ip, port))
			if dialErr == nil {
				break
			}
		}
		latency := int(time.Since(start).Milliseconds())
		if dialErr != nil {
			return false, latency, fmt.Sprintf("TCP Connection Failed: %v", dialErr)
		}
		conn.Close()
		return true, latency, "TCP Ping Success"
	}

	// ICMP Ping via host ping execution (prevents root permission requirements)
	start := time.Now()
	var cmd *exec.Cmd

	if runtime.GOOS == "windows" {
		// -n 1: 1 packet, -w timeout_ms
		timeoutMs := strconv.FormatInt(timeout.Milliseconds(), 10)
		cmd = exec.Command("ping", "-n", "1", "-w", timeoutMs, target)
	} else {
		// Linux / Alpine
		// -c 1: 1 packet, -W timeout_sec
		timeoutSec := strconv.FormatInt(int64(timeout.Seconds()), 10)
		if timeoutSec == "0" {
			timeoutSec = "1"
		}
		cmd = exec.Command("ping", "-c", "1", "-W", timeoutSec, target)
	}

	output, err := cmd.CombinedOutput()
	latency := int(time.Since(start).Milliseconds())

	if err != nil {
		return false, latency, fmt.Sprintf("Ping failed: target unreachable or timeout. Output: %s", strings.TrimSpace(string(output)))
	}

	// Double check output to confirm success
	outStr := string(output)
	if strings.Contains(outStr, "100% packet loss") || strings.Contains(outStr, "Unreachable") || strings.Contains(outStr, "timed out") {
		return false, latency, "Ping packet lost (Timeout/Unreachable)"
	}

	// Try parsing actual ping latency from stdout if possible
	parsedLatency := parsePingLatency(outStr)
	if parsedLatency > 0 {
		latency = parsedLatency
	}

	return true, latency, "Ping Success"
}

// parsePingLatency attempts to extract the round-trip latency from standard ping outputs
func parsePingLatency(out string) int {
	// Match expressions like "time=12.3 ms" or "time=12ms" or "time<1ms"
	re := regexp.MustCompile(`time[=<]([0-9.]+)\s*ms`)
	matches := re.FindStringSubmatch(out)
	if len(matches) > 1 {
		val, err := strconv.ParseFloat(matches[1], 64)
		if err == nil {
			return int(val)
		}
	}
	return 0
}

func resolveHostDoH(ctx context.Context, host string) ([]string, error) {
	client := &http.Client{
		Timeout: 3 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}

	url := fmt.Sprintf("https://1.1.1.1/dns-query?name=%s&type=A", host)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/dns-json")

	resp, err := client.Do(req)
	if err != nil {
		url = fmt.Sprintf("https://8.8.8.8/resolve?name=%s&type=A", host)
		req, err = http.NewRequestWithContext(ctx, "GET", url, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Accept", "application/dns-json")
		resp, err = client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("DoH resolution failed: %w", err)
		}
	}
	defer resp.Body.Close()

	var result struct {
		Status int `json:"Status"`
		Answer []struct {
			Type int    `json:"type"`
			Data string `json:"data"`
		} `json:"Answer"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	if result.Status == 3 {
		return nil, fmt.Errorf("DNS name not found (NXDOMAIN)")
	}

	var ips []string
	for _, ans := range result.Answer {
		if ans.Type == 1 {
			ips = append(ips, ans.Data)
		}
	}

	if len(ips) == 0 {
		return nil, fmt.Errorf("no A records resolved")
	}

	return ips, nil
}

func resolveHost(ctx context.Context, host string) ([]string, error) {
	ips, err := resolveHostDoH(ctx, host)
	if err == nil {
		return ips, nil
	}

	if strings.Contains(err.Error(), "NXDOMAIN") {
		return nil, err
	}

	log.Printf("[Scheduler.Resolver] DoH failed for %s, falling back to system resolver: %v", host, err)
	return net.LookupHost(host)
}
