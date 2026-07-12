package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
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
	"github.com/totvs/tcloud-planner/backend/internal/middleware"
	"github.com/totvs/tcloud-planner/backend/internal/repository"
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
		})
	})

	addr := fmt.Sprintf("%s:%s", cfg.Server.Host, cfg.Server.Port)
	srv := &http.Server{
		Addr:         addr,
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
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

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Fatal("server forced to shutdown", zap.Error(err))
	}

	logger.Info("server stopped")
}
