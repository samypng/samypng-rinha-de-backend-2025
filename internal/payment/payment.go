package internal

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"os"
	"rinha-backend-2025/internal/helpers/logs"
	"rinha-backend-2025/internal/types"
	"strconv"
	"sync"
	"time"

	"github.com/bytedance/sonic"
	"github.com/redis/go-redis/v9"
)

var (
	PaymentHostDefault  = os.Getenv("PAYMENT_HOST_DEFAULT")
	PaymentHostFallback = os.Getenv("PAYMENT_HOST_FALLBACK")
)

type PaymentProcessor struct {
	RDB         *redis.Client
	CTX         context.Context
	Cancel      context.CancelFunc
	workerPool  *WorkerPool
	client      *http.Client
	paymentChan chan types.Payment
}

type WorkerPool struct {
	numWorkers  int
	paymentChan chan types.Payment
	wg          sync.WaitGroup
	ctx         context.Context
	cancel      context.CancelFunc
	processor   *PaymentProcessor
}

// NewPaymentProcessor initializes a new PaymentProcessor with a Redis client and context.
func NewPaymentProcessor(ctx context.Context, cancel context.CancelFunc, rdb *redis.Client, client *http.Client) *PaymentProcessor {
	var numWorkers int
	if envWorkers := os.Getenv("PAYMENT_WORKERS"); envWorkers != "" {
		nw, err := strconv.Atoi(envWorkers)
		if err != nil {
			log.Fatalf("Invalid PAYMENT_WORKERS value: %v", err)
		}
		numWorkers = nw
	} else {
		numWorkers = 10
	}

	var paymentChannelSize int
	if envPaymentChannelSize := os.Getenv("PAYMENT_CHANNEL_SIZE"); envPaymentChannelSize != "" {
		pcs, err := strconv.Atoi(envPaymentChannelSize)
		if err != nil {
			log.Fatalf("Invalid PAYMENT_CHANNEL_SIZE value: %v", err)
		}
		paymentChannelSize = pcs
	} else {
		paymentChannelSize = 100
	}

	paymentChan := make(chan types.Payment, paymentChannelSize)

	processor := &PaymentProcessor{
		RDB:         rdb,
		CTX:         ctx,
		Cancel:      cancel,
		paymentChan: paymentChan,
		client:      client,
	}

	workerPool := &WorkerPool{
		numWorkers:  numWorkers,
		paymentChan: paymentChan,
		ctx:         ctx,
		cancel:      cancel,
		processor:   processor,
	}

	processor.workerPool = workerPool
	return processor
}

// Start initializes the worker pool and starts processing payments.
func (wp *WorkerPool) Start() {
	for i := 0; i < wp.numWorkers; i++ {
		wp.wg.Add(1)
		go wp.worker()
	}
}

// Stop gracefully stops the worker pool by closing the payment channel and waiting for workers to finish.
func (wp *WorkerPool) Stop() {
	wp.cancel()
	close(wp.paymentChan)
	wp.wg.Wait()
}

// worker processes payments from the payment channel.
func (wp *WorkerPool) worker() {
	defer wp.wg.Done()
	for {
		select {
		case payment, ok := <-wp.paymentChan:
			if !ok {
				return
			}
			wp.processor.processPaymentInternal(payment)
		case <-wp.ctx.Done():
			return
		}
	}
}

// StartWorkerPool starts the worker pool for processing payments.
func (p *PaymentProcessor) StartWorkerPool() {
	p.workerPool.Start()
}

// StopWorkerPool stops the worker pool and waits for all workers to finish.
func (p *PaymentProcessor) StopWorkerPool() {
	p.workerPool.Stop()
}

// ProcessPayment adds the payment to redis stream
func (p *PaymentProcessor) ProcessPayment(payment types.Payment) error {
	return p.SendPaymentToStream(payment)
}

// processPaymentInternal is the actual payment processing logic
func (p *PaymentProcessor) processPaymentInternal(payment types.Payment) error {
	logs.ShowLogs(fmt.Sprintf("Processing payment: %s", payment.CorrelationID))
	paymentHost, isDefaultProcessor, err := p.determinePaymentHost()
	if err != nil {
		return p.handleUnhealthyHosts(payment, err)
	}

	RequestedAt := time.Now().UTC()
	payment.RequestedAt = RequestedAt.Format(time.RFC3339Nano)
	paymentData, err := p.preparePaymentData(payment)
	if err != nil {
		return p.handlePaymentError(payment, err, "failed to marshal payment data")
	}

	isDefaultProcessor, err = p.sendPaymentRequest(paymentHost, paymentData, isDefaultProcessor)
	if err != nil {
		logs.ShowLogs(fmt.Sprintf("Payment processing failed for %s: %v", payment.CorrelationID, err))
		return p.handlePaymentError(payment, err, "failed to process payment")
	}

	err = p.savePaymentToRedis(payment, isDefaultProcessor)
	if err != nil {
		logs.ShowLogs(fmt.Sprintf("Failed to save payment to Redis: %v", err))
		return p.handlePaymentError(payment, err, "failed to save payment to Redis")
	}
	logs.ShowLogs(fmt.Sprintf("Payment processed successfully: %s", payment.CorrelationID))
	return nil
}

