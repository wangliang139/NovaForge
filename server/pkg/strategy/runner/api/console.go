package api

import (
	"strings"

	"github.com/rs/zerolog/log"
	"github.com/wangliang139/NovaForge/server/pkg/strategy/infra/logging"
	"rogchap.com/v8go"
)

// ConsoleAPI 控制台API
type ConsoleAPI struct {
	logger logging.Logger
}

// NewConsoleAPI 创建控制台API
func NewConsoleAPI(logger logging.Logger) *ConsoleAPI {
	return &ConsoleAPI{logger: logger}
}

func (c *ConsoleAPI) Debug(info *v8go.FunctionCallbackInfo) *v8go.Value {
	ctx := info.Context()
	args := info.Args()

	if len(args) > 0 {
		var msg strings.Builder
		msg.WriteString(args[0].DetailString())
		for i := 1; i < len(args); i++ {
			msg.WriteString(" ")
			msg.WriteString(args[i].DetailString())
		}
		if c.logger != nil {
			c.logger.Debugf("%s", msg.String())
		} else {
			log.Debug().Str("source", "strategy").Msg(msg.String())
		}
	} else {
		if c.logger != nil {
			c.logger.Debugf("%s", "")
		} else {
			log.Debug().Send()
		}
	}
	return v8go.Undefined(ctx.Isolate())
}

func (c *ConsoleAPI) Log(info *v8go.FunctionCallbackInfo) *v8go.Value {
	ctx := info.Context()
	args := info.Args()

	if len(args) > 0 {
		var msg strings.Builder
		msg.WriteString(args[0].DetailString())
		for i := 1; i < len(args); i++ {
			msg.WriteString(" ")
			msg.WriteString(args[i].DetailString())
		}
		if c.logger != nil {
			c.logger.Infof("%s", msg.String())
		} else {
			log.Info().Str("source", "strategy").Msg(msg.String())
		}
	} else {
		if c.logger != nil {
			c.logger.Infof("%s", "")
		} else {
			log.Info().Send()
		}
	}

	return v8go.Undefined(ctx.Isolate())
}

func (c *ConsoleAPI) Error(info *v8go.FunctionCallbackInfo) *v8go.Value {
	ctx := info.Context()
	args := info.Args()

	if len(args) > 0 {
		var msg strings.Builder
		msg.WriteString(args[0].DetailString())
		for i := 1; i < len(args); i++ {
			msg.WriteString(" ")
			msg.WriteString(args[i].DetailString())
		}
		if c.logger != nil {
			c.logger.Errorf("%s", msg.String())
		} else {
			log.Error().Str("source", "strategy").Msg(msg.String())
		}
	} else {
		if c.logger != nil {
			c.logger.Errorf("%s", "")
		} else {
			log.Error().Send()
		}
	}

	return v8go.Undefined(ctx.Isolate())
}

func (c *ConsoleAPI) Warn(info *v8go.FunctionCallbackInfo) *v8go.Value {
	ctx := info.Context()
	args := info.Args()

	if len(args) > 0 {
		var msg strings.Builder
		msg.WriteString(args[0].DetailString())
		for i := 1; i < len(args); i++ {
			msg.WriteString(" ")
			msg.WriteString(args[i].DetailString())
		}
		if c.logger != nil {
			c.logger.Warnf("%s", msg.String())
		} else {
			log.Warn().Str("source", "strategy").Msg(msg.String())
		}
	} else {
		if c.logger != nil {
			c.logger.Warnf("%s", "")
		} else {
			log.Warn().Send()
		}
	}

	return v8go.Undefined(ctx.Isolate())
}
