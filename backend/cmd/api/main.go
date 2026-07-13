package main

import (
	"context"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/totvs/tcloud-planner/backend/internal/auth"
	"github.com/totvs/tcloud-planner/backend/internal/config"
	"github.com/totvs/tcloud-planner/backend/internal/handler"
	"github.com/totvs/tcloud-planner/backend/internal/jira"
	"github.com/totvs/tcloud-planner/backend/internal/middleware"
	"github.com/totvs/tcloud-planner/backend/internal/repository"
	"github.com/totvs/tcloud-planner/backend/internal/service"
	"github.com/totvs/tcloud-planner/backend/internal/worker"
	"go.uber.org/zap"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to load config: %v\n", err)
		os.Exit(1)
	}

	if cfg.Auth.JWTSecret == "" {
		fmt.Fprintf(os.Stderr, "JWT_SECRET is required\n")
		os.Exit(1)
	}

	var logger *zap.Logger
	if cfg.Log.Level == "debug" {
		logger, err = zap.NewDevelopment()
	} else {
		logger, err = zap.NewProduction()
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to init logger: %v\n", err)
		os.Exit(1)
	}
	defer logger.Sync()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pool, err := pgxpool.New(ctx, cfg.DB.DSN())
	if err != nil {
		logger.Fatal("failed to connect to database", zap.Error(err))
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		logger.Fatal("failed to ping database", zap.Error(err))
	}
	logger.Info("connected to database")

	tokenService := auth.NewTokenService(cfg.Auth.JWTSecret, cfg.Auth.JWTExpirationHours)

	fonteDadosRepo := repository.NewFonteDadosRepository(pool)
	usuarioRepo := repository.NewUsuarioRepository(pool)
	equipeRepo := repository.NewEquipeRepository(pool)

	fonteDadosHandler := handler.NewFonteDadosHandler(fonteDadosRepo, logger)
	authHandler := handler.NewAuthHandler(usuarioRepo, tokenService, logger)
	usuarioHandler := handler.NewUsuarioHandler(usuarioRepo, logger)
	equipeHandler := handler.NewEquipeHandler(equipeRepo, logger)

	timelineRepo := repository.NewTimelineRepository(pool)

	var analyzer service.AnalisadorCapacidade
	if cfg.Gemini.APIKey != "" {
		analyzer = service.NewGeminiAnalyzer(cfg.Gemini.APIKey, cfg.Gemini.Model)
		logger.Info("gemini analyzer configured", zap.String("model", cfg.Gemini.Model))
	} else {
		logger.Warn("GEMINI_API_KEY not set, AI analysis disabled")
	}

	timelineHandler := handler.NewTimelineHandler(timelineRepo, analyzer, logger)

	membroRepo := repository.NewMembroRepository(pool)
	membroHandler := handler.NewMembroHandler(membroRepo, logger)

	syncRepo := repository.NewSyncRepository(pool)
	clientFactory := func(baseURL, email, apiToken string, rateLimit int, logger *zap.Logger) jira.Client {
		return jira.NewHTTPClient(baseURL, email, apiToken, rateLimit, logger)
	}
	syncService := service.NewSyncService(syncRepo, fonteDadosRepo, clientFactory, cfg.Sync.RateLimitPerSec, logger)
	syncHandler := handler.NewSyncHandler(syncService, logger)

	syncWorker := worker.NewSyncWorker(func(ctx context.Context) error {
		_, err := syncService.SyncAll(ctx)
		return err
	}, cfg.Sync.IntervalMinutes, logger)
	go syncWorker.Start(ctx)

	r := chi.NewRouter()

	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type"},
		ExposedHeaders:   []string{"Link"},
		AllowCredentials: false,
		MaxAge:           300,
	}))
	r.Use(chimw.Logger)
	r.Use(chimw.Recoverer)
	r.Use(chimw.RequestID)

	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	})

	r.Route("/api/v1", func(r chi.Router) {
		r.Post("/auth/login", authHandler.Login)

		r.Group(func(r chi.Router) {
			r.Use(middleware.AuthJWT(tokenService))
			r.Use(middleware.ProjetoFilter(usuarioRepo))

			r.Get("/fontes", fonteDadosHandler.List)
			r.Post("/fontes", fonteDadosHandler.Create)
			r.Get("/fontes/{id}", fonteDadosHandler.GetByID)
			r.Put("/fontes/{id}", fonteDadosHandler.Update)
			r.Delete("/fontes/{id}", fonteDadosHandler.Delete)

			r.Get("/usuarios", usuarioHandler.List)
			r.Post("/usuarios", usuarioHandler.Create)
			r.Get("/usuarios/{id}", usuarioHandler.GetByID)
			r.Put("/usuarios/{id}", usuarioHandler.Update)
			r.Put("/usuarios/{id}/senha", usuarioHandler.AlterarSenha)
			r.Get("/usuarios/{id}/projetos", usuarioHandler.ListProjetos)
			r.Put("/usuarios/{id}/projetos", usuarioHandler.UpdateProjetos)

			r.Get("/equipes", equipeHandler.List)
			r.Get("/equipes/{team}/resumo", equipeHandler.GetResumo)
			r.Get("/equipes/{team}/membros", equipeHandler.GetMembros)

			r.Get("/timeline-capacidade", timelineHandler.ListTimeline)
			r.Post("/timeline-capacidade/analisar", timelineHandler.AnalisarCapacidade)
			r.Get("/projetos", timelineHandler.ListProjetos)
			r.Put("/projetos/{id}/metadata", timelineHandler.UpdateProjetoMetadata)

			r.Get("/membros", membroHandler.List)
			r.Put("/membros/{id}/team", membroHandler.UpdateTeam)
			r.Get("/membros/teams", membroHandler.ListTeams)

			r.Post("/sync/trigger", syncHandler.TriggerSync)
			r.Get("/sync/status", syncHandler.GetSyncStatus)
			r.Get("/sync/logs", syncHandler.ListSyncLogs)
		})
	})

	frontendDir := filepath.Join("..", "frontend")
	if _, err := os.Stat(frontendDir); err == nil {
		indexPath := filepath.Join(frontendDir, "index.html")
		serveIndex := func(w http.ResponseWriter, req *http.Request) {
			http.ServeFile(w, req, indexPath)
		}
		r.Get("/", serveIndex)
		r.Get("/static/*", http.StripPrefix("/static/", http.FileServer(http.Dir(filepath.Join(frontendDir, "static")))).ServeHTTP)
		r.NotFound(func(w http.ResponseWriter, req *http.Request) {
			if len(req.URL.Path) > 1 {
				filePath := req.URL.Path[1:]
				if _, err := fs.Stat(os.DirFS(frontendDir), filePath); err == nil {
					http.ServeFile(w, req, filepath.Join(frontendDir, filePath))
					return
				}
			}
			serveIndex(w, req)
		})
		logger.Info("serving frontend", zap.String("dir", frontendDir))
	}

	addr := fmt.Sprintf("%s:%s", cfg.Server.Host, cfg.Server.Port)
	srv := &http.Server{
		Addr:         addr,
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		logger.Info("starting server", zap.String("addr", addr))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal("server failed", zap.Error(err))
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	sig := <-quit
	logger.Info("shutting down server", zap.String("signal", sig.String()))

	syncWorker.Stop()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Fatal("server forced to shutdown", zap.Error(err))
	}

	logger.Info("server stopped")
}
