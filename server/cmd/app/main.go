package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/pprof"
	"os"
	"os/signal"
	"runtime/debug"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unicode"

	"github.com/99designs/gqlgen/graphql"
	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/handler/extension"
	"github.com/99designs/gqlgen/graphql/handler/transport"
	"github.com/99designs/gqlgen/graphql/playground"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/kelseyhightower/envconfig"
	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog/log"
	"github.com/vektah/gqlparser/v2/gqlerror"
	"github.com/wangliang139/NovaForge/server/pkg/action"
	"github.com/wangliang139/NovaForge/server/pkg/action/resolver"
	"github.com/wangliang139/NovaForge/server/pkg/chat"
	"github.com/wangliang139/NovaForge/server/pkg/cronjob"
	"github.com/wangliang139/NovaForge/server/pkg/entity"
	"github.com/wangliang139/NovaForge/server/pkg/entity/account"
	"github.com/wangliang139/NovaForge/server/pkg/entity/document"
	"github.com/wangliang139/NovaForge/server/pkg/gateway/auth"
	ah "github.com/wangliang139/NovaForge/server/pkg/gateway/handler"
	mcpkg "github.com/wangliang139/NovaForge/server/pkg/gateway/mcp"
	"github.com/wangliang139/NovaForge/server/pkg/gateway/stream"
	"github.com/wangliang139/NovaForge/server/pkg/gateway/wsctx"
	"github.com/wangliang139/NovaForge/server/pkg/repos"
	"github.com/wangliang139/NovaForge/server/pkg/service/accountsvc"
	"github.com/wangliang139/NovaForge/server/pkg/service/alertsvc"
	"github.com/wangliang139/NovaForge/server/pkg/service/documentsvc"
	"github.com/wangliang139/NovaForge/server/pkg/service/llmsvc"
	"github.com/wangliang139/NovaForge/server/pkg/service/marketsvc"
	"github.com/wangliang139/NovaForge/server/pkg/service/ordersvc"
	"github.com/wangliang139/NovaForge/server/pkg/service/strategysvc"
	"github.com/wangliang139/NovaForge/server/pkg/service/streamsvc"
	"github.com/wangliang139/NovaForge/server/pkg/service/tgbotsvc"
	"github.com/wangliang139/NovaForge/server/pkg/service/usersvc"
	"github.com/wangliang139/NovaForge/server/pkg/settings"
	"github.com/wangliang139/NovaForge/server/pkg/strategy/proxy"
	"github.com/wangliang139/mow/database/cache"
	"github.com/wangliang139/mow/database/wpgx"
	"github.com/wangliang139/mow/env"
	merrors "github.com/wangliang139/mow/errors"
	"github.com/wangliang139/mow/executors"
	"github.com/wangliang139/mow/ginx"
	"github.com/wangliang139/mow/health"
	"github.com/wangliang139/mow/logger"
	"github.com/wangliang139/mow/otel"
	"github.com/wangliang139/mow/snowflake"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"
)

type Config struct {
	LightWeightMode bool `split_words:"true" default:"false"`

	ExecutorWorkerCount int  `split_words:"true" default:"100"`
	ExecutorQueueSize   int  `split_words:"true" default:"1000"`
	ExecutorWithTrace   bool `split_words:"true" default:"false"`

	PprofPort int `envconfig:"PPROF_PORT" split_words:"true" default:"6060"`
}

var wsConnSeq atomic.Int64

func main() {
	RunApplication()
}

type App struct {
	cfg   Config
	errch chan error

	cronCancel context.CancelFunc

	server http.Server

	db       *repos.Entity
	cache    redis.UniversalClient
	executor *executors.Executor

	// server
	health *health.Server

	// services
	strategySvc *strategysvc.Service
	userSvc     *usersvc.Service
	streamSvc   *streamsvc.Service
	accountSvc  *accountsvc.Service
	orderSvc    *ordersvc.Service
	documentSvc *documentsvc.Service
	llmSvc      *llmsvc.Service
	chatSvc     *chat.Service
	marketSvc   *marketsvc.Service
	tgBotSvc    *tgbotsvc.Service
	alertSvc    *alertsvc.Service
}

