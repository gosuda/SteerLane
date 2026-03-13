// Package main is the entry point for the SteerLane server.
package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	goredis "github.com/redis/go-redis/v9"

	steerlane "github.com/gosuda/steerlane"
	"github.com/gosuda/steerlane/internal/adrengine"
	agentpkg "github.com/gosuda/steerlane/internal/agent"
	"github.com/gosuda/steerlane/internal/agent/claude"
	"github.com/gosuda/steerlane/internal/agent/codex"
	"github.com/gosuda/steerlane/internal/agent/gemini"
	"github.com/gosuda/steerlane/internal/agent/opencode"
	v1 "github.com/gosuda/steerlane/internal/api/v1"
	"github.com/gosuda/steerlane/internal/api/ws"
	"github.com/gosuda/steerlane/internal/audit"
	"github.com/gosuda/steerlane/internal/auth"
	"github.com/gosuda/steerlane/internal/bootstrap"
	"github.com/gosuda/steerlane/internal/config"
	"github.com/gosuda/steerlane/internal/docker"
	"github.com/gosuda/steerlane/internal/domain"
	"github.com/gosuda/steerlane/internal/domain/hitl"
	"github.com/gosuda/steerlane/internal/gitops"
	"github.com/gosuda/steerlane/internal/hitlrouter"
	"github.com/gosuda/steerlane/internal/messenger"
	messengerdiscord "github.com/gosuda/steerlane/internal/messenger/discord"
	messengerslack "github.com/gosuda/steerlane/internal/messenger/slack"
	messengertelegram "github.com/gosuda/steerlane/internal/messenger/telegram"
	dbmigrate "github.com/gosuda/steerlane/internal/migrate"
	"github.com/gosuda/steerlane/internal/notify"
	"github.com/gosuda/steerlane/internal/orchestrator"
	"github.com/gosuda/steerlane/internal/server"
	"github.com/gosuda/steerlane/internal/store/postgres"
	redisPkg "github.com/gosuda/steerlane/internal/store/redis"
	"github.com/gosuda/steerlane/internal/volume"
	"github.com/gosuda/steerlane/web"
)

