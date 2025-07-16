package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"os/signal"
	"rinha-backend-2025/internal"
	"rinha-backend-2025/types"
	"syscall"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/redis/go-redis/v9"
)

type Handlers struct {
	processor *internal.PaymentProcessor
}

// ProcessPayment processes a payment by sending it to redis queue.
func (h *Handlers) PaymentHandler(c *fiber.Ctx) error {
	body := c.Body()
	var payment types.Payment
	if err := json.Unmarshal(body, &payment); err != nil {
		return c.SendStatus(http.StatusBadRequest)
	}

	go h.processor.ProcessPayment(payment)

	return c.SendStatus(http.StatusCreated)
}

func (h *Handlers) PaymentsSummaryHandler(c *fiber.Ctx) error {
	summary := map[string]interface{}{
		"total_payments": 100,
		"currency":       "USD",
	}

	return c.JSON(summary)
}

func main() {
	app := fiber.New(fiber.Config{
		DisableStartupMessage: false,
	})

	rdb := redis.NewClient(&redis.Options{
		Addr: os.Getenv("REDIS_ADDR"),
		DB:   0,
	})
	ctx := context.Background()
	if err := rdb.Ping(ctx).Err(); err != nil {
		log.Fatalf("Could not connect to Redis: %v", err)
	}

	handlers := &Handlers{
		processor: internal.NewPaymentProcessor(rdb, ctx),
	}

	handlers.processor.StartWorkerPool()
	go handlers.processor.ProcessQueue()

	app.Post("/payment", handlers.PaymentHandler)
	app.Get("/payments/summary", handlers.PaymentsSummaryHandler)


	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)

	go func() {
		if err := app.Listen(":8000"); err != nil {
			log.Printf("Error starting server: %v", err)
		}
	}()

	<-c

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := app.ShutdownWithContext(shutdownCtx); err != nil {
		log.Printf("Error during server shutdown: %v", err)
	}

	handlers.processor.StopWorkerPool()

}
