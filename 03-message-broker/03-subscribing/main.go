package main

import (
	"context"
	"fmt"
	"os"

	"github.com/ThreeDotsLabs/watermill"
	"github.com/ThreeDotsLabs/watermill-redisstream/pkg/redisstream"
	"github.com/redis/go-redis/v9"
)

func main() {
	logger := watermill.NewStdLogger(false, false)

	rdb := redis.NewClient(&redis.Options{
		Addr: os.Getenv("REDIS_ADDR"),
	})

	subscriber, err := redisstream.NewSubscriber(redisstream.SubscriberConfig{
		Client: rdb,
	}, logger)
	if err != nil {
		panic(err)
	}

	messages, err := subscriber.Subscribe(context.Background(), "progress")
	if err != nil {
		panic(err)
	}

	for msg := range messages {
		p := string(msg.Payload)
		id := msg.UUID
		fmt.Printf("Message ID: %v - %v%", id, p)
		msg.Ack()
	}
}
