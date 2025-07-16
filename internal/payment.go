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
	"sync"
	"time"

	"github.com/gofrs/uuid"
	"github.com/redis/go-redis/v9"
)

type PaymentProcessor struct {
	rdb         *redis.Client
	ctx         context.Context
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
		rdb:         rdb,
		ctx:         ctx,
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
			if err := wp.processor.processPaymentInternal(payment); err != nil {
				log.Printf("Worker %d error processing payment %s: %v", id, payment.CorrelationID, err)
			}
		case <-wp.ctx.Done():
			log.Printf("Worker %d shutting down due to context cancellation", id)
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
	paymentHostHealthy := p.IsPaymentHostHealthy(paymentHost, false)
	if !paymentHostHealthy {
		paymentHost = os.Getenv("PAYMENT_HOST_FALLBACK")
		paymentHostHealthy = p.IsPaymentHostHealthy(paymentHost, true)
		if !paymentHostHealthy {
			if err := p.SendPaymentToQueue(payment); err != nil {
				return fmt.Errorf("failed to send payment to retry queue: %w", err)
			}
			return fmt.Errorf("both payment hosts are unhealthy, payment sent to retry queue")
		}
	}

	host := fmt.Sprintf("%s/payments", paymentHost)
	payment.RequestedAt = time.Now().UTC().Format(time.RFC3339)
	paymentData, err := json.Marshal(payment)
	if err != nil {
		return fmt.Errorf("failed to marshal payment data: %w", err)
	}
	req, err := http.NewRequestWithContext(p.ctx, http.MethodPost, host, io.NopCloser(bytes.NewBuffer(paymentData)))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		if err := p.SendPaymentToQueue(payment); err != nil {
			return fmt.Errorf("failed to send payment to retry queue: %w", err)
		}
		return fmt.Errorf("failed to process payment: %w, payment sent to retry queue", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		if err := p.SendPaymentToQueue(payment); err != nil {
			return fmt.Errorf("failed to send payment to retry queue: %w", err)
		}
		return fmt.Errorf("payment processing failed with status code: %d, payment sent to retry queue", resp.StatusCode)
	}
	return nil
}

// IsPaymentHostHealthy checks if the payment host is healthy by querying its health endpoint.
func (p *PaymentProcessor) IsPaymentHostHealthy(paymentHost string, isFallback bool) bool {
	paymentHostHealth, err := p.rdb.HGet(p.ctx, "payment_hosts:health", paymentHost).Result()
	if err != nil {
		paymentHostHealthStatusPayload, err := p.GetHealthCheck(paymentHost)
		if err != nil {
			return false
		}
		p.rdb.HSet(p.ctx, "payment_hosts:health", paymentHost, paymentHostHealthStatusPayload, time.Minute*1)
		return true
	}
	var paymentHostHealthStatusPayload types.PaymentHostHealthStatusPayload
	err = json.Unmarshal([]byte(paymentHostHealth), &paymentHostHealthStatusPayload)
	if err != nil {
		return true
	}
	return !paymentHostHealthStatusPayload.Failing
}

// GetHealthCheck queries the health endpoint of the payment host and returns its health status.
func (p *PaymentProcessor) GetHealthCheck(paymentHost string) (types.PaymentHostHealthStatusPayload, error) {
	healthCheckHost := fmt.Sprintf("%s/health", paymentHost)
	healthCheckResponse, err := http.Get(healthCheckHost)
	if err != nil {
		return types.PaymentHostHealthStatusPayload{}, err
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
	jobID, _ := uuid.NewV4()
	p.rdb.LPush(p.ctx, "payments", jobID.String())
	p.rdb.HSet(p.ctx, "payments", jobID, payment)
	return nil
}

// ProcessQueue processes payments from the Redis queue.
func (p *PaymentProcessor) ProcessQueue() error {

	for {
		job, err := p.rdb.BLPop(p.ctx, 0*time.Second, "payments").Result()
		if err != nil {
			continue
		}
		jobID := job[1]
		paymentData, err := p.rdb.HGet(p.ctx, "payments", jobID).Result()
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