// graphql hooks order:
// 1. mutate operation parameters
// 2. mutate operation context
// 3. around operations
// 4. response start
// 5. root fields start
// 6. fields start
// 7. fields end
// 8. root fields end
// 9. response end

func (app *App) startGateway() {
	rsv := &resolver.Resolver{
		StrategySvc:   app.strategySvc,
		UserSvc:       app.userSvc,
		StreamSvc:     app.streamSvc,
		AccountSvc:    app.accountSvc,
		OrderSvc:      app.orderSvc,
		DocumentSvc:   app.documentSvc,
		LlmSvc:        app.llmSvc,
		MarketSvc:     app.marketSvc,
		TgBotSvc:      app.tgBotSvc,
		AlertSvc:      app.alertSvc,
		StreamManager: stream.NewManager(app.streamSvc),
	}
	srv := handler.New(action.NewExecutableSchema(action.Config{Resolvers: rsv}))
	srv.AddTransport(transport.Options{})
	srv.AddTransport(transport.GET{})
	srv.AddTransport(transport.POST{})
	srv.AddTransport(transport.MultipartForm{})
	srv.AddTransport(transport.Websocket{
		Upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return true
			},
		},
		KeepAlivePingInterval: 20 * time.Second,
		InitFunc: func(ctx context.Context, initPayload transport.InitPayload) (context.Context, *transport.InitPayload, error) {
			connID := fmt.Sprintf("ws-%d", wsConnSeq.Add(1))
			ctx = wsctx.WithConnID(ctx, connID)

			token := auth.ExtractToken(initPayload)
			if token == "" {
				return ctx, nil, nil
			}
			claims, err := auth.ValidateToken(token)
			if err != nil {
				return nil, nil, gqlerror.Errorf("invalid or expired token")
			}
			ctx = context.WithValue(ctx, auth.ContextKeyUser, &auth.User{
				ID:     claims.UserID,
				Name:   claims.Name,
				Access: claims.Access,
				Source: auth.AuthSourceJWT,
			})
			return ctx, nil, nil
		},
		CloseFunc: func(ctx context.Context, closeCode int) {
			connID, ok := wsctx.ConnIDFromContext(ctx)
			if !ok {
				return
			}
			rsv.StreamManager.ReleaseAll(connID)
		},
	})
	srv.SetErrorPresenter(func(ctx context.Context, e error) *gqlerror.Error {
		err := graphql.DefaultErrorPresenter(ctx, e)
		if e2, ok := merrors.From(err.Err); ok {
			err.Extensions = map[string]any{
				"code": e2.Code,
			}
			err.Message = e2.Message
		}
		return err
	})
	srv.SetRecoverFunc(func(ctx context.Context, err any) error {
		stack := debug.Stack()
		logger.Ctx(ctx).Error().Stack().
			Err(fmt.Errorf("%s", string(stack))).
			Msgf("GraphQL panic: %v", err)
		return gqlerror.Errorf("internal system error")
	})
	srv.AroundOperations(func(ctx context.Context, next graphql.OperationHandler) graphql.ResponseHandler {
		return auth.CheckApiKeyPermission(ctx, next)
	})
	srv.AroundResponses(func(ctx context.Context, next graphql.ResponseHandler) *graphql.Response {
		res := next(ctx)
		//var (
		//	rc *graphql.Stats
		//	td *apollotracing.TracingExtension
		//)
		//rc = &graphql.GetOperationContext(ctx).Stats
		//td, _ = graphql.GetExtension(ctx, "tracing").(*apollotracing.TracingExtension)
		//log.Info().Interface("stats", rc).Interface("tracing", td).Msg("graphql response ends")
		return res
	})
	// srv.Use(apollotracing.Tracer{})
	srv.AroundRootFields(func(ctx context.Context, next graphql.RootResolver) graphql.Marshaler {
		// log.Info().Msg("graphql root fields starts")
		res := next(ctx)
		// log.Info().Msg("graphql root fields ends")
		return res
	})

	gin.DebugPrintFunc = func(format string, args ...any) {
		log.Debug().Msgf(strings.TrimRightFunc(format, unicode.IsSpace), args...)
	}

	r := gin.New()
	r.Use(otelgin.Middleware(env.ServiceName()))
	r.Use(ginx.TraceHeader())
	r.Use(ginx.Logger())
	r.Use(ginx.Recovery())
	r.Use(cors.New(cors.Config{
		AllowWildcard: true,
		AllowOrigins:  []string{"http://localhost:8000", "http://127.0.0.1:3000", "http://localhost:5173", "http://192.168.3.220:3000", "http://pc.tailc961cf.ts.net:8000"},
		AllowMethods:  []string{"GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS"},
		AllowHeaders: []string{
			"Origin", "Content-Length", "Content-Type", "Authorization", "X-API-Key",
			"Mcp-Protocol-Version", "Mcp-Session-Id", "Last-Event-ID",
		},
		MaxAge: 6 * time.Hour,
	}))

	// MCP Streamable HTTP（仅允许 X-API-Key）
	r.Any("/mcp", auth.APIKeyAuthMiddleware(app.userSvc), gin.WrapH(mcpkg.HTTPHandler(rsv)))

	// GraphQL endpoints
	r.Any("/query", auth.OptionalAuthMiddleware(app.userSvc), gin.WrapH(srv))

	// GraphQL playground
	srv.Use(extension.Introspection{}) // 开启
	r.Any("/", gin.WrapH(playground.Handler("GraphQL playground", "/query")))

	// Auth routes
	authHandler := ah.NewAuthHandler(app.userSvc)
	authGroup := r.Group("/api/auth")
	{
		authGroup.POST("/login", authHandler.Login)

		// Current user (requires auth)
		authGroup.GET("/me", auth.AuthMiddleware(), authHandler.GetCurrentUser)

		// Logout (optional, JWT is stateless)
		authGroup.POST("/logout", authHandler.Logout)
	}

	chatHandler := chat.NewHandler(app.chatSvc)
	chatGroup := r.Group("/api/chat")
	chatGroup.Use(auth.AuthMiddleware())
	chatHandler.Register(chatGroup)

	app.server = http.Server{
		Addr:              fmt.Sprintf(":%d", 3000),
		Handler:           r.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		app.errch <- app.server.ListenAndServe()
	}()
}

