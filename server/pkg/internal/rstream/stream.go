package rstream

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog/log"
)

func Subscribe(ctx context.Context, client redis.UniversalClient, stream string, group string, consumer string) <-chan []byte {
	if err := client.XGroupCreateMkStream(ctx, stream, group, "$").Err(); err != nil {
		if !strings.Contains(err.Error(), "BUSYGROUP") {
			log.Err(err).Str("stream", stream).Str("group", group).Msg("failed to create redis consumer group")
		}
	}

	ch := make(chan []byte, 1024)
	go func() {
		defer func() {
			log.Info().Str("stream", stream).Str("group", group).Msg("stop consuming redis stream")
		}()
		defer close(ch)
		for {
			if ctx.Err() != nil {
				break
			}

			result, err := client.XReadGroup(ctx, &redis.XReadGroupArgs{
				Group:    group,
				Consumer: consumer,
				Streams:  []string{stream, ">"},
				Block:    5 * time.Second,
				Count:    20,
			}).Result()
			if err != nil {
				if errors.Is(err, context.Canceled) {
					break
				}
				if errors.Is(err, redis.Nil) {
					continue
				}
				log.Err(err).Str("consumer_id", consumer).Msg("redis XREADGROUP error")
				time.Sleep(1 * time.Second)
				continue
			}
			for _, xstream := range result {
				for _, msg := range xstream.Messages {
					msgID := msg.ID
					payloadVal, ok := msg.Values["payload"]
					if !ok {
						continue
					}
					var payload []byte
					switch v := payloadVal.(type) {
					case string:
						payload = []byte(v)
					case []byte:
						payload = v
					default:
						log.Warn().Interface("payload_type", payloadVal).Msg("redis stream payload type unsupported")
						continue
					}

					select {
					case ch <- payload:
					default:
						log.Warn().Msg("redis stream channel full, dropping message")
					}
					if ackErr := client.XAck(ctx, stream, group, msgID).Err(); ackErr != nil {
						log.Err(ackErr).Str("consumer_id", consumer).Str("message_id", msgID).Msg("failed to ack message")
					}
				}
			}
		}
	}()
	return ch
}