// Helper function to determine the payment host
func (p *PaymentProcessor) determinePaymentHost() (string, bool, error) {
	paymentHost := PaymentHostDefault
	isDefaultProcessor := true

	if !p.IsPaymentHostHealthy(paymentHost) {
		paymentHost = PaymentHostFallback
		isDefaultProcessor = false

		if !p.IsPaymentHostHealthy(paymentHost) {
			return "", false, fmt.Errorf("both payment hosts are unhealthy")
		}
	}

	return paymentHost, isDefaultProcessor, nil
}

// Helper function to handle unhealthy hosts
func (p *PaymentProcessor) handleUnhealthyHosts(payment types.Payment, err error) error {
	if sendErr := p.SendPaymentToStream(payment); sendErr != nil {
		logs.ShowLogs(fmt.Sprintf("Failed to send payment to stream: %v", sendErr))
		return fmt.Errorf("failed to send payment to stream: %w", sendErr)
	}
	return fmt.Errorf("payment should retry: %w", err)
}

// Helper function to prepare payment data
func (p *PaymentProcessor) preparePaymentData(payment types.Payment) ([]byte, error) {
	paymentData, err := sonic.ConfigFastest.Marshal(payment)
	if err != nil {
		return nil, err
	}
	return paymentData, nil
}