func main() {
	if err := run(context.Background()); err != nil {
		fmt.Fprintf(os.Stderr, "steerlane: %v\n", err)
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	logger := newLogger(cfg.LogLevel)
	slog.SetDefault(logger)

	logger.Info("starting steerlane",
		"mode", cfg.Mode,
		"addr", cfg.HTTP.Addr,
	)

	// Migrations
	if migrateErr := dbmigrate.Run(steerlane.Migrations, cfg.Postgres.DSN, logger); migrateErr != nil {
		return fmt.Errorf("run migrations: %w", migrateErr)
	}

	// Database
	store, err := postgres.New(ctx, cfg.Postgres)
	if err != nil {
		return fmt.Errorf("connect postgres: %w", err)
	}
	defer store.Close()

	// Redis
	redisClient := goredis.NewClient(&goredis.Options{
		Addr:     cfg.Redis.Addr,
		Password: cfg.Redis.Password,
		DB:       cfg.Redis.DB,
	})
	if err := redisClient.Ping(ctx).Err(); err != nil { //nolint:govet // short-lived err shadow is idiomatic Go
		return fmt.Errorf("connect redis: %w", err)
	}
	defer redisClient.Close()

	// Services
	jwtSvc := auth.NewJWTService(cfg.Auth.JWTSecret, cfg.Auth.JWTIssuer, cfg.Auth.JWTExpiry, cfg.Auth.JWTRefreshExpiry)
	authSvc := auth.NewService(store.Users(), store.APIKeys(), jwtSvc, logger)
	authAdapter := auth.NewAuthAdapter(authSvc)
	linkingSvc := auth.NewLinkingService(cfg.Auth.JWTSecret, cfg.HTTP.PublicBaseURL, time.Hour)
	auditSvc := audit.NewService(store.Audit())

	// PubSub and WS
	redisPubSub := redisPkg.NewPubSub(redisClient)
	hub := ws.NewHub(logger, redisPubSub)

	registry := agentpkg.NewRegistry()
	claude.RegisterDefault(registry)
	codex.RegisterDefault(registry)
	gemini.RegisterDefault(registry)
	opencode.RegisterDefault(registry)

	runtimeManager, err := docker.NewCLIRuntime(logger)
	if err != nil {
		return fmt.Errorf("create docker runtime: %w", err)
	}
	volumeManager, err := volume.NewManager(logger)
	if err != nil {
		return fmt.Errorf("create volume manager: %w", err)
	}
	gitOperator, err := gitops.NewOps(logger)
	if err != nil {
		return fmt.Errorf("create git operator: %w", err)
	}

	sessionThreads := messenger.NewSessionContextRegistry()
	hitlRouter := hitlrouter.NewRouter(logger, store.HITL(), store.Agents(), redisPubSub)
	adrEngine := adrengine.NewEngine(logger, store.ADRs(), redisPubSub)
	timeoutDispatcher := notify.NewDispatcher(
		logger,
		messenger.PlatformSlack,
		nil,
		nil,
		sessionThreads,
		store.Queries(),
		store.Tasks(),
		store.Agents(),
		store.HITL(),
		store.Users(),
	)
	orchestratorSvc := orchestrator.New(orchestrator.Deps{
		Logger:    logger,
		Registry:  registry,
		Runtime:   runtimeManager,
		Volumes:   volumeManager,
		GitOps:    gitOperator,
		Projects:  store.Projects(),
		Tasks:     store.Tasks(),
		Sessions:  store.Agents(),
		Questions: store.HITL(),
		ADRs:      store.ADRs(),
		Events:    store.AgentEvents(),
		PubSub:    redisPubSub,
		HITL:      hitlRouter,
		Threads:   sessionThreads,
		ADREngine: adrEngine,
	})

	// Bootstrap
	if cfg.IsSelfHosted() {
		if _, err := bootstrap.Run(ctx, cfg, store.Tenants(), store.Users(), logger); err != nil { //nolint:govet // short-lived err shadow is idiomatic Go
			return fmt.Errorf("bootstrap: %w", err)
		}
	}

	// Embedded dashboard assets — fs.Sub strips the "build" prefix so the
	// filesystem is rooted directly at the asset files.
	dashboardFS, err := fs.Sub(web.Build, "build")
	if err != nil {
		return fmt.Errorf("embed dashboard assets: %w", err)
	}

	var slackHandler *messengerslack.Handler
	var discordWebhook http.Handler
	var telegramWebhook http.Handler
	emailSender := notify.NewEmailSender(cfg.Email)
	activeNotifierPlatform := messenger.Platform("")
	if cfg.Slack.SigningSecret != "" || cfg.Slack.BotToken != "" {
		verifier := messengerslack.NewNoopVerifier()
		if cfg.Slack.SigningSecret != "" {
			verifier = messengerslack.NewHMACVerifier(cfg.Slack.SigningSecret)
		} else {
			logger.Warn("slack signing secret not configured; using no-op verifier")
		}

		var eventHandler messengerslack.EventHandler
		var interactionHandlers []messengerslack.InteractionHandler
		if cfg.Slack.BotToken != "" {
			slackMessenger := messengerslack.NewMessenger(cfg.Slack.BotToken)
			resolver := messengerslack.NewContextResolver(store.Queries())
			hitlRouter.ConfigureThreadingForPlatform(slackMessenger, sessionThreads)
			notifierDispatcher := notify.NewDispatcher(
				logger,
				slackMessenger.Platform(),
				notify.NewWithEmail(logger, slackMessenger, emailSender),
				slackMessenger,
				sessionThreads,
				store.Queries(),
				store.Tasks(),
				store.Agents(),
				store.HITL(),
				store.Users(),
			)
			orchestratorSvc = orchestrator.New(orchestrator.Deps{
				Logger:    logger,
				Registry:  registry,
				Runtime:   runtimeManager,
				Volumes:   volumeManager,
				GitOps:    gitOperator,
				Projects:  store.Projects(),
				Tasks:     store.Tasks(),
				Sessions:  store.Agents(),
				Questions: store.HITL(),
				ADRs:      store.ADRs(),
				Events:    store.AgentEvents(),
				PubSub:    redisPubSub,
				HITL:      hitlRouter,
				Notifier:  notifierDispatcher,
				Threads:   sessionThreads,
				ADREngine: adrEngine,
			})
			timeoutDispatcher = notifierDispatcher
			activeNotifierPlatform = slackMessenger.Platform()
			eventHandler = messengerslack.NewService(messengerslack.ServiceDeps{
				Logger:     logger,
				Resolver:   resolver,
				Tasks:      store.Tasks(),
				Dispatcher: orchestratorSvc,
				Sender:     slackMessenger,
				Notifier:   slackMessenger,
				Linker:     linkingSvc,
				Resumer:    orchestratorSvc,
				Threads:    sessionThreads,
				Answerer:   hitlRouter,
			})
			interactionHandlers = append(interactionHandlers, messengerslack.NewHITLInteractionHandler(
				logger,
				hitlRouter,
				orchestratorSvc,
				resolver,
				slackMessenger,
			))
		} else {
			logger.Warn("slack bot token not configured; Slack runtime handlers disabled")
		}

		slackHandler = messengerslack.NewHandler(logger, verifier, eventHandler, interactionHandlers...)
	}

	if cfg.Discord.Enabled() {
		discordMessenger := messengerdiscord.NewMessenger(cfg.Discord.BotToken)
		discordResolver := messengerdiscord.NewContextResolver(store.Queries())
		discordService := messengerdiscord.NewService(messengerdiscord.ServiceDeps{
			Logger:     logger,
			Resolver:   discordResolver,
			Tasks:      store.Tasks(),
			Dispatcher: orchestratorSvc,
			Sender:     discordMessenger,
			Notifier:   discordMessenger,
			Linker:     linkingSvc,
			Resumer:    orchestratorSvc,
			Threads:    sessionThreads,
			Answerer:   hitlRouter,
		})
		discordHandler, handlerErr := messengerdiscord.NewHandler(logger, cfg.Discord.PublicKey, discordService)
		if handlerErr != nil {
			return fmt.Errorf("create discord handler: %w", handlerErr)
		}
		discordWebhook = discordHandler
		hitlRouter.ConfigureThreadingForPlatform(discordMessenger, sessionThreads)
		if activeNotifierPlatform == "" {
			timeoutDispatcher = notify.NewDispatcher(
				logger,
				discordMessenger.Platform(),
				notify.NewWithEmail(logger, discordMessenger, emailSender),
				nil,
				sessionThreads,
				store.Queries(),
				store.Tasks(),
				store.Agents(),
				store.HITL(),
				store.Users(),
			)
			activeNotifierPlatform = discordMessenger.Platform()
		}
	}

	if cfg.Telegram.Enabled() {
		telegramMessenger := messengertelegram.NewMessenger(cfg.Telegram.BotToken)
		telegramResolver := messengertelegram.NewContextResolver(store.Queries())
		telegramService := messengertelegram.NewService(messengertelegram.ServiceDeps{
			Logger:     logger,
			Resolver:   telegramResolver,
			Tasks:      store.Tasks(),
			Dispatcher: orchestratorSvc,
			Sender:     telegramMessenger,
			Notifier:   telegramMessenger,
			Linker:     linkingSvc,
			Resumer:    orchestratorSvc,
			Threads:    sessionThreads,
			Answerer:   hitlRouter,
			Questions:  hitlRouter,
		})
		telegramWebhook = messengertelegram.NewHandler(logger, cfg.Telegram.WebhookSecret, telegramService)
		hitlRouter.ConfigureThreadingForPlatform(telegramMessenger, sessionThreads)
		if activeNotifierPlatform == "" {
			timeoutDispatcher = notify.NewDispatcher(
				logger,
				telegramMessenger.Platform(),
				notify.NewWithEmail(logger, telegramMessenger, emailSender),
				nil,
				sessionThreads,
				store.Queries(),
				store.Tasks(),
				store.Agents(),
				store.HITL(),
				store.Users(),
			)
		}
	}

	deps := server.Dependencies{
		V1: v1.Dependencies{
			Audit:         auditSvc,
			Auth:          authSvc,
			Orchestrator:  orchestratorSvc,
			HITLRouter:    hitlRouter,
			Tenants:       store.Tenants(),
			Users:         store.Users(),
			Projects:      store.Projects(),
			Tasks:         store.Tasks(),
			ADRs:          store.ADRs(),
			AgentSessions: store.Agents(),
			AgentEvents:   store.AgentEvents(),
		},
		Hub:             hub,
		RateLimit:       redisClient,
		SlackHandler:    slackHandler,
		DiscordWebhook:  discordWebhook,
		TelegramWebhook: telegramWebhook,
		Linking:         linkingSvc,
		Links:           store.Queries(),
		DashboardAssets: dashboardFS,
	}

	srvAPI := server.New(cfg, logger, authAdapter, deps)

	srv := &http.Server{
		Addr:              cfg.HTTP.Addr,
		Handler:           srvAPI.Handler(),
		ReadHeaderTimeout: cfg.HTTP.ReadHeaderTimeout,
		ReadTimeout:       cfg.HTTP.ReadTimeout,
		WriteTimeout:      cfg.HTTP.WriteTimeout,
		IdleTimeout:       cfg.HTTP.IdleTimeout,
	}

	// Graceful shutdown on SIGINT/SIGTERM.
	ctx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	orchestratorSvc.StartStallDetector(ctx)
	orchestratorSvc.StartRetryScheduler(ctx)
	orchestratorSvc.StartReconciler(ctx)
	go runHITLTimeoutSweepLoop(ctx, logger, timeoutDispatcher, orchestratorSvc, cfg.HITL.ExtendedTimeout) //nolint:gosec // process-scoped shutdown context is intentional for background sweeper

	errCh := make(chan error, 1)
	go func() {
		logger.Info("listening", "addr", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) { //nolint:govet // short-lived err shadow is idiomatic Go
			errCh <- err
		}
		close(errCh)
	}()

	select {
	case <-ctx.Done():
		logger.Info("shutdown signal received")
	case err := <-errCh: //nolint:govet // short-lived err shadow is idiomatic Go
		return fmt.Errorf("server error: %w", err)
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil { //nolint:govet // short-lived err shadow is idiomatic Go
		return fmt.Errorf("shutdown: %w", err)
	}
	if err := orchestratorSvc.Shutdown(shutdownCtx); err != nil { //nolint:govet // short-lived err shadow is idiomatic Go
		return fmt.Errorf("shutdown orchestrator: %w", err)
	}

	logger.Info("server stopped cleanly")
	return nil
}

func newLogger(level string) *slog.Logger {
	var lvl slog.Level
	switch level {
	case "debug":
		lvl = slog.LevelDebug
	case "warn":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}
	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: lvl}))
}

