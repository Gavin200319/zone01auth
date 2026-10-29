package api

import (
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/limiter"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
)

func rateLimit(max int) fiber.Handler {
	return limiter.New(limiter.Config{
		Max:        max,
		Expiration: time.Minute,
		LimitReached: func(c *fiber.Ctx) error {
			return newErr(fiber.StatusTooManyRequests, "RATE_LIMITED", "too many requests, slow down")
		},
	})
}

func New(h *Handler) *fiber.App {
	cfg := fiber.Config{
		AppName:      "z01auth",
		ErrorHandler: errorHandler,
		BodyLimit:    1 << 20,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}
	if h.Cfg.TrustProxy {
		cfg.ProxyHeader = fiber.HeaderXForwardedFor
	}
	app := fiber.New(cfg)

	app.Use(recover.New())
	app.Use(logger.New())
	if h.Cfg.CORSOrigins != "" {
		app.Use(cors.New(cors.Config{
			AllowOrigins: h.Cfg.CORSOrigins,
			AllowHeaders: "Authorization, Content-Type",
			AllowMethods: "GET, POST, PUT, PATCH, DELETE, OPTIONS",
		}))
	}
	app.Use(rateLimit(300))

	app.Get("/health", h.Health)

	v1 := app.Group("/api/v1")
	v1.Get("/config", h.Config)
	v1.Post("/proximity/check", rateLimit(60), h.ProximityCheck)

	a := v1.Group("/auth", rateLimit(30))
	a.Post("/google", h.GoogleLogin)
	a.Post("/biometric/challenge", h.BiometricLoginChallenge)
	a.Post("/biometric/login", h.BiometricLogin)
	a.Post("/refresh", h.Refresh)
	a.Post("/logout", h.RequireAuth, h.Logout)

	v1.Get("/me", h.RequireAuth, h.Me)
	v1.Patch("/me/settings", h.RequireAuth, h.UpdateSettings)
	v1.Put("/me/device/biometric-key", h.RequireAuth, h.SetBiometricKey)
	v1.Get("/me/devices", h.RequireAuth, h.ListDevices)
	v1.Delete("/me/devices/:id", h.RequireAuth, h.DeleteDevice)

	att := v1.Group("/attendance")
	att.Get("/today", h.RequireAuth, h.AttendanceToday)
	att.Post("/biometric/challenge", h.RequireAuth, h.AttendanceChallenge)
	att.Post("/heartbeat", h.RequireAuth, h.Heartbeat)
	att.Get("/history", h.RequireAuth, h.History)
	att.Get("/months", h.RequireAuth, h.Months)

	return app
}
