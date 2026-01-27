package main

import (
	"context"
	"time"

	"github.com/IBM/sarama"
)

type KafkaHeader struct {
	raw []sarama.RecordHeader
}

func (k *KafkaHeader) Set(key, value string) {
	k.raw = append(k.raw, sarama.RecordHeader{Key: []byte(key), Value: []byte(value)})
}

func (k *KafkaHeader) Get(key string) string {
	for _, h := range k.raw {
		if string(h.Key) == key {
			return string(h.Value)
		}
	}
	return ""
}

func (k *KafkaHeader) Keys() []string {
	keys := make([]string, len(k.raw))
	for i, h := range k.raw {
		keys[i] = string(h.Key)
	}
	return keys
}

type KafkaBegin struct {
	Topic      string
	IsProducer bool
	BeginTime  time.Time
}

type KafkaEnd struct {
	Topic      string
	Error      error
	EndTime    time.Time
	BeginTime  time.Time
	IsProducer bool
}

type kafkaHandler interface {
	KafkaBegin(ctx context.Context, info *KafkaBegin) context.Context
	KafkaEnd(ctx context.Context, info *KafkaEnd)
}