// Helper function to send payment request
func (p *PaymentProcessor) sendPaymentRequest(paymentHost string, paymentData []byte, isDefaultProcessor bool) (bool, error) {
	for {
		host := fmt.Sprintf("%s/payments", paymentHost)
		req, _ := http.NewRequestWithContext(p.CTX, http.MethodPost, host, io.NopCloser(bytes.NewBuffer(paymentData)))
		req.Header.Set("Content-Type", "application/json")
		resp, err := p.client.Do(req)
		if err != nil {
			return isDefaultProcessor, err
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusUnprocessableEntity {
			if isDefaultProcessor {
				paymentHost = PaymentHostFallback
				isDefaultProcessor = false
				continue
			}
			return isDefaultProcessor, fmt.Errorf("payment processing failed with status code: %d", resp.StatusCode)
		}
		return isDefaultProcessor, nil
	}
}

// Helper function to save payment to Redis
func (p *PaymentProcessor) savePaymentToRedis(payment types.Payment, isDefaultProcessor bool) error {
	payment.IsDefaultProcessor = isDefaultProcessor
	pipe := p.RDB.Pipeline()
	paymentBytes, _ := sonic.ConfigFastest.Marshal(payment)
	pipe.HSet(p.CTX, "payments:processed", payment.CorrelationID, string(paymentBytes))
	_, err := pipe.Exec(p.CTX)
	return err
}

// Helper function to handle payment errors
func (p *PaymentProcessor) handlePaymentError(payment types.Payment, err error, message string) error {
	if sendErr := p.SendPaymentToStream(payment); sendErr != nil {
		return fmt.Errorf("%s: %w", message, sendErr)
	}
	return fmt.Errorf("%s: %w", message, err)
}

// IsPaymentHostHealthy checks if the payment host is healthy by querying its health endpoint.
func (p *PaymentProcessor) IsPaymentHostHealthy(paymentHost string) bool {
	paymentHostHealth, err := p.RDB.HGet(p.CTX, "payment_hosts:health_status", paymentHost).Result()
	if err != nil {
		if err == redis.Nil {
			go func() {
				paymentHostHealthStatusPayload, err := p.GetHealthCheck(paymentHost)
				if err != nil {
					return
				}
				paymentHostHealthStatusPayloadBytes, _ := sonic.ConfigFastest.Marshal(paymentHostHealthStatusPayload)
				pipe := p.RDB.Pipeline()
				pipe.HSet(p.CTX, "payment_hosts:health_status", paymentHost, string(paymentHostHealthStatusPayloadBytes))
				pipe.Expire(p.CTX, "payment_hosts:health_status", time.Second*6)
				_, err = pipe.Exec(p.CTX)
				if err != nil {
					logs.ShowLogs(fmt.Sprintf("Failed to set health status for payment host %s: %v", paymentHost, err))
				}
			}()
			return true
		}
		return false
	}
	var paymentHostHealthStatusPayload types.PaymentHostHealthStatusPayload
	err = sonic.ConfigFastest.Unmarshal([]byte(paymentHostHealth), &paymentHostHealthStatusPayload)
	if err != nil {
		return true
	}
	return !paymentHostHealthStatusPayload.Failing && paymentHostHealthStatusPayload.MinResponseTime < 1000
}

// GetHealthCheck queries the health endpoint of the payment host and returns its health status.
func (p *PaymentProcessor) GetHealthCheck(paymentHost string) (types.PaymentHostHealthStatusPayload, error) {
	healthCheckHost := fmt.Sprintf("%s/payments/service-health", paymentHost)
	healthCheckResponse, err := p.client.Get(healthCheckHost)
	if err != nil {
		return types.PaymentHostHealthStatusPayload{}, err
	}
	if healthCheckResponse.StatusCode != http.StatusOK {
		return types.PaymentHostHealthStatusPayload{
			Failing:         false,
			MinResponseTime: 0,
		}, nil
	}
	defer healthCheckResponse.Body.Close()
	body, err := io.ReadAll(healthCheckResponse.Body)
	if err != nil {
		return types.PaymentHostHealthStatusPayload{}, err
	}
	var paymentHostHealthStatusPayload types.PaymentHostHealthStatusPayload
	err = sonic.ConfigFastest.Unmarshal(body, &paymentHostHealthStatusPayload)
	if err != nil {
		return types.PaymentHostHealthStatusPayload{}, err
	}
	return paymentHostHealthStatusPayload, nil
}

// SendPaymentToStream sends the payment to the Redis stream for processing.
func (p *PaymentProcessor) SendPaymentToStream(payment types.Payment) error {
	jobID := payment.CorrelationID
	paymentBytes, _ := sonic.ConfigFastest.Marshal(payment)
	xAddArgs := &redis.XAddArgs{
		Stream: "payments",
		Values: map[string]interface{}{
			"jobID":   jobID,
			"payment": string(paymentBytes),
		},
	}

	_, err := p.RDB.XAdd(p.CTX, xAddArgs).Result()
	if err != nil {
		logs.ShowLogs(fmt.Sprintf("Failed to send payment to Redis stream: %v", err))
		return err
	}
	return nil
}

// ProcessStream processes payments from the Redis stream.
func (p *PaymentProcessor) ProcessStream() error {
	for {
		select {
		case <-p.CTX.Done():
			return nil
		default:
		streams, err := p.RDB.XReadGroup(p.CTX, &redis.XReadGroupArgs{
			Group:    "payment-group",
			Consumer: "payment-consumer",
			Streams:  []string{"payments", ">"},
			Block:    0,
			NoAck:    true,
		}).Result()
		if err != nil {
			continue
		}
		for _, stream := range streams {
			for _, message := range stream.Messages {
				paymentData := message.Values["payment"].(string)
				var payment types.Payment
				if err := sonic.ConfigFastest.Unmarshal([]byte(paymentData), &payment); err != nil {
					continue
				}
				p.paymentChan <- payment
				}
			}
		}
	}
}

// GetPaymentsSummary retrieves the payment summary for a given date range.
func (p *PaymentProcessor) GetPaymentsSummary(startDate, endDate time.Time) (map[string]*types.PaymentSummary, error) {
	payments, err := p.RDB.HGetAll(p.CTX, "payments:processed").Result()
	if err != nil {
		return nil, fmt.Errorf("failed to query payments by date range: %w", err)
	}

	summary := make(map[string]*types.PaymentSummary)
	summary["default"] = &types.PaymentSummary{}
	summary["fallback"] = &types.PaymentSummary{}

	for _, paymentData := range payments {
		var payment types.Payment
		if err := sonic.ConfigFastest.Unmarshal([]byte(paymentData), &payment); err != nil {
			continue
		}
		requestedAt, err := time.Parse(time.RFC3339Nano, payment.RequestedAt)
		if err != nil {
			logs.ShowLogs(fmt.Sprintf("Error parsing payment requested at time: %v", err))
			continue
		}
		if requestedAt.Before(startDate) || requestedAt.After(endDate) {
			continue
		}
		key := "default"
		if !payment.IsDefaultProcessor {
			key = "fallback"
		}

		summary[key].TotalRequests++

		summary[key].TotalAmount += payment.Amount
	}
	summary["default"].TotalAmount = math.Round(summary["default"].TotalAmount*10) / 10
	summary["fallback"].TotalAmount = math.Round(summary["fallback"].TotalAmount*10) / 10
	return summary, nil
}

// PurgePayments clears all payments from the Redis stream.
func (p *PaymentProcessor) PurgePayments() error {
	_, err := p.RDB.XTrimMaxLen(p.CTX, "payments", 0).Result()
	if err != nil {
		logs.ShowLogs(fmt.Sprintf("Failed to purge payments: %v", err))
		return err
	}
	_, err = p.RDB.Del(p.CTX, "payments:processed").Result()
	if err != nil {
		logs.ShowLogs(fmt.Sprintf("Failed to clear processed payments: %v", err))
		return err
	}
	logs.ShowLogs("Payments purged successfully")
	return nil
}
