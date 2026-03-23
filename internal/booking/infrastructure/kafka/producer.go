package kafka

import (
	"context"
	"fmt"
	"time"

	kafkago "github.com/segmentio/kafka-go"
)

type Producer struct {
	writer *kafkago.Writer
}

func NewProducer(brokers []string) *Producer {
	return &Producer{
		writer: &kafkago.Writer{
			Addr:                   kafkago.TCP(brokers...),
			AllowAutoTopicCreation: false,
			RequiredAcks:           kafkago.RequireOne,
			BatchTimeout:           10 * time.Millisecond,
			Async:                  false,
		},
	}
}

func (p *Producer) Publish(ctx context.Context, topic string, key string, value []byte) error {
	err := p.writer.WriteMessages(ctx, kafkago.Message{Topic: topic, Key: []byte(key), Value: value, Time: time.Now().UTC()})
	if err != nil {
		return fmt.Errorf("kafka publish failed: %w", err)
	}
	return nil
}

func (p *Producer) Close() error {
	return p.writer.Close()
}
