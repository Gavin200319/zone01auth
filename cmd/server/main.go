package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "time/tzdata"

	"github.com/joho/godotenv"

	"z01auth/internal/api"
	"z01auth/internal/attendance"
	"z01auth/internal/auth"
	"z01auth/internal/config"
	"z01auth/internal/store"
)

func main() {
	if err := godotenv.Load(".env"); err != nil {
		if err := godotenv.Load("../../.env"); err != nil {
			log.Println("Warning: .env file not found; using system environment variables")
		}
	}

	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}

	ctx := context.Background()
	st, err := store.Open(ctx, cfg)
	if err != nil {
		log.Fatalf("firebase: %v", err)
	}
	pingCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	err = st.Ping(pingCtx)
	cancel()
	if err != nil {
		log.Fatalf("firebase database unreachable (check DATABASE_URL and the service account key): %v", err)
	}

	if err := st.SeedPremises(ctx, cfg.Premises); err != nil {
		log.Fatalf("seed premises: %v", err)
	}
	premises, err := st.LoadPremises(ctx)
	if err != nil {
		log.Fatalf("load premises: %v", err)
	}

	var verifier auth.Verifier = auth.NewGoogleVerifier(cfg.GoogleClientIDs)
	if cfg.DevAuth {
		log.Println("WARNING: DEV_AUTH is enabled; id_token 'dev:<email>' is accepted")
		verifier = auth.NewDevVerifier()
	}

	h := &api.Handler{
		Cfg:    cfg,
		Store:  st,
		Tokens: auth.NewIssuer(cfg.JWTSecret, cfg.AccessTTL),
		Att: &attendance.Service{
			Store:        st,
			Premises:     premises,
			Loc:          cfg.Location,
			MaxAccuracyM: cfg.MaxGPSAccuracyM,
			RequireBio:   cfg.RequireBiometricTimeIn,
			Now:          time.Now,
		},
		Verifier: verifier,
	}
	app := api.New(h)

	workerCtx, cancelWorker := context.WithCancel(ctx)
	defer cancelWorker()
	st.StartCleanupWorker(workerCtx, 30*time.Minute)

	go func() {
		if err := app.Listen(":" + cfg.Port); err != nil {
			log.Fatalf("listen: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	log.Println("shutting down")
	if err := app.ShutdownWithTimeout(10 * time.Second); err != nil {
		log.Printf("shutdown: %v", err)
	}
}