func runHITLTimeoutSweepLoop(ctx context.Context, logger *slog.Logger, dispatcher *notify.Dispatcher, orchestratorSvc *orchestrator.Orchestrator, extendedTimeout time.Duration) {
	if dispatcher == nil {
		return
	}

	if extendedTimeout <= 0 {
		extendedTimeout = time.Hour
	}

	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			// Tier 1: escalate pending -> escalated (extend timeout, notify, do NOT cancel session).
			sweepExpiredHITLQuestions(ctx, logger, dispatcher, now.UTC(), extendedTimeout)
			// Tier 2: escalated questions whose extended timeout expired -> cancel session.
			sweepEscalatedHITLQuestions(ctx, logger, dispatcher, orchestratorSvc, now.UTC())
			retryTimedOutHITLNotifications(ctx, logger, dispatcher)

			timedOutQuestions, timedOutErr := dispatcher.ListTimedOutQuestions(ctx, 50)
			if timedOutErr != nil {
				logger.ErrorContext(ctx, "timed-out HITL retry sweep failed", "error", timedOutErr)
				continue
			}
			for _, question := range timedOutQuestions {
				opCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
				if cancelErr := orchestratorSvc.CancelSession(opCtx, question.TenantID, question.AgentSessionID); cancelErr != nil {
					logger.ErrorContext(ctx, "failed to finalize previously timed-out HITL session",
						"error", cancelErr,
						"question_id", question.ID,
						"session_id", question.AgentSessionID,
					)
				} else {
					tryNotifyTimedOutQuestion(ctx, logger, dispatcher, question)
				}
				cancel()
			}
		}
	}
}

