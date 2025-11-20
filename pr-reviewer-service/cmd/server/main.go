package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"pr-reviewer-service/internal/config"
	"pr-reviewer-service/internal/db"
	transport "pr-reviewer-service/internal/http"
	"pr-reviewer-service/internal/repository"
	"pr-reviewer-service/internal/service"
)

func main() {
	cfg := config.Load()

	database, err := db.New(cfg.DSN)
	if err != nil {
		log.Fatalf("db connection failed: %v", err)
	}
	defer database.Close()

	teamsRepo := repository.NewTeamsRepo(database)
	usersRepo := repository.NewUsersRepo(database)
	prsRepo := repository.NewPRsRepo(database)

	teamsSvc := service.NewTeamsService(teamsRepo)
	usersSvc := service.NewUsersService(usersRepo)
	prsSvc := service.NewPRService(prsRepo, usersRepo, database)

	handler := transport.NewHandler(teamsSvc, usersSvc, prsSvc)
	router := transport.NewRouter(handler)

	srv := &http.Server{
		Addr:    cfg.Addr,
		Handler: router,
	}

	go func() {
		log.Printf("listening on %s", cfg.Addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server error: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("server shutdown error: %v", err)
	}
}
