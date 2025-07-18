package internal

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"rinha-backend-2025/types"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

type PaymentProcessor struct {
	RDB         *redis.Client
	CTX         context.Context
	workerPool  *WorkerPool
	paymentChan chan types.Payment
}

type WorkerPool struct {
	numWorkers  int
	paymentChan chan types.Payment
	wg          sync.WaitGroup
	ctx         context.Context
	processor   *PaymentProcessor
}

// NewPaymentProcessor initializes a new PaymentProcessor with a Redis client and context.
func NewPaymentProcessor(rdb *redis.Client, ctx context.Context) *PaymentProcessor {
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
		paymentChan: paymentChan,
	}

	workerPool := &WorkerPool{
		numWorkers:  numWorkers,
		paymentChan: paymentChan,
		ctx:         ctx,
		processor:   processor,
	}

	processor.workerPool = workerPool
	return processor
}

// Start initializes the worker pool and starts processing payments.
func (wp *WorkerPool) Start() {
	for i := 0; i < wp.numWorkers; i++ {
		wp.wg.Add(1)
		go wp.worker(i)
	}
}

// Stop gracefully stops the worker pool by closing the payment channel and waiting for workers to finish.
func (wp *WorkerPool) Stop() {
	close(wp.paymentChan)
	wp.wg.Wait()
}