func (app *App) serve() {
	log.Info().Msg("application starts")
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP, syscall.SIGQUIT, syscall.SIGUSR1)
	select {
	case s := <-sig:
		log.Warn().Msgf("received signal %s, shutting down", s)
		app.shutdown()
	case err := <-app.errch:
		log.Err(err).Msg("failed to start application, shutting down")
		app.shutdown()
	}
}

func (app *App) shutdown() {
	if app.cronCancel != nil {
		app.cronCancel()
	}
	// shutdown services concurrently and wait for all to finish, e.g. grpc server, etc.
	wg := sync.WaitGroup{}
	stoppers := []func(){
		func() {
			defer wg.Done()
			if app.tgBotSvc != nil {
				log.Info().Msg("shutting down telegram news collector")
				app.tgBotSvc.Close()
			}
		},
		func() {
			defer wg.Done()
			if app.alertSvc != nil {
				log.Info().Msg("shutting down alert service")
				app.alertSvc.Stop()
			}
		},
		func() {
			defer wg.Done()
			if entity.Account != nil {
				log.Info().Msg("shutting down account engine")
				entity.Account.Stop()
			}
		},
		func() {
			defer wg.Done()
			if entity.Market != nil {
				log.Info().Msg("shutting down market data engine")
				ctx := context.Background()
				if err := entity.Market.Stop(ctx); err != nil {
					log.Err(err).Msg("failed to stop market data engine")
				}
			}
		},
		func() {
			defer wg.Done()
			if entity.Strategy != nil {
				log.Info().Msg("shutting down strategy entity")
				ctx := context.Background()
				if err := entity.Strategy.Stop(ctx); err != nil {
					log.Err(err).Msg("failed to stop strategy entity")
				}
			}
		},
	}
	wg.Add(len(stoppers))
	for _, stopper := range stoppers {
		go stopper()
	}
	wg.Wait()
	closers := []func(){
		func() {
			defer wg.Done()
			if app.executor != nil {
				app.executor.Shutdown()
			}
		},
		func() {
			defer wg.Done()
			if app.health != nil {
				app.health.Stop()
			}
		},
		func() {
			defer wg.Done()
			if app.db.ConnPool != nil {
				app.db.ConnPool.Close()
			}
		},
		func() {
			defer wg.Done()
			if app.cache != nil {
				if err := app.cache.Close(); err != nil {
					log.Error().Err(err).Msg("failed to close redis connection")
				}
			}
		},
	}
	wg.Add(len(closers))
	for _, closer := range closers {
		go closer()
	}
	log.Info().Msg("application shutdown")
}

