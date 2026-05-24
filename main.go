package main

import (
	"context"
	"encoding/json"
	"flag"
	"html/template"
	"log"
	"net/http"
	"os"
	"os/signal"
	"runtime/debug"
	"syscall"
	"time"
	"uptimego/db"
	"uptimego/handlers"
	"uptimego/i18n"
	"uptimego/models"
	"uptimego/scheduler"
	"uptimego/web"
)

func main() {
	// Optimize runtime memory foot-print (more aggressive garbage collection)
	debug.SetGCPercent(15)

	// Periodically release unused memory back to the OS
	go func() {
		ticker := time.NewTicker(2 * time.Minute)
		for range ticker.C {
			debug.FreeOSMemory()
		}
	}()

	// CLI Flags
	port := flag.String("port", "8080", "Port to run the application on")
	dbPath := flag.String("db", "data/uptimego.db", "Path to SQLite database file")
	flag.Parse();

	log.Printf("[System] Starting UptimeGo...")

	// 1. Initialize SQLite Database
	if err := db.InitDB(*dbPath); err != nil {
		log.Fatalf("[System] Database initialization failed: %v", err)
	}

	// 2. Initialize embedded template views
	handlers.InitTemplates(nil, func() *template.Template {
		return template.Must(template.New("").Funcs(template.FuncMap{
			"T": func(lang string, key string) string {
				return i18n.T(lang, key)
			},
			"safeHTML": func(s string) template.HTML {
				return template.HTML(s)
			},
			"timeNowStr": func() string {
				return time.Now().Format("2006-01-02 15:04:05")
			},
			"deref": func(pi *int) int {
				if pi == nil {
					return 0
				}
				return *pi
			},
			"json": func(v interface{}) string {
				b, _ := json.Marshal(v)
				return string(b)
			},
		}).ParseFS(web.Assets, "templates/*.html"))
	})

	// 3. Start Uptime checking engine
	scheduler.StartAll()

	// 4. Set up standard routing mux (Using Go 1.22+ enhanced routing)
	mux := http.NewServeMux()

	// Public Pages & Authentication
	mux.HandleFunc("GET /setup", handlers.HandleSetupPage)
	mux.HandleFunc("POST /api/setup", handlers.HandleSetupSubmit)
	mux.HandleFunc("GET /login", handlers.HandleLoginPage)
	mux.HandleFunc("POST /api/login", handlers.HandleLoginSubmit)
	mux.HandleFunc("GET /logout", handlers.HandleLogout)
	mux.HandleFunc("GET /lang", handlers.HandleLanguageSwitch)
	mux.HandleFunc("GET /status", handlers.HandlePublicStatusPage)

	// Admin Panel (Protected by RBAC middlewares)
	// RoleViewer level: can read dashboard and latency history
	viewerChain := handlers.RequireAuth(models.RoleViewer)
	mux.Handle("GET /", viewerChain(http.HandlerFunc(handlers.HandleDashboardPage)))
	mux.Handle("GET /settings", viewerChain(http.HandlerFunc(handlers.HandleSettingsPage)))
	mux.Handle("GET /api/monitors/history", viewerChain(http.HandlerFunc(handlers.HandleAPIMonitorHistory)))

	// RoleAdmin level: can write monitors and settings
	adminChain := handlers.RequireAuth(models.RoleAdmin)
	mux.Handle("POST /api/monitors", adminChain(http.HandlerFunc(handlers.HandleAPIMonitorSave)))
	mux.Handle("DELETE /api/monitors/delete", adminChain(http.HandlerFunc(handlers.HandleAPIMonitorDelete)))
	mux.Handle("POST /api/monitors/toggle", adminChain(http.HandlerFunc(handlers.HandleAPIMonitorToggle)))
	mux.Handle("POST /api/settings", adminChain(http.HandlerFunc(handlers.HandleAPISettingsSave)))
	mux.Handle("POST /api/settings/test", adminChain(http.HandlerFunc(handlers.HandleAPISettingsTestAlert)))

	// RoleSuperAdmin level: can manage users
	superAdminChain := handlers.RequireAuth(models.RoleSuperAdmin)
	mux.Handle("POST /api/users", superAdminChain(http.HandlerFunc(handlers.HandleAPIUserAdd)))
	mux.Handle("DELETE /api/users/delete", superAdminChain(http.HandlerFunc(handlers.HandleAPIUserDelete)))

	// API Key Reset (Accessible by logged in users)
	mux.Handle("POST /api/users/apikey/reset", viewerChain(http.HandlerFunc(handlers.HandleAPIAPIKeyReset)))
	mux.Handle("POST /api/users/profile", viewerChain(http.HandlerFunc(handlers.HandleAPIUserProfileSave)))

	// REST API V1 (Protected by API Key Authentication)
	apiV1Chain := handlers.APIKeyAuthMiddleware
	mux.Handle("GET /api/v1/monitors", apiV1Chain(http.HandlerFunc(handlers.HandleAPIV1MonitorList)))
	mux.Handle("POST /api/v1/monitors", apiV1Chain(http.HandlerFunc(handlers.HandleAPIV1MonitorSave)))
	mux.Handle("DELETE /api/v1/monitors", apiV1Chain(http.HandlerFunc(handlers.HandleAPIV1MonitorDelete)))

	// 5. Apply global middlewares (Setup Checker, Localization Injector, and Request Logging)
	var handler http.Handler = mux
	handler = handlers.SetupCheckMiddleware(handler)
	handler = i18n.Middleware(handler)
	handler = handlers.LoggingMiddleware(handler)

	// 6. Graceful HTTP server setup
	server := &http.Server{
		Addr:         ":" + *port,
		Handler:      handler,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Capture OS signals for graceful shutdown
	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
		<-sigChan

		log.Printf("[System] Shutting down gracefully...")
		scheduler.StopAll()

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := server.Shutdown(ctx); err != nil {
			log.Printf("[System] HTTP server shutdown error: %v", err)
		}
		
		if err := db.DB.Close(); err != nil {
			log.Printf("[System] Database close error: %v", err)
		}
		
		log.Printf("[System] Shutdown complete.")
		os.Exit(0)
	}()

	log.Printf("[System] Web server is listening on port %s", *port)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("[System] Listen and serve failed: %v", err)
	}
}
