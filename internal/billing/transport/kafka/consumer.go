package kafka

import (
	"context"
	"log/slog"
	"time"

	"appointment-service/internal/billing/application"

	kafkago "github.com/segmentio/kafka-go"
)

type Consumer struct {
	reader    *kafkago.Reader
	processor *application.Processor
	logger    *slog.Logger
}

func NewConsumer(brokers []string, topic string, groupID string, processor *application.Processor, logger *slog.Logger) *Consumer {
	reader := kafkago.NewReader(kafkago.ReaderConfig{
		Brokers:        brokers,
		GroupID:        groupID,
		Topic:          topic,
		MinBytes:       1,
		MaxBytes:       10e6,
		CommitInterval: 0,
		MaxWait:        1 * time.Second,
	})

	return &Consumer{reader: reader, processor: processor, logger: logger}
}

func (c *Consumer) Run(ctx context.Context) error {
	for {
		if ctx.Err() != nil {
			return nil
		}

		msg, err := c.reader.FetchMessage(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			c.logger.Error("kafka fetch failed", "error", err)
			time.Sleep(500 * time.Millisecond)
			continue
		}

		if err := c.processor.Process(ctx, msg.Value); err != nil {
			c.logger.Error("billing process failed", "topic", msg.Topic, "partition", msg.Partition, "offset", msg.Offset, "error", err)
			time.Sleep(500 * time.Millisecond)
			continue
		}

		if err := c.reader.CommitMessages(ctx, msg); err != nil {
			c.logger.Error("kafka commit failed", "offset", msg.Offset, "error", err)
			time.Sleep(500 * time.Millisecond)
			continue
		}
	}
}

func (c *Consumer) Close() error {
	if c.reader == nil {
		return nil
	}
	return c.reader.Close()
}
