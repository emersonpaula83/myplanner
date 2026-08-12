package main

import (
	"context"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/emersonpaula83/myplanner/backend/internal/admin"
	"github.com/emersonpaula83/myplanner/backend/internal/auth"
	"github.com/emersonpaula83/myplanner/backend/internal/config"
	"github.com/emersonpaula83/myplanner/backend/internal/handler"
	"github.com/emersonpaula83/myplanner/backend/internal/jira"
	"github.com/emersonpaula83/myplanner/backend/internal/middleware"
	"github.com/emersonpaula83/myplanner/backend/internal/repository"
	samlpkg "github.com/emersonpaula83/myplanner/backend/internal/saml"
	"github.com/emersonpaula83/myplanner/backend/internal/service"
	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/jackc/pgx/v5/pgxpool"
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

	var samlAuthHandler *handler.SAMLAuthHandler
	if cfg.SAML.IDPMetadataURL != "" {
		samlProvider, err := samlpkg.NewSAMLProvider(cfg.SAML)
		if err != nil {
			logger.Fatal("failed to init SAML provider", zap.Error(err))
		}
		samlAuthHandler = handler.NewSAMLAuthHandler(samlProvider, usuarioRepo, tokenService, cfg.SAML.FrontendURL, logger)
		logger.Info("SAML SSO configured", zap.String("entity_id", cfg.SAML.EntityID))
	} else {
		logger.Warn("SAML_IDP_METADATA_URL not set, SAML SSO disabled")
	}

	fonteDadosHandler := handler.NewFonteDadosHandler(fonteDadosRepo, logger)
	authHandler := handler.NewAuthHandler(usuarioRepo, tokenService, logger)
	usuarioHandler := handler.NewUsuarioHandler(usuarioRepo, logger, cfg.Auth.AdminEmail)
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

	investRepo := repository.NewInvestimentoRepository(pool)
	investService := service.NewInvestimentoService(equipeRepo, membroRepo, investRepo, logger)
	investHandler := handler.NewInvestimentoHandler(investService, membroRepo, logger)

	sprintRepo := repository.NewSprintRepository(pool)
	sprintService := service.NewSprintService(sprintRepo, logger)
	sprintHandler := handler.NewSprintHandler(sprintService, logger)

	feriadoRepo := repository.NewFeriadoRepository(pool)
	feriadoHandler := handler.NewFeriadoHandler(feriadoRepo, logger)

	checkpointRepo := repository.NewCheckpointRepository(pool)
	checkpointHandler := handler.NewCheckpointHandler(checkpointRepo, logger)

	syncRepo := repository.NewSyncRepository(pool)
	clientFactory := func(baseURL, email, apiToken string, rateLimit int, logger *zap.Logger) jira.Client {
		return jira.NewHTTPClient(baseURL, email, apiToken, rateLimit, logger)
	}
	oauthClientFactory := func(baseURL, accessToken string, rateLimit int, logger *zap.Logger) jira.Client {
		return jira.NewOAuthClient(baseURL, accessToken, rateLimit, logger)
	}

	var oauthSvc *jira.OAuthService
	var oauthHandler *handler.OAuthHandler
	if cfg.AtlassianOAuth.ClientID != "" && cfg.AtlassianOAuth.ClientSecret != "" {
		oauthCfg := jira.OAuthConfig{
			ClientID:     cfg.AtlassianOAuth.ClientID,
			ClientSecret: cfg.AtlassianOAuth.ClientSecret,
			CallbackURL:  cfg.AtlassianOAuth.AppBaseURL + "/auth/atlassian/callback",
		}
		oauthSvc = jira.NewOAuthService(oauthCfg)
		oauthHandler = handler.NewOAuthHandler(oauthSvc, fonteDadosRepo, logger)
		logger.Info("atlassian oauth configured", zap.String("callback", oauthCfg.CallbackURL))
	} else {
		logger.Warn("ATLASSIAN_CLIENT_ID/SECRET not set, OAuth disabled")
	}

	syncService := service.NewSyncService(syncRepo, fonteDadosRepo, clientFactory, oauthClientFactory, oauthSvc, cfg.Sync.RateLimitPerSec, logger)
	syncHandler := handler.NewSyncHandler(syncService, logger)

	scheduleRepo := repository.NewSyncScheduleRepository(pool)
	scheduleHandler := handler.NewSyncScheduleHandler(scheduleRepo, logger)
	schedulerSvc := service.NewSchedulerService(syncService, scheduleRepo, logger)
	go schedulerSvc.Start(ctx)

	if os.Getenv("KUBERNETES_SERVICE_HOST") != "" {
		secretWriter := admin.NewSecretWriter(cfg.AdminSecret, logger)
		adminStore := admin.NewRepoAdapter(usuarioRepo)
		adminRotator := admin.NewAdminRotator(adminStore, secretWriter, cfg.Auth.AdminEmail, logger)
		go adminRotator.Start(ctx)
	} else {
		logger.Info("dev mode: admin password rotation disabled, use PASS_APP from .env")
	}

	sprintGenService := service.NewSprintGenerationService(fonteDadosRepo, equipeRepo, syncRepo, sprintRepo, clientFactory, oauthClientFactory, oauthSvc, cfg.Sync.RateLimitPerSec, logger)
	sprintGenHandler := handler.NewSprintGenerationHandler(sprintGenService, logger)

	configRepo := repository.NewConfigRepository(pool)

	equalizerSvc := service.NewEqualizerService(sprintService, sprintRepo, fonteDadosRepo, configRepo, clientFactory, oauthClientFactory, oauthSvc, cfg.Sync.RateLimitPerSec, logger)
	equalizerHandler := handler.NewEqualizerHandler(equalizerSvc, logger)

	skillRepo := repository.NewSkillRepository(pool)
	skillHandler := handler.NewSkillHandler(skillRepo, logger)

	reviewRepo := repository.NewReviewRepository(pool)
	reviewService := service.NewReviewService(reviewRepo, configRepo, logger)
	reviewHandler := handler.NewReviewHandler(reviewService, configRepo, logger)

	destRepo := repository.NewDestinatarioRepository(pool)
	emailProv := service.NewEmailProvider(configRepo, logger)
	whatsappProv := service.NewWhatsAppProvider(configRepo, logger)
	notifSvc := service.NewNotificationService(reviewService, destRepo, sprintRepo, emailProv, whatsappProv, logger)
	notifHandler := handler.NewNotificationHandler(destRepo, notifSvc, logger)

	allocRepo := repository.NewAllocationRepository(pool)
	allocSvc := service.NewAllocationService(allocRepo, sprintService, sprintRepo, fonteDadosRepo, syncService, clientFactory, oauthClientFactory, oauthSvc, cfg.Sync.RateLimitPerSec, logger)
	allocHandler := handler.NewAllocationHandler(allocSvc, logger)

	tarefaRepo := repository.NewTarefaRepository(pool)
	tarefaHandler := handler.NewTarefaHandler(tarefaRepo, logger)

	r := chi.NewRouter()

	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "PATCH", "OPTIONS"},
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

	if oauthHandler != nil {
		r.Get("/auth/atlassian/authorize", oauthHandler.Authorize)
		r.Get("/auth/atlassian/callback", oauthHandler.Callback)
	}

	if samlAuthHandler != nil {
		r.Get("/api/v1/auth/saml/login", samlAuthHandler.Login)
		r.Post("/api/v1/auth/saml/acs", samlAuthHandler.ACS)
		r.Get("/api/v1/auth/saml/metadata", samlAuthHandler.Metadata)
	}

	r.Route("/api/v1", func(r chi.Router) {
		r.Post("/auth/login", authHandler.Login)

		r.Group(func(r chi.Router) {
			r.Use(middleware.AuthJWT(tokenService))
			r.Use(middleware.EquipeFilter(usuarioRepo, equipeRepo, cfg.Auth.AdminEmail))

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
			r.Get("/usuarios/{id}/equipes", usuarioHandler.ListEquipes)
			r.Put("/usuarios/{id}/equipes", usuarioHandler.UpdateEquipes)

			r.Get("/equipes", equipeHandler.List)
			r.Post("/equipes", equipeHandler.Create)
			r.Put("/equipes/{id}", equipeHandler.Update)
			r.Delete("/equipes/{id}", equipeHandler.Delete)
			r.Get("/equipes/{id}/resumo", equipeHandler.GetResumo)
			r.Get("/equipes/{id}/membros", equipeHandler.GetMembros)
			r.Post("/equipes/{id}/membros", equipeHandler.AddMembro)
			r.Delete("/equipes/{id}/membros/{membroId}", equipeHandler.RemoveMembro)

			// Investimentos
			r.Get("/equipes/{id}/investimentos", investHandler.GetDashboard)
			r.Get("/equipes/{id}/investimentos/gastos-mensais", investHandler.GetGastosMensais)

			// Membro financial
			r.Put("/membros/{id}/salario", investHandler.UpdateSalario)
			r.Put("/membros/{id}/banco-horas", investHandler.UpdateBancoHoras)
			r.Put("/membros/{id}/data-admissao", investHandler.UpdateDataAdmissao)
			r.Get("/membros/{id}/salario/historico", investHandler.GetHistoricoSalario)
			r.Get("/membros/{id}/banco-horas/historico", investHandler.GetHistoricoBancoHoras)
			r.Get("/membros/{id}/alocacoes-projetos", investHandler.GetAlocacoesProjetos)

			r.Get("/timeline-capacidade", timelineHandler.ListTimeline)
			r.Post("/timeline-capacidade/analisar", timelineHandler.AnalisarCapacidade)
			r.Get("/projetos", timelineHandler.ListProjetos)
			r.Put("/projetos/{id}/metadata", timelineHandler.UpdateProjetoMetadata)
			r.Get("/projetos/{id}/equipes", timelineHandler.GetProjetoEquipes)

			r.Get("/membros", membroHandler.List)
			r.Get("/membros/search", membroHandler.Search)
			r.Get("/membros/{id}", membroHandler.GetByID)
			r.Post("/membros/{id}/disponibilidade", membroHandler.CreateDisponibilidade)
			r.Put("/membros/{id}/disponibilidade/{dispId}", membroHandler.UpdateDisponibilidade)
			r.Delete("/membros/{id}/disponibilidade/{dispId}", membroHandler.DeleteDisponibilidade)
			r.Put("/membros/{id}/desligamento", membroHandler.UpdateDataDesligamento)

			r.Get("/skills", skillHandler.List)
			r.Post("/skills", skillHandler.Create)
			r.Delete("/skills/{id}", skillHandler.Delete)

			r.Get("/membros/{id}/skills", skillHandler.GetMembroSkills)
			r.Post("/membros/{id}/skills", skillHandler.AddMembroSkill)
			r.Delete("/membros/{id}/skills/{skillId}", skillHandler.RemoveMembroSkill)

			r.Put("/membros/{id}/cargo", equipeHandler.UpdateCargo)
			r.Get("/membros/{id}/produtos", equipeHandler.GetMembroProdutos)
			r.Put("/membros/{id}/produtos", equipeHandler.SetMembroProdutos)

			r.Get("/produtos", equipeHandler.ListProdutos)
			r.Get("/cargos", equipeHandler.ListCargos)

			r.Get("/feriados", feriadoHandler.List)
			r.Post("/feriados", feriadoHandler.Create)
			r.Delete("/feriados/{id}", feriadoHandler.Delete)

			r.Get("/checkpoints", checkpointHandler.List)
			r.Post("/checkpoints", checkpointHandler.Create)
			r.Delete("/checkpoints/{id}", checkpointHandler.Delete)

			r.Get("/sprints", sprintHandler.ListSprints)
			r.Get("/sprints/projetos", sprintHandler.ListProjetos)
			r.Get("/sprints/timeline", sprintHandler.GetSprintsTimeline)
			r.Get("/sprints/boards", sprintGenHandler.GetBoards)
			r.Post("/sprints/generate/preview", sprintGenHandler.Preview)
			r.Post("/sprints/generate", sprintGenHandler.Generate)
			r.Get("/projetos/{id}/sprints", sprintHandler.ListByProjeto)
			r.Get("/sprints/{id}/capacity", sprintHandler.GetCapacity)
			r.Get("/sprints/{id}/unplanned", sprintHandler.GetUnplanned)
			r.Get("/sprints/{id}/burndown", sprintHandler.GetBurndown)
			r.Get("/sprints/{id}/disclaimer-tasks", sprintHandler.GetDisclaimerTasks)
			r.Get("/sprints/{id}/timeline-detail", sprintHandler.GetTimelineDetail)
			r.Get("/sprints/{id}/equalizer", equalizerHandler.GetSuggestions)
			r.Post("/sprints/{id}/equalizer/apply", equalizerHandler.ApplyTransfers)

			r.Get("/sprints/{id}/review", reviewHandler.GetReviewData)
			r.Get("/sprints/{sprintId}/review/destaques", reviewHandler.ListDestaques)
			r.Post("/sprints/{sprintId}/review/destaques", reviewHandler.CreateDestaque)
			r.Put("/destaques/{id}", reviewHandler.UpdateDestaque)
			r.Delete("/destaques/{id}", reviewHandler.DeleteDestaque)

			r.Get("/sprints/{id}/review/analise", reviewHandler.GetReviewAnalise)
			r.Post("/sprints/{id}/review/analise", reviewHandler.PostReviewAnalise)

			r.Get("/equipes/{id}/destinatarios", notifHandler.ListDestinatarios)
			r.Post("/equipes/{id}/destinatarios", notifHandler.CreateDestinatario)
			r.Delete("/equipes/{id}/destinatarios/{destId}", notifHandler.DeleteDestinatario)

			r.Post("/sprints/{id}/review/enviar", notifHandler.EnviarReview)

			r.Get("/config/{chave}", reviewHandler.GetConfig)
			r.Post("/config", reviewHandler.SetConfig)

			r.Post("/sync/trigger", syncHandler.TriggerSync)
			r.Get("/sync/status", syncHandler.GetSyncStatus)
			r.Get("/sync/logs", syncHandler.ListSyncLogs)
			r.Get("/sync/projects", syncHandler.ListJiraProjects)

			r.Get("/sync/schedules", scheduleHandler.Get)
			r.Put("/sync/schedules", scheduleHandler.Upsert)
			r.Delete("/sync/schedules", scheduleHandler.Delete)
			r.Patch("/sync/schedules/{id}/toggle", scheduleHandler.Toggle)

			r.Get("/allocation/projects", allocHandler.ListProjects)
			r.Get("/allocation/projects/{epicId}", allocHandler.GetProjectDetail)
			r.Post("/allocation/tasks/{taskId}/allocate", allocHandler.AllocateTask)
			r.Post("/allocation/projects/{epicId}/sync", allocHandler.SyncProject)
			r.Get("/allocation/sprints", allocHandler.ListSprints)
			r.Post("/allocation/projects/{epicId}/close", allocHandler.CloseProject)
			r.Delete("/allocation/projects/{epicId}/close", allocHandler.ReopenProject)
			r.Get("/allocation/products", allocHandler.ListFilteredProducts)

			r.Get("/tarefas", tarefaHandler.ListTarefas)
			r.Delete("/tarefas/{id}", tarefaHandler.DeleteTarefa)
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
		WriteTimeout: 300 * time.Second,
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
