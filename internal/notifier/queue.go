package notifier

import (
	"context"
	"fmt"
	"sync"
	"time"

	"django/internal/logger"
)

// NotificationQueue manages rate-limited message dispatching to Telegram.
type NotificationQueue struct {
	client            *TelegramClient
	msgChan           chan string
	rateLimitInterval time.Duration
	ctx               context.Context
	cancel            context.CancelFunc
	wg                sync.WaitGroup
	enabled           bool
}

// NewNotificationQueue initializes a message queue rate limiter.
func NewNotificationQueue(client *TelegramClient, bufferSize int, rateLimitInterval time.Duration) *NotificationQueue {
	if bufferSize <= 0 {
		bufferSize = 100
	}
	if rateLimitInterval <= 0 {
		rateLimitInterval = 500 * time.Millisecond // 2 messages per second default
	}

	enabled := client != nil && client.IsEnabled()

	ctx, cancel := context.WithCancel(context.Background())

	return &NotificationQueue{
		client:            client,
		msgChan:           make(chan string, bufferSize),
		rateLimitInterval: rateLimitInterval,
		ctx:               ctx,
		cancel:            cancel,
		enabled:           enabled,
	}
}

// IsEnabled checks if queue processing is active.
func (nq *NotificationQueue) IsEnabled() bool {
	return nq != nil && nq.enabled
}

// Start launches the background worker loop that processes queued messages.
func (nq *NotificationQueue) Start() {
	if nq == nil || !nq.enabled {
		return
	}

	nq.wg.Add(1)
	go nq.worker()
}

// worker is the background consumer loop sending rate-limited messages.
func (nq *NotificationQueue) worker() {
	defer nq.wg.Done()

	ticker := time.NewTicker(nq.rateLimitInterval)
	defer ticker.Stop()

	for {
		select {
		case <-nq.ctx.Done():
			// Drain remaining messages before exiting
			nq.drainQueue()
			return
		case msg, ok := <-nq.msgChan:
			if !ok {
				nq.drainQueue()
				return
			}
			nq.sendMessageWithRecovery(msg)
			select {
			case <-ticker.C:
			case <-nq.ctx.Done():
				nq.drainQueue()
				return
			}
		}
	}
}

// sendMessageWithRecovery executes client.SendMessage with panic recovery.
func (nq *NotificationQueue) sendMessageWithRecovery(msg string) {
	defer func() {
		if r := recover(); r != nil {
			logger.Error("Notifier", fmt.Sprintf("Panic recovered in notification queue worker: %v", r))
		}
	}()

	sendCtx, sendCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer sendCancel()

	if err := nq.client.SendMessage(sendCtx, msg); err != nil {
		logger.Warn("Notifier", "Failed to send Telegram notification", logger.Err(err))
	}
}

// drainQueue sends remaining buffered messages on shutdown.
func (nq *NotificationQueue) drainQueue() {
	for {
		select {
		case msg, ok := <-nq.msgChan:
			if !ok {
				return
			}
			nq.sendMessageWithRecovery(msg)
			time.Sleep(nq.rateLimitInterval)
		default:
			return
		}
	}
}

// Enqueue adds a message to the notification queue.
func (nq *NotificationQueue) Enqueue(msg string) bool {
	if nq == nil || !nq.enabled || msg == "" {
		return false
	}

	select {
	case <-nq.ctx.Done():
		return false
	case nq.msgChan <- msg:
		return true
	default:
		logger.Warn("Notifier", fmt.Sprintf("Warning: Notification queue full (%d msgs), dropping message", cap(nq.msgChan)))
		return false
	}
}

// Stop gracefully stops the queue worker and drains any pending messages.
func (nq *NotificationQueue) Stop() {
	if nq == nil || !nq.enabled {
		return
	}

	nq.cancel()
	nq.wg.Wait()
}
