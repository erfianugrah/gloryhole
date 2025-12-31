// Package dns contains the query logger worker pool for async query logging
package dns

import (
	"context"
	"sync"
	"sync/atomic"

	"glory-hole/pkg/logging"
	"glory-hole/pkg/storage"
)

// QueryLogger manages a worker pool for asynchronous query logging
// This prevents spawning a new goroutine for every DNS query
type QueryLogger struct {
	logCh      chan *storage.QueryLog
	workers    int
	wg         sync.WaitGroup
	ctx        context.Context
	cancel     context.CancelFunc
	storage    storage.Storage
	logger     *logging.Logger
	dropped    atomic.Uint64
	buffered   atomic.Uint64
	closeOnce  sync.Once
}

// NewQueryLogger creates a new query logger with a fixed worker pool
func NewQueryLogger(stor storage.Storage, logger *logging.Logger, bufferSize, workers int) *QueryLogger {
	ctx, cancel := context.WithCancel(context.Background())

	ql := &QueryLogger{
		logCh:   make(chan *storage.QueryLog, bufferSize),
		workers: workers,
		ctx:     ctx,
		cancel:  cancel,
		storage: stor,
		logger:  logger,
	}

	// Start worker pool
	for i := 0; i < workers; i++ {
		ql.wg.Add(1)
		go ql.worker(i)
	}

	if logger != nil {
		logger.Info("Query logger worker pool started",
			"workers", workers,
			"buffer_size", bufferSize)
	}

	return ql
}

// worker processes query log entries from the channel
func (ql *QueryLogger) worker(id int) {
	defer ql.wg.Done()

	for {
		select {
		case <-ql.ctx.Done():
			// Drain remaining entries before exiting
			ql.drainChannel()
			return

		case entry, ok := <-ql.logCh:
			if !ok {
				return
			}

			// Decrement buffered counter
			ql.buffered.Add(^uint64(0)) // Atomic decrement

			// Create context with timeout for logging
			logCtx, cancel := context.WithTimeout(ql.ctx, storage.DefaultLogTimeout)

			if err := ql.storage.LogQuery(logCtx, entry); err != nil {
				if ql.logger != nil {
					ql.logger.Error("Failed to log query",
						"worker", id,
						"domain", entry.Domain,
						"client_ip", entry.ClientIP,
						"error", err)
				}
			}

			cancel()
		}
	}
}

// drainChannel attempts to process remaining entries in the channel during shutdown
func (ql *QueryLogger) drainChannel() {
	for {
		select {
		case entry, ok := <-ql.logCh:
			if !ok {
				return
			}

			ql.buffered.Add(^uint64(0)) // Atomic decrement

			// Use background context since main context is canceled
			logCtx, cancel := context.WithTimeout(context.Background(), storage.DefaultLogTimeout)

			if err := ql.storage.LogQuery(logCtx, entry); err != nil {
				if ql.logger != nil {
					ql.logger.Error("Failed to log query during shutdown",
						"domain", entry.Domain,
						"error", err)
				}
			}

			cancel()

		default:
			// Channel empty
			return
		}
	}
}

// LogAsync queues a query log entry for async processing
// Returns error if buffer is full (non-blocking)
func (ql *QueryLogger) LogAsync(entry *storage.QueryLog) error {
	select {
	case ql.logCh <- entry:
		ql.buffered.Add(1)
		return nil
	default:
		// Buffer full - drop query and increment counter
		ql.dropped.Add(1)

		if ql.logger != nil {
			ql.logger.Warn("Query log buffer full, dropping entry",
				"domain", entry.Domain,
				"client_ip", entry.ClientIP,
				"dropped_total", ql.dropped.Load())
		}

		return storage.ErrBufferFull
	}
}

// Close gracefully shuts down the query logger worker pool
// Waits for all workers to finish processing remaining entries
// Safe to call multiple times (uses sync.Once)
func (ql *QueryLogger) Close() error {
	var closeErr error

	ql.closeOnce.Do(func() {
		if ql.logger != nil {
			buffered := ql.buffered.Load()
			dropped := ql.dropped.Load()

			ql.logger.Info("Shutting down query logger",
				"buffered_entries", buffered,
				"dropped_total", dropped)
		}

		// Signal workers to stop
		ql.cancel()

		// Wait for all workers to finish
		ql.wg.Wait()

		// Close channel
		close(ql.logCh)

		if ql.logger != nil {
			ql.logger.Info("Query logger shutdown complete")
		}
	})

	return closeErr
}

// Stats returns query logger statistics
func (ql *QueryLogger) Stats() (buffered, dropped uint64) {
	return ql.buffered.Load(), ql.dropped.Load()
}

// BufferSize returns the current number of buffered entries
func (ql *QueryLogger) BufferSize() int {
	return len(ql.logCh)
}

// BufferCapacity returns the maximum buffer capacity
func (ql *QueryLogger) BufferCapacity() int {
	return cap(ql.logCh)
}