func (app *App) OK(ctx context.Context) (err error) {
	return health.GoCheck(ctx)
}

func (app *App) startHealthChecker() {
	app.health = health.New(nil, app)
	go func() {
		log.Info().Msg("health checker starts")
		app.errch <- app.health.Start()
	}()
}

func (app *App) initDeps() {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	app.connectStorage()

	snowflake.Init(ctx, app.cache)

	app.executor = executors.NewExecutor(app.cfg.ExecutorWorkerCount, app.cfg.ExecutorQueueSize, app.cfg.ExecutorWithTrace)
	app.executor.Start()

	settings.Init(app.db)

	entity.Init(app.cache, app.db, app.executor)
}

func (app *App) connectStorage() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	// create db connection pool
	pool, err := wpgx.NewWPGXPool(ctx, "postgres")
	if err != nil {
		panic(fmt.Errorf("failed to create db connection pool: %w", err))
	}
	// create redis connection
	app.cache = cache.NewRedisClient("redis")
	// create dcache instance
	dCache, err := cache.NewDCache("dcache", app.cache)
	if err != nil {
		panic(fmt.Errorf("failed to create dcache: %w", err))
	}
	app.db = repos.New(pool, dCache)
}

func (app *App) startCronJob() {
	if app.cfg.LightWeightMode {
		return
	}

	cronCtx, cancel := context.WithCancel(context.Background())
	app.cronCancel = cancel

	go func() {
		log.Info().Msg("internal cron scheduler starts")
		cronjob.Run(cronCtx,
			cronjob.Spec{
				Name: "SyncGateCalendar", Interval: time.Minute * 10, ExecuteTimeout: time.Minute * 10,
				Run: func(ctx context.Context) error {
					_, err := entity.Document.SyncGateCalendar(ctx, document.SyncCalendarInput{})
					return err
				},
			},
			cronjob.Spec{
				Name: "RefreshAccountSnapshots", Interval: time.Minute * 5, ExecuteTimeout: time.Minute * 10,
				Run: func(ctx context.Context) error {
					_, err := entity.Account.RefreshAccountSnapshots(ctx, account.RefreshAccountSnapshotsInput{})
					return err
				},
			},
			cronjob.Spec{
				Name: "RefreshAccountEquity", Interval: time.Minute * 5, ExecuteTimeout: time.Minute * 10,
				Run: func(ctx context.Context) error {
					_, err := entity.Account.RefreshAccountEquity(ctx, account.RefreshAccountEquityInput{})
					return err
				},
			},
			cronjob.Spec{
				Name: "AccountRiskCheck", Interval: time.Minute * 5, ExecuteTimeout: time.Minute * 10,
				Run: func(ctx context.Context) error {
					_, err := entity.Account.AccountRiskCheck(ctx, account.AccountRiskCheckInput{})
					return err
				},
			},
			cronjob.Spec{
				Name: "CleanupAlertTriggerEvents", Interval: time.Hour, ExecuteTimeout: time.Minute,
				Run: func(ctx context.Context) error {
					if app.alertSvc == nil {
						return nil
					}
					_, err := app.alertSvc.CleanupEventsBefore(ctx, time.Now().Add(-30*24*time.Hour))
					return err
				},
			},
			cronjob.Spec{
				Name: "CleanOldDocuments", Interval: time.Minute * 5, ExecuteTimeout: time.Minute * 10,
				Run: func(ctx context.Context) error {
					_, err := entity.Document.CleanOldDocuments(ctx, document.CleanOldDocumentsInput{})
					return err
				},
			},
		)
		log.Info().Msg("internal cron scheduler stopped")
	}()
}

