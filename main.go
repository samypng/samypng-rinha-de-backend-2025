package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"rinha-backend-2025/internal"
	"rinha-backend-2025/types"
	"syscall"
	"time"

	"github.com/go-playground/validator/v10"
	_ "github.com/go-sql-driver/mysql"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/redis/go-redis/v9"
)

type Handlers struct {
	processor *internal.PaymentProcessor
}

var validatorInstance = validator.New()

// ProcessPayment processes a payment by sending it to redis queue.
func (h *Handlers) PaymentHandler(c *fiber.Ctx) error {
	payment := new(types.Payment)
	if err := c.BodyParser(payment); err != nil {
		log.Printf("Error parsing payment: %v", err)
		return c.SendStatus(http.StatusBadRequest)
	}
	if err := validatorInstance.Struct(payment); err != nil {
		log.Printf("Validation error: %v", err)
		return c.SendStatus(http.StatusBadRequest)
	}
	if exist, err := h.processor.RDB.HExists(h.processor.CTX, "payments", payment.CorrelationID).Result(); err != nil || exist {
		return c.SendStatus(http.StatusConflict)
	}

	go h.processor.ProcessPayment(*payment)

	return c.SendStatus(http.StatusCreated)
}

// PaymentsSummaryHandler retrieves the payment summary for a given date range.
func (h *Handlers) PaymentsSummaryHandler(c *fiber.Ctx) error {
	fromISO := c.Query("from")
	toISO := c.Query("to")
	if fromISO == "" || toISO == "" {
		return c.SendStatus(http.StatusBadRequest)
	}

	fromTime, err := time.Parse(time.RFC3339, fromISO)
	if err != nil {
		return c.SendStatus(http.StatusBadRequest)
	}
	toTime, err := time.Parse(time.RFC3339, toISO)
	if err != nil {
		return c.SendStatus(http.StatusBadRequest)
	}

	from := fromTime.Unix()
	to := toTime.Unix()

	summary, err := h.processor.GetPaymentsSummary(from, to)
	if err != nil {
		return c.SendStatus(http.StatusInternalServerError)
	}

	if _, ok := summary["default"]; !ok {
		summary["default"] = &types.PaymentSummary{
			TotalRequests: 0,
			TotalAmount:   0.0,
		}
	}
	if _, ok := summary["fallback"]; !ok {
		summary["fallback"] = &types.PaymentSummary{
			TotalRequests: 0,
			TotalAmount:   0.0,
		}
	}
	return c.Status(http.StatusOK).JSON(summary)
}

func main() {
	app := fiber.New(fiber.Config{
		DisableStartupMessage: false,
	})
	app.Use(logger.New())

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
	defer func() {
		if err := rdb.Close(); err != nil {
			log.Printf("Error closing Redis client: %v", err)
		}
		cancel()
	}()

	if err := app.ShutdownWithContext(shutdownCtx); err != nil {
		log.Printf("Error during server shutdown: %v", err)
	}

	handlers.processor.StopWorkerPool()

}