// sweepExpiredHITLQuestions implements tier 1: pending questions whose initial timeout
// expired are escalated — their timeout is extended and a notification is sent, but the
// agent session is NOT cancelled yet.
func sweepExpiredHITLQuestions(
	ctx context.Context,
	logger *slog.Logger,
	dispatcher *notify.Dispatcher,
	now time.Time,
	extendedTimeout time.Duration,
) {
	expiredQuestions, err := dispatcher.ListExpiredPendingQuestions(ctx, now, 50)
	if err != nil {
		logger.ErrorContext(ctx, "hitl tier-1 escalation sweep failed", "error", err)
		return
	}

	for _, question := range expiredQuestions {
		if question.Status != hitl.StatusPending || question.TimeoutAt == nil || question.TimeoutAt.After(now) {
			continue
		}

		newTimeout := now.Add(extendedTimeout)
		opCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		if escErr := dispatcher.EscalateQuestion(opCtx, question.TenantID, question.ID, newTimeout); escErr != nil {
			logger.ErrorContext(ctx, "failed to escalate HITL question",
				"error", escErr,
				"question_id", question.ID,
				"session_id", question.AgentSessionID,
			)
			cancel()
			continue
		}
		cancel()

		// Best-effort notification — failure does not revert the escalation.
		notifyCtx, notifyCancel := context.WithTimeout(context.Background(), 15*time.Second)
		if notifyErr := dispatcher.NotifyQuestionEscalated(notifyCtx, question); notifyErr != nil {
			logger.ErrorContext(ctx, "failed to send HITL escalation notification",
				"error", notifyErr,
				"question_id", question.ID,
				"session_id", question.AgentSessionID,
			)
		}
		notifyCancel()

		logger.InfoContext(ctx, "escalated HITL question (tier 1)",
			"question_id", question.ID,
			"session_id", question.AgentSessionID,
			"new_timeout_at", newTimeout,
		)
	}
}

