package handlers

import (
	"github.com/go-playground/validator/v10"

	"net/http"
	"rinha-backend-2025/internal/helpers/logs"
	internal "rinha-backend-2025/internal/payment"
	"rinha-backend-2025/internal/types"
	"time"

	"github.com/gofiber/fiber/v2"
)

var validatorInstance = validator.New()

type Handlers struct {
	Processor *internal.PaymentProcessor
}

// ProcessPayment processes a payment by sending it to redis queue.
func (h *Handlers) PaymentHandler(c *fiber.Ctx) error {
	payment := new(types.Payment)
	if err := c.BodyParser(payment); err != nil {
		return c.SendStatus(http.StatusBadRequest)
	}
	if err := validatorInstance.Struct(payment); err != nil {
		return c.SendStatus(http.StatusBadRequest)
	}
	go h.Processor.ProcessPayment(*payment)

	return c.SendStatus(http.StatusCreated)
}

// PaymentsSummaryHandler retrieves the payment summary for a given date range.
func (h *Handlers) PaymentsSummaryHandler(c *fiber.Ctx) error {
	fromISO := c.Query("from")
	toISO := c.Query("to")
	if fromISO == "" || toISO == "" {
		return c.SendStatus(http.StatusBadRequest)
	}
	logs.ShowLogs("Fetching payments summary from " + fromISO + " to " + toISO)

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

	summary, err := h.Processor.GetPaymentsSummary(from, to)
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
