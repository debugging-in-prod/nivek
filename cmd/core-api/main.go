package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/sourcegraph/conc/pool"
	"github.com/tim-the-toolman-taylor/nivek/cmd/core-api/coreconfig"
	"github.com/tim-the-toolman-taylor/nivek/cmd/core-api/routes"
	"github.com/tim-the-toolman-taylor/nivek/internal/libraries/nivek"
)

func main() {
	nivek.Bootstrap(
		nivek.BootstrapParameters{
			NivekServiceConfig: nivek.NivekServiceConfig{
				UsePSQL: true,

				//
				// Startup connections

				RequireStartupConnections:  true,
				StartupConnectionsPostgres: nivek.GetStartupConnectionsForPostgres(),
			},
			CustomConfig: coreconfig.GetCoreApiConfig(),
		},
		func(nivek nivek.NivekService, ctx context.Context) error {
			// Type assertion to convert interface{} to CoreApiConfig
			cfg, ok := nivek.CustomConfig().(coreconfig.CoreApiConfig)
			if !ok {
				panic("failed to assert custom config")
			}

			fmt.Println("=======================================")
			fmt.Println("=======================================")
			fmt.Println("Hello World! - ", nivek.CommonConfig().AppName)
			fmt.Println("=======================================")
			fmt.Println("=======================================")

			//
			// Start the API server
			e := echo.New()

			//
			// Middleware
			// e.Use(nivekmiddleware.NewJWTMiddleware(nivek).Run())

			e.Use(middleware.Gzip())

			e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
				AllowOrigins: []string{"http://localhost"},
				AllowMethods: []string{echo.GET, echo.POST, echo.PUT, echo.DELETE},
			}))

			// Static SPA served from /app/dist (baked into the image alongside
			// the Go binary; see Dockerfile.core-api.prod). HTML5 fallback means
			// unknown paths return index.html so Vue Router can take over
			// client-side. Skip /api/* so API route handlers run instead.
			e.Use(middleware.StaticWithConfig(middleware.StaticConfig{
				Root:   "/app/dist",
				Index:  "index.html",
				HTML5:  true,
				Browse: false,
				Skipper: func(c echo.Context) bool {
					return strings.HasPrefix(c.Request().URL.Path, "/api")
				},
			}))

			//
			// Register REST routes under /api so the root is free for the SPA.
			// External URLs are unchanged (nginx used to strip /api/ before
			// proxying; now Echo's group does the equivalent in-process).
			api := e.Group("/api")

			// Liveness/readiness probe for the container healthcheck and
			// zero-downtime rollouts. Dependency-free on purpose: a 200 means
			// the HTTP server is accepting requests (startup already blocks on
			// required DB connections).
			api.GET("/healthz", func(c echo.Context) error {
				return c.String(http.StatusOK, "ok")
			})

			routes.RegisterRoutes(nivek, api)

			//
			// Graceful shutdown
			nivek.RegisterShutdownHandler(func(ctx context.Context) error {
				nivek.Logger().Infof("graceful shutdown - initiated")

				// wait for requests to complete
				if err := e.Shutdown(context.Background()); err != nil {
					nivek.Logger().Errorf("graceful shutdown - error occurred during REST shutdown: %s", err.Error())
				}

				nivek.Logger().Infof("graceful shutdown - closing connections")

				closers := []func() error{
					nivek.Postgres().Close,
				}

				p := pool.New().WithContext(context.Background())

				for i := range closers {
					closer := closers[i]

					p.Go(func(_ context.Context) error {
						return closer()
					})
				}

				// flush remaining data and close connections
				if err := p.Wait(); err != nil {
					nivek.Logger().Errorf("failed to close connections: %s", err.Error())
				}

				nivek.Logger().Infof("graceful shutdown - done")

				return nil
			})

			nivek.Logger().Infof("starting REST server on port %s", cfg.ApiServerPort)

			if err := e.Start(fmt.Sprintf("%s:%s", cfg.ListenAddress, cfg.ApiServerPort)); err != nil {
				if !errors.Is(err, http.ErrServerClosed) {
					return err
				}
			}

			return nil
		},
	)
}
