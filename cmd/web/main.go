package main

import (
	"context"
	"github.com/bytedance/sonic"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"log"
	"net/http"
	"os"
	"os/signal"
	"rinha-backend-2025/internal/handlers"
	"rinha-backend-2025/internal/payment"
	"syscall"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/redis/go-redis/v9"
)

func main() {
	debug := os.Getenv("DEBUG")
	app := fiber.New(fiber.Config{
		DisableStartupMessage: false,
		JSONEncoder:           sonic.Marshal,
		JSONDecoder:           sonic.Unmarshal,
	})
	if debug != "" && debug != "0" && debug != "false" {
		app.Use(logger.New())
	}
	rdb := redis.NewClient(&redis.Options{
		Addr: os.Getenv("REDIS_ADDR"),
		DB:   0,
	})
	ctx, cancel := context.WithCancel(context.Background())
	if err := rdb.Ping(ctx).Err(); err != nil {
		log.Fatalf("Could not connect to Redis: %v", err)
		return
	}
	err := rdb.XGroupCreateMkStream(ctx, "payments", "payment-group", "0").Err()
	if err != nil && err.Error() != "BUSYGROUP Consumer Group name already exists" {
		return
	}
	handlers := &handlers.Handlers{
		Processor: internal.NewPaymentProcessor(ctx, cancel, rdb, &http.Client{}),
	}

	handlers.Processor.StartWorkerPool()
	go handlers.Processor.ProcessStream()

	app.Post("/payments", handlers.PaymentHandler)
	app.Get("/payments-summary", handlers.PaymentsSummaryHandler)
	app.Post("/purge-payments", handlers.PurgePaymentsHandler)

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)

	go func() {
		if err := app.Listen(":8000"); err != nil {
			log.Printf("Error starting server: %v", err)
		}
	}()

	<-c

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer func() {
		if err := rdb.Close(); err != nil {
			log.Printf("Error closing Redis client: %v", err)
		}
		cancel()
	}()

	if err := app.ShutdownWithContext(shutdownCtx); err != nil {
		log.Printf("Error during server shutdown: %v", err)
	}

	handlers.Processor.StopWorkerPool()

}
