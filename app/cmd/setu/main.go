package main

import (
	"context"
	"fmt"
	"learning-setu/app/internal/config"
	"learning-setu/app/internal/platform"
	"learning-setu/app/internal/storage"
	"learning-setu/app/internal/store"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	if e := run(); e != nil {
		log.Print(e)
		os.Exit(1)
	}
}
func run() error {
	command := "api"
	if len(os.Args) > 1 {
		command = os.Args[1]
	}
	if command == "health" {
		client := http.Client{Timeout: 2 * time.Second}
		r, e := client.Get("http://localhost:3000/api/health")
		if e != nil {
			return fmt.Errorf("unhealthy")
		}
		r.Body.Close()
		if r.StatusCode != 200 {
			return fmt.Errorf("unhealthy")
		}
		return nil
	}
	c, e := config.Load()
	if e != nil {
		return e
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()
	repo, e := store.New(ctx, c)
	if e != nil {
		return e
	}
	objects, e := storage.New(ctx, c)
	if e != nil {
		return e
	}
	a := &platform.Service{Config: c, Repo: repo, Objects: objects}
	switch command {
	case "api":
		server := http.Server{Addr: fmt.Sprintf(":%d", c.Port), Handler: a.Handler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 30 * time.Second, WriteTimeout: 60 * time.Second, IdleTimeout: 60 * time.Second}
		go func() {
			<-ctx.Done()
			shutdown, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			_ = server.Shutdown(shutdown)
		}()
		log.Printf("event=api_started port=%d", c.Port)
		if e = server.ListenAndServe(); e != http.ErrServerClosed {
			return e
		}
		return nil
	case "seed":
		for attempt := 0; attempt < 30; attempt++ {
			if e = a.Seed(ctx); e == nil {
				log.Print("Synthetic local accounts and organizations are ready.")
				return nil
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(2 * time.Second):
			}
		}
		return fmt.Errorf("local seed failed: %w", e)
	case "worker":
		w := platform.NewWorker(a)
		nextSweep := time.Time{}
		for ctx.Err() == nil {
			if e = w.Tick(ctx); e != nil {
				log.Print("event=worker_error")
			}
			if c.Mode != "provisioning" && time.Now().After(nextSweep) {
				if e = w.Sweep(ctx); e != nil {
					log.Print("event=retention_sweep_error")
				}
				nextSweep = time.Now().Add(time.Hour)
			}
			select {
			case <-ctx.Done():
			case <-time.After(time.Second):
			}
		}
		return nil
	case "migrate":
		return platform.Migrate(ctx, a, os.Args[2:])
	case "archive-assessments":
		return platform.MigrateAssessmentResults(ctx, a, os.Args[2:])
	default:
		return fmt.Errorf("usage: setu [api|worker|seed|health|migrate|archive-assessments]")
	}
}
