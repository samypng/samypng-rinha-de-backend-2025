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
	var fromTime time.Time
	fromTime = time.Unix(0, 0)
	var err error
	if fromISO != "" {
		fromTime, err = time.Parse(time.RFC3339Nano, fromISO)
		if err != nil {
			logs.ShowLogs("Error parsing 'from' time: " + err.Error())
			return c.SendStatus(http.StatusBadRequest)
		}
	}

	var toTime time.Time
	toTime = time.Unix(time.Now().UTC().Unix(), time.Now().UTC().UnixNano())
	if toISO != "" {
		toTime, err = time.Parse(time.RFC3339Nano, toISO)
		if err != nil {
			logs.ShowLogs("Error parsing 'to' time: " + err.Error())
			return c.SendStatus(http.StatusBadRequest)
		}
	}

	summary, err := h.Processor.GetPaymentsSummary(fromTime, toTime)
	if err != nil {
		logs.ShowLogs("Error retrieving payments summary: " + err.Error())
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

// PurgePaymentsHandler clears all payments from the Redis stream.
func (h *Handlers) PurgePaymentsHandler(c *fiber.Ctx) error {
	if err := h.Processor.PurgePayments(); err != nil {
		logs.ShowLogs("Error purging payments: " + err.Error())
		return c.SendStatus(http.StatusInternalServerError)
	}
	if err := h.Processor.RDB.Del(h.Processor.CTX, "payment_hosts:health_status").Err(); err != nil {
		logs.ShowLogs("Error deleting payment hosts health status: " + err.Error())
		return c.SendStatus(http.StatusInternalServerError)
	}

	if err := h.Processor.RDB.Del(h.Processor.CTX, "payments_by_date").Err(); err != nil {
		logs.ShowLogs("Error deleting payments by date: " + err.Error())
		return c.SendStatus(http.StatusInternalServerError)
	}
	logs.ShowLogs("Payments purged successfully")
	return c.SendStatus(http.StatusOK)
}
