package chsdk

import (
	"context"
	"fmt"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/kelseyhightower/envconfig"
	"github.com/rs/zerolog/log"
)

type Config struct {
	Addr     string `default:"127.0.0.1:9000"`
	Database string `default:"trade"`
	Username string
	Password string
}

func (c *Config) String() string {
	if c == nil {
		return "nil"
	}
	tmp := *c
	if len(tmp.Password) > 0 {
		tmp.Password = "*****"
	}
	return fmt.Sprintf("%+v", tmp)
}

type Client struct {
	driver.Conn

	Conf Config
}

func Connect(ctx context.Context) (*Client, error) {
	var conf Config
	envconfig.MustProcess("CH", &conf)
	log.Info().Msgf("clickhouse config: %+v", &conf)

	conn, err := clickhouse.Open(&clickhouse.Options{
		Addr: []string{conf.Addr},
		Auth: clickhouse.Auth{
			Database: conf.Database,
			Username: conf.Username,
			Password: conf.Password,
		},
		ClientInfo: clickhouse.ClientInfo{
			Products: []struct {
				Name    string
				Version string
			}{
				{Name: "novaforge", Version: "0.1"},
			},
		},
		Compression: &clickhouse.Compression{
			Method: clickhouse.CompressionZSTD,
		},
	})
	if err != nil {
		return nil, err
	}

	if err := conn.Ping(ctx); err != nil {
		return nil, err
	}
	return &Client{
		Conf: conf,
		Conn: conn,
	}, nil
}