// worker processes payments from the payment channel.
func (wp *WorkerPool) worker(id int) {
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

// ProcessPayment adds the payment to redis queue
func (p *PaymentProcessor) ProcessPayment(payment types.Payment) error {
	return p.SendPaymentToQueue(payment)
}

// processPaymentInternal is the actual payment processing logic
func (p *PaymentProcessor) processPaymentInternal(payment types.Payment) error {
	paymentHost := os.Getenv("PAYMENT_HOST_DEFAULT")
	if paymentHost == "" {
		return errors.New("payment host is not set")
	}
	isDefaultProcessor := true
	paymentHostHealthy := p.IsPaymentHostHealthy(paymentHost, false)
	if !paymentHostHealthy {
		paymentHost = os.Getenv("PAYMENT_HOST_FALLBACK")
		isDefaultProcessor = false
		paymentHostHealthy = p.IsPaymentHostHealthy(paymentHost, true)

		if !paymentHostHealthy {
			if err := p.SendPaymentToQueue(payment); err != nil {
				return fmt.Errorf("failed to send payment to queue: %w", err)
			}
			return fmt.Errorf("both payment hosts are unhealthy, payment should retry")
		}
	}
	RequestedAt := time.Now().UTC()
	payment.RequestedAt = RequestedAt.Format(time.RFC3339)
	host := fmt.Sprintf("%s/payments", paymentHost)
	paymentData, err := json.Marshal(payment)
	if err != nil {
		if err := p.SendPaymentToQueue(payment); err != nil {
			return fmt.Errorf("failed to send payment to queue: %w", err)
		}
		return fmt.Errorf("failed to marshal payment data, payment should retry: %w", err)
	}
	req, _ := http.NewRequestWithContext(p.CTX, http.MethodPost, host, io.NopCloser(bytes.NewBuffer(paymentData)))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		if err := p.SendPaymentToQueue(payment); err != nil {
			return fmt.Errorf("failed to send payment to queue: %w", err)
		}
		return fmt.Errorf("failed to process payment: %w, payment should retry", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusUnprocessableEntity {
		if err := p.SendPaymentToQueue(payment); err != nil {
			return fmt.Errorf("failed to send payment to queue: %w", err)
		}
		return fmt.Errorf("payment processing failed with status code: %d, payment should retry", resp.StatusCode)
	}
	payment.IsDefaultProcessor = isDefaultProcessor
	paymentBytes, _ := json.Marshal(payment)
	pipe := p.RDB.Pipeline()
	requestedAtUnix := RequestedAt.Unix()
	pipe.ZAdd(p.CTX, "payments_by_date", redis.Z{
		Score:  float64(requestedAtUnix),
		Member: fmt.Sprintf("%s|%.2f|%t", payment.CorrelationID, payment.Amount, payment.IsDefaultProcessor),
	})
	pipe.HSet(p.CTX, "payments", payment.CorrelationID, paymentBytes)
	_, err = pipe.Exec(p.CTX)
	if err != nil {
		if err := p.SendPaymentToQueue(payment); err != nil {
			return fmt.Errorf("failed to send payment to queue: %w", err)
		}
		return err
	}
	return nil
}

// IsPaymentHostHealthy checks if the payment host is healthy by querying its health endpoint.
func (p *PaymentProcessor) IsPaymentHostHealthy(paymentHost string, isFallback bool) bool {
	paymentHostHealth, err := p.RDB.HGet(p.CTX, "payment_hosts:health_status", paymentHost).Result()
	if err != nil {
		paymentHostHealthStatusPayload, err := p.GetHealthCheck(paymentHost)
		if err != nil {
			return false
		}
		paymentHostHealthStatusPayloadBytes, _ := json.Marshal(paymentHostHealthStatusPayload)
		pipe := p.RDB.Pipeline()
		pipe.HSet(p.CTX, "payment_hosts:health_status", paymentHost, string(paymentHostHealthStatusPayloadBytes))
		pipe.Expire(p.CTX, "payment_hosts:health_status", time.Second*6)
		_, err = pipe.Exec(p.CTX)
		if err != nil {
			log.Printf("Failed to set health status for payment host %s: %v", paymentHost, err)
		}
		return false
	}
	var paymentHostHealthStatusPayload types.PaymentHostHealthStatusPayload
	err = json.Unmarshal([]byte(paymentHostHealth), &paymentHostHealthStatusPayload)
	if err != nil {
		return true
	}
	return !paymentHostHealthStatusPayload.Failing && paymentHostHealthStatusPayload.MinResponseTime < 1000
}

// GetHealthCheck queries the health endpoint of the payment host and returns its health status.
func (p *PaymentProcessor) GetHealthCheck(paymentHost string) (types.PaymentHostHealthStatusPayload, error) {
	healthCheckHost := fmt.Sprintf("%s/payments/service-health", paymentHost)
	healthCheckResponse, err := http.Get(healthCheckHost)
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
	err = json.Unmarshal(body, &paymentHostHealthStatusPayload)
	if err != nil {
		return types.PaymentHostHealthStatusPayload{}, err
	}
	return paymentHostHealthStatusPayload, nil
}

// SendPaymentToQueue sends the payment to the Redis queue for processing.
func (p *PaymentProcessor) SendPaymentToQueue(payment types.Payment) error {
	jobID := payment.CorrelationID
	paymentBytes, _ := json.Marshal(payment)
	pipe := p.RDB.Pipeline()
	pipe.HSet(p.CTX, "payments", jobID, paymentBytes)
	pipe.LPush(p.CTX, "payments_jobs", jobID)
	_, err := pipe.Exec(p.CTX)
	if err != nil {
		return err
	}
	return nil
}

// ProcessQueue processes payments from the Redis queue.
func (p *PaymentProcessor) ProcessQueue() error {
	for {
		job, err := p.RDB.BLPop(p.CTX, 0*time.Second, "payments_jobs").Result()
		if err != nil {
			continue
		}
		jobID := job[1]
		paymentData, err := p.RDB.HGet(p.CTX, "payments", jobID).Result()
		if err != nil {
			continue
		}
		var payment types.Payment
		err = json.Unmarshal([]byte(paymentData), &payment)
		if err != nil {
			continue
		}
		p.paymentChan <- payment
	}
}

// GetPaymentsSummary retrieves the payment summary for a given date range.
func (p *PaymentProcessor) GetPaymentsSummary(startDate, endDate int64) (map[string]*types.PaymentSummary, error) {
	members, err := p.RDB.ZRangeByScore(p.CTX, "payments_by_date", &redis.ZRangeBy{
		Min: fmt.Sprintf("%d", startDate),
		Max: fmt.Sprintf("%d", endDate),
	}).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to query payments by date range: %w", err)
	}

	summary := make(map[string]*types.PaymentSummary)
	summary["default"] = &types.PaymentSummary{}
	summary["fallback"] = &types.PaymentSummary{}

	for _, member := range members {
		parts := strings.Split(member, "|")
		if len(parts) != 3 {
			continue
		}
		amount, err := strconv.ParseFloat(parts[1], 64)
		if err != nil {
			continue
		}
		isDefaultProcessor, err := strconv.ParseBool(parts[2])
		if err != nil {
			continue
		}

		key := "default"
		if !isDefaultProcessor {
			key = "fallback"
		}

		summary[key].TotalRequests++
		summary[key].TotalAmount += amount
	}

	return summary, nil
}
