package cronjob

import (
	"context"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
)

// Spec 描述一个进程内定时任务：按 Interval 触发，单次执行受 ExecuteTimeout 限制。
type Spec struct {
	Name           string
	Interval       time.Duration
	ExecuteTimeout time.Duration
	Run            func(ctx context.Context) error
}

// Run 在 ctx 取消时停止所有任务并等待 goroutine 退出。
func Run(ctx context.Context, jobs ...Spec) {
	var wg sync.WaitGroup
	for _, j := range jobs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ticker := time.NewTicker(j.Interval)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					runCtx, cancel := context.WithTimeout(ctx, j.ExecuteTimeout)
					err := j.Run(runCtx)
					cancel()
					if err != nil {
						log.Err(err).Str("job", j.Name).Msg("cron job failed")
					}
				}
			}
		}()
	}
	<-ctx.Done()
	wg.Wait()
}
