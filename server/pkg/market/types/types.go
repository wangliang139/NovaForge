package types

import (
	"context"
	"sync"

	"github.com/wangliang139/NovaForge/server/pkg/internal/encrypt"
	ctypes "github.com/wangliang139/NovaForge/server/pkg/types"
)

type ConnectMode int

const (
	ConnectModePublic ConnectMode = iota
	ConnectModePrivate
)

type StreamHandle struct {
	C     <-chan *ctypes.Message
	Stop  func()
	ErrCh <-chan error
}

func BuildHandle(ctx context.Context, out chan *ctypes.Message, errCh chan error, stopC chan struct{}, doneC chan struct{}) *StreamHandle {
	ctx, cancel := context.WithCancel(ctx)
	var once sync.Once
	stop := func() {
		once.Do(func() {
			cancel()
			if stopC != nil {
				close(stopC)
			}
		})
	}
	go func() {
		// defer close(out)
		// defer close(errCh)
		select {
		case <-ctx.Done():
			stop()
		case <-doneC:
			stop()
		}
	}()
	return &StreamHandle{
		C:     out,
		Stop:  stop,
		ErrCh: errCh,
	}
}

type ApiAccount struct {
	ID            string
	Exchange      ctypes.Exchange
	ApiKey        string
	ApiSecret     string
	Passphrase    string
	AuthAlgorithm string

	IsSimulate bool

	DescryptFn func(raw string) (string, error)
}

func NewApiAccount(id string, exchange ctypes.Exchange, apiKey, apiSecret, passphrase, authAlgorithm string, isSimulate bool, descryptFn func(raw string) (string, error)) *ApiAccount {
	return &ApiAccount{
		ID:            id,
		Exchange:      exchange,
		ApiKey:        apiKey,
		ApiSecret:     apiSecret,
		Passphrase:    passphrase,
		AuthAlgorithm: authAlgorithm,
		IsSimulate:    isSimulate,
		DescryptFn:    descryptFn,
	}
}

func NewSecretApiAccount(id string, exchange ctypes.Exchange, apiKey, apiSecret, passphrase, authAlgorithm string) *ApiAccount {
	return NewApiAccount(id, exchange, apiKey, apiSecret, passphrase, authAlgorithm, false, encrypt.DecryptBase64)
}

func NewPlainApiAccount(id string, exchange ctypes.Exchange, apiKey, apiSecret, passphrase, authAlgorithm string) *ApiAccount {
	return NewApiAccount(id, exchange, apiKey, apiSecret, passphrase, authAlgorithm, false, nil)
}

func NewSimulateApiAccount(id string, exchange ctypes.Exchange) *ApiAccount {
	return NewApiAccount(id, exchange, "", "", "", "", true, nil)
}