func (app *App) createSvc() {
	var err error
	if app.userSvc, err = usersvc.New(app.db); err != nil {
		panic(err)
	}
	if app.streamSvc, err = streamsvc.New(app.db); err != nil {
		panic(err)
	}
	if app.accountSvc, err = accountsvc.New(app.db); err != nil {
		panic(err)
	}
	if app.orderSvc, err = ordersvc.New(app.db, app.cache); err != nil {
		panic(err)
	}
	if app.documentSvc, err = documentsvc.New(); err != nil {
		panic(err)
	}
	if app.llmSvc, err = llmsvc.New(); err != nil {
		panic(err)
	}
	app.chatSvc = chat.NewService(app.db, entity.Llm)
	if app.marketSvc, err = marketsvc.New(app.db); err != nil {
		panic(err)
	}
	if app.strategySvc, err = strategysvc.New(app.db, app.accountSvc, app.orderSvc, app.llmSvc); err != nil {
		panic(err)
	}
	if app.tgBotSvc, err = tgbotsvc.New(app.cache, app.db); err != nil {
		panic(err)
	}
	if app.alertSvc, err = alertsvc.New(app.db, app.streamSvc, entity.Market.Engine().GetMarketProvider()); err != nil {
		panic(err)
	}
}

func (app *App) startService() {
	proxy.AssignStub(
		app.marketSvc.GetMarkets,
		app.marketSvc.GetMarket,
		app.marketSvc.GetPrice,
		app.marketSvc.GetBookPrice,
		app.marketSvc.GetMarkPrice,
		app.marketSvc.GetIndexPrice,
		app.marketSvc.GetTicker,
		app.marketSvc.GetTrades,
		app.marketSvc.GetOrderBook,
		app.marketSvc.GetKlines,
		app.marketSvc.GetHisKlines,
		app.marketSvc.GetFundingRate,
		app.marketSvc.GetHisFundingRates,
		app.marketSvc.GetOpenInterest,
		app.accountSvc.GetSymbolConfig,
		app.accountSvc.GetBalance,
		app.accountSvc.GetPositions,
		app.orderSvc.GetOpenOrders,
		app.orderSvc.GetOrder,
		app.orderSvc.PlaceOrder,
		app.orderSvc.CancelOrder,
		app.accountSvc.GetLeverage,
		app.accountSvc.SetLeverage,
		app.accountSvc.FundsFreeze,
		app.accountSvc.FundsUnfreeze,
		app.streamSvc.SubscribeStream,
	)

	if entity.User != nil {
		if err := entity.User.Start(); err != nil {
			log.Err(err).Msg("failed to start user entity")
		}
	}
	if entity.Market != nil {
		if err := entity.Market.Start(context.Background()); err != nil {
			log.Err(err).Msg("failed to start market engine")
		}
	}
	if entity.Account != nil {
		if err := entity.Account.Start(); err != nil {
			log.Err(err).Msg("failed to start account engine")
		}
	}
	if err := entity.Strategy.Start(); err != nil {
		panic(err)
	}

	if !app.cfg.LightWeightMode {
		entity.Document.Start()
		if app.tgBotSvc != nil {
			go app.tgBotSvc.Start()
		}
	}
	if app.alertSvc != nil {
		if err := app.alertSvc.Start(context.Background()); err != nil {
			log.Err(err).Msg("failed to start alert service")
		}
	}
}

func (app *App) startPprof() {
	go func() {
		log.Info().Msg("pprof server starts")
		mux := http.NewServeMux()
		// mux.Handle("/debug/pprof/", http.DefaultServeMux)
		mux.HandleFunc("/debug/pprof/", pprof.Index)
		mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
		mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
		mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
		mux.HandleFunc("/debug/pprof/trace", pprof.Trace)
		server := &http.Server{
			Addr:    fmt.Sprintf(":%d", app.cfg.PprofPort),
			Handler: mux,
		}
		app.errch <- server.ListenAndServe()
	}()
}

func RunApplication() {
	otel.ConfigureOpenTelemetry()
	logger.RegisterTraceHook()
	defer func() {
		_ = otel.ForceFlush(context.Background())
	}()

	cfg := Config{}
	envconfig.MustProcess("APP", &cfg)

	app := &App{
		cfg:   cfg,
		errch: make(chan error, 1),
	}

	app.initDeps()
	app.createSvc()
	app.startService()
	app.startGateway()
	app.startCronJob()
	app.startHealthChecker()
	app.startPprof()
	app.serve()
}