// sweepEscalatedHITLQuestions implements tier 2: escalated questions whose extended
// timeout has also expired. The session is cancelled and a final timeout notification
// is sent.
func sweepEscalatedHITLQuestions(
	ctx context.Context,
	logger *slog.Logger,
	dispatcher *notify.Dispatcher,
	orchestratorSvc *orchestrator.Orchestrator,
	now time.Time,
) {
	expired, err := dispatcher.ListEscalatedExpiredQuestions(ctx, now, 50)
	if err != nil {
		logger.ErrorContext(ctx, "hitl tier-2 timeout sweep failed", "error", err)
		return
	}

	// Group by session so we cancel each session only once.
	questionsBySession := make(map[domain.AgentSessionID][]*hitl.Question)
	tenantBySession := make(map[domain.AgentSessionID]domain.TenantID)
	for _, q := range expired {
		questionsBySession[q.AgentSessionID] = append(questionsBySession[q.AgentSessionID], q)
		tenantBySession[q.AgentSessionID] = q.TenantID
	}

	for sessionID, questions := range questionsBySession {
		tenantID := tenantBySession[sessionID]

		// Mark all escalated questions as timed out first.
		var markedQuestions []*hitl.Question
		for _, q := range questions {
			opCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			if markErr := dispatcher.MarkEscalatedTimedOut(opCtx, q.TenantID, q.ID); markErr != nil {
				logger.ErrorContext(ctx, "failed to mark escalated HITL question as timed out",
					"error", markErr,
					"question_id", q.ID,
					"session_id", q.AgentSessionID,
				)
				cancel()
				continue
			}
			cancel()
			markedQuestions = append(markedQuestions, q)
		}

		if len(markedQuestions) == 0 {
			continue
		}

		// Cancel the agent session.
		opCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		if cancelErr := orchestratorSvc.CancelSession(opCtx, tenantID, sessionID); cancelErr != nil {
			logger.ErrorContext(ctx, "failed to cancel escalated timed-out HITL session",
				"error", cancelErr,
				"session_id", sessionID,
			)
			cancel()
			continue
		}
		cancel()

		// Send final timeout notifications.
		for _, q := range markedQuestions {
			tryNotifyTimedOutQuestion(ctx, logger, dispatcher, q)
		}

		logger.InfoContext(ctx, "finalized escalated HITL session (tier 2)",
			"session_id", sessionID,
			"questions_timed_out", len(markedQuestions),
		)
	}
}

