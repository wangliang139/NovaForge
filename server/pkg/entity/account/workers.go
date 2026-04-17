package account

import (
	"context"
	"hash/fnv"
	"strings"

	ctypes "github.com/wangliang139/NovaForge/server/pkg/types"
	"github.com/wangliang139/mow/logger"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

const accountMsgWorkerQueueCap = 512

type accountRawJob struct {
	ctx        context.Context
	span       trace.Span
	consumerID string
	envelope   *ctypes.Envelope
}

func accountMessageShardIndex(accountID string, shards int) int {
	if shards <= 1 {
		return 0
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte(accountID))
	return int(h.Sum32() % uint32(shards))
}

func (e *Entity) startAccountMessageWorkers() {
	e.accountMsgWorkersOnce.Do(func() {
		n := e.cfg.AccountMessageShardCount
		if n < 1 {
			n = 1
		}
		if n > 256 {
			n = 256
		}
		e.accountMsgCh = make([]chan accountRawJob, n)
		for i := 0; i < n; i++ {
			e.accountMsgCh[i] = make(chan accountRawJob, accountMsgWorkerQueueCap)
			go e.accountMsgWorkerLoop(i, e.accountMsgCh[i])
		}
	})
}

func (e *Entity) accountMsgWorkerLoop(shard int, ch <-chan accountRawJob) {
	for {
		select {
		case <-e.ctx.Done():
			return
		case job, ok := <-ch:
			if !ok {
				return
			}
			e.runAccountRawJob(shard, job)
		}
	}
}

func (e *Entity) runAccountRawJob(shard int, job accountRawJob) {
	defer func() {
		if job.span != nil {
			job.span.End()
		}
	}()
	if job.envelope == nil {
		return
	}
	env := job.envelope
	if job.span != nil {
		job.span.SetAttributes(attribute.String("exchange", env.Exchange))
		if env.Account != nil {
			job.span.SetAttributes(attribute.String("account", *env.Account))
		}
		if env.Symbol != nil {
			job.span.SetAttributes(attribute.String("symbol", *env.Symbol))
		}
		job.span.SetAttributes(
			attribute.String("stream", env.Stream.String()),
			attribute.Int("account_msg_shard", shard),
		)
	}

	err := e.handleAccountMessage(job.ctx, env)
	if err != nil {
		if job.span != nil {
			job.span.SetStatus(codes.Error, err.Error())
		}
		logger.Ctx(job.ctx).Err(err).Str("consumer_id", job.consumerID).Int("shard", shard).
			Msg("Failed to process account message")
		return
	}
	if job.span != nil {
		job.span.SetStatus(codes.Ok, "success")
	}
}

// enqueueAccountRawJob 将单条 account_raw 投递到按账户 id 固定的分片队列（阻塞直至入队或 ctx 取消）。
func (e *Entity) enqueueAccountRawJob(ctx context.Context, span trace.Span, consumerID string, envelope ctypes.Envelope) error {
	accountID := ""
	if envelope.Account != nil {
		accountID = strings.TrimSpace(*envelope.Account)
	}
	if accountID == "" {
		if span != nil {
			span.SetStatus(codes.Error, "account id is required")
			span.End()
		}
		return nil
	}

	shards := len(e.accountMsgCh)
	if shards == 0 {
		if span != nil {
			span.SetStatus(codes.Error, "account message workers not started")
			span.End()
		}
		return nil
	}
	shard := accountMessageShardIndex(accountID, shards)

	envCopy := envelope
	envPtr := &envCopy
	job := accountRawJob{
		ctx:        ctx,
		span:       span,
		consumerID: consumerID,
		envelope:   envPtr,
	}

	select {
	case e.accountMsgCh[shard] <- job:
		return nil
	case <-e.ctx.Done():
		return e.ctx.Err()
	case <-ctx.Done():
		return ctx.Err()
	}
}