func retryTimedOutHITLNotifications(ctx context.Context, logger *slog.Logger, dispatcher *notify.Dispatcher) {
	questions, err := dispatcher.ListUnnotifiedTimedOutQuestions(ctx, 50)
	if err != nil {
		logger.ErrorContext(ctx, "timed-out HITL notification retry sweep failed", "error", err)
		return
	}

	for _, question := range questions {
		tryNotifyTimedOutQuestion(ctx, logger, dispatcher, question)
	}
}

func tryNotifyTimedOutQuestion(ctx context.Context, logger *slog.Logger, dispatcher *notify.Dispatcher, question *hitl.Question) {
	if question == nil {
		return
	}

	claimCtx, claimCancel := context.WithTimeout(context.Background(), 15*time.Second)
	claimed, err := dispatcher.ClaimTimeoutNotification(claimCtx, question.TenantID, question.ID)
	claimCancel()
	if err != nil {
		if errors.Is(err, domain.ErrInvalidTransition) {
			return
		}
		logger.ErrorContext(ctx, "failed to claim HITL timeout notification",
			"error", err,
			"question_id", question.ID,
			"session_id", question.AgentSessionID,
		)
		return
	}
	if claimed == nil {
		return
	}

	notifyCtx, notifyCancel := context.WithTimeout(context.Background(), 15*time.Second)
	if notifyErr := dispatcher.NotifyQuestionTimedOut(notifyCtx, question); notifyErr != nil {
		notifyCancel()
		clearCtx, clearCancel := context.WithTimeout(context.Background(), 15*time.Second)
		if clearErr := dispatcher.ClearTimeoutNotificationClaim(clearCtx, question.TenantID, question.ID); clearErr != nil {
			logger.ErrorContext(ctx, "failed to release HITL timeout notification claim",
				"error", clearErr,
				"question_id", question.ID,
				"session_id", question.AgentSessionID,
			)
		}
		clearCancel()
		logger.ErrorContext(ctx, "failed to send HITL timeout notification",
			"error", notifyErr,
			"question_id", question.ID,
			"session_id", question.AgentSessionID,
		)
		return
	}
	notifyCancel()

	markCtx, markCancel := context.WithTimeout(context.Background(), 15*time.Second)
	if markErr := dispatcher.MarkTimeoutNotificationSent(markCtx, question.TenantID, question.ID); markErr != nil {
		markCancel()
		logger.ErrorContext(ctx, "failed to mark HITL timeout notification sent",
			"error", markErr,
			"question_id", question.ID,
			"session_id", question.AgentSessionID,
		)
		return
	}
	markCancel()
}
