module github.com/wangliang139/NovaForge/server

go 1.25.8

replace google.golang.org/genproto => google.golang.org/genproto v0.0.0-20240401170217-c3f982113cda

replace github.com/adshao/go-binance/v2 => github.com/wangliang139/go-binance/v2 v2.8.17

require (
	github.com/99designs/gqlgen v0.17.88
	github.com/ClickHouse/clickhouse-go/v2 v2.43.0
	github.com/adshao/go-binance/v2 v2.8.11
	github.com/antchfx/htmlquery v1.3.6
	github.com/antchfx/xmlquery v1.5.0
	github.com/bitly/go-simplejson v0.5.1
	github.com/bsm/redislock v0.9.4
	github.com/bytedance/sonic v1.15.0
	github.com/gin-contrib/cors v1.7.6
	github.com/gin-gonic/gin v1.12.0
	github.com/golang-jwt/jwt/v5 v5.3.1
	github.com/google/uuid v1.6.0
	github.com/gorilla/websocket v1.5.4-0.20250319132907-e064f32e3674
	github.com/invopop/jsonschema v0.13.0
	github.com/jackc/pgx/v5 v5.8.0
	github.com/kelseyhightower/envconfig v1.4.0
	github.com/microcosm-cc/bluemonday v1.0.27
	github.com/modelcontextprotocol/go-sdk v1.4.1
	github.com/mymmrac/telego v1.7.0
	github.com/openai/openai-go/v3 v3.29.0
	github.com/patrickmn/go-cache v2.1.0+incompatible
	github.com/pemistahl/lingua-go v1.4.0
	github.com/redis/go-redis/v9 v9.18.0
	github.com/rs/zerolog v1.34.0
	github.com/samber/lo v1.53.0
	github.com/shopspring/decimal v1.4.0
	github.com/stumble/dcache v0.4.0
	github.com/stumble/wpgx v0.4.8
	github.com/tiktoken-go/tokenizer v0.7.0
	github.com/vektah/gqlparser/v2 v2.5.32
	github.com/wangliang139/mow/env v0.0.0-20260225012344-ae8a98a539fb
	github.com/wangliang139/mow/errors v0.0.0-20260225012344-ae8a98a539fb
	github.com/wangliang139/mow/executors v0.0.0-20260225012344-ae8a98a539fb
	github.com/wangliang139/mow/ginx v0.0.0-20260225012344-ae8a98a539fb
	github.com/wangliang139/mow/health v0.0.0-20260225012344-ae8a98a539fb
	github.com/wangliang139/mow/logger v0.0.0-20260225012344-ae8a98a539fb
	github.com/wangliang139/mow/number v0.0.0-20260225012344-ae8a98a539fb
	github.com/wangliang139/mow/otel v0.0.0-20260225012344-ae8a98a539fb
	github.com/wangliang139/mow/snowflake v0.0.0-20260225012344-ae8a98a539fb
	github.com/wangliang139/okx-connector-go v0.0.3-0.20260304090626-c3f702c0b128
	github.com/yuin/goldmark v1.7.17
	go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin v0.67.0
	go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.67.0
	go.opentelemetry.io/otel v1.42.0
	go.uber.org/ratelimit v0.3.1
	golang.org/x/net v0.52.0
	golang.org/x/sync v0.20.0
	golang.org/x/text v0.35.0
	golang.org/x/time v0.15.0
	google.golang.org/grpc v1.79.3
	google.golang.org/protobuf v1.36.11
)

require (
	github.com/google/jsonschema-go v0.4.2 // indirect
	github.com/joho/godotenv v1.5.1 // indirect
	github.com/miekg/dns v1.1.62 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/redis/go-redis/extra/rediscmd/v9 v9.7.0 // indirect
	github.com/segmentio/encoding v0.5.4 // indirect
	github.com/wangliang139/mow/dns v0.0.0-20241112041056-673cdf316618 // indirect
	github.com/wangliang139/mow/otel/instrumentation/redis v0.0.0-20241112043448-2351af87391b // indirect
	github.com/yosida95/uritemplate/v3 v3.0.2 // indirect
	golang.org/x/oauth2 v0.35.0 // indirect
)

require (
	github.com/ClickHouse/ch-go v0.71.0 // indirect
	github.com/agnivade/levenshtein v1.2.1 // indirect
	github.com/amarnathcjd/gogram v1.7.2
	github.com/andybalholm/brotli v1.2.0 // indirect
	github.com/antchfx/xpath v1.3.6 // indirect
	github.com/aymerick/douceur v0.2.0 // indirect
	github.com/bahlo/generic-list-go v0.2.0 // indirect
	github.com/benbjohnson/clock v1.3.0 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/buger/jsonparser v1.1.1 // indirect
	github.com/bwmarrin/snowflake v0.3.0 // indirect
	github.com/bytedance/gopkg v0.1.3 // indirect
	github.com/bytedance/sonic/loader v0.5.0 // indirect
	github.com/cenkalti/backoff/v4 v4.3.0 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/cloudwego/base64x v0.1.6 // indirect
	github.com/coocood/freecache v1.2.4 // indirect
	github.com/dgryski/go-rendezvous v0.0.0-20200823014737-9f7001d12a5f // indirect
	github.com/dlclark/regexp2 v1.11.5 // indirect
	github.com/felixge/httpsnoop v1.0.4 // indirect
	github.com/gabriel-vasile/mimetype v1.4.13 // indirect
	github.com/gin-contrib/sse v1.1.0 // indirect
	github.com/go-faster/city v1.0.1 // indirect
	github.com/go-faster/errors v0.7.1 // indirect
	github.com/go-logr/logr v1.4.3 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/go-playground/locales v0.14.1 // indirect
	github.com/go-playground/universal-translator v0.18.1 // indirect
	github.com/go-playground/validator/v10 v10.30.1 // indirect
	github.com/go-viper/mapstructure/v2 v2.5.0 // indirect
	github.com/goccy/go-json v0.10.5 // indirect
	github.com/goccy/go-yaml v1.19.2 // indirect
	github.com/golang/groupcache v0.0.0-20210331224755-41bb18bfe9da // indirect
	github.com/gorilla/css v1.0.1 // indirect
	github.com/grbit/go-json v0.11.0 // indirect
	github.com/grpc-ecosystem/grpc-gateway/v2 v2.28.0 // indirect
	github.com/hashicorp/golang-lru/v2 v2.0.7 // indirect
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761 // indirect
	github.com/jackc/puddle/v2 v2.2.2 // indirect
	github.com/jpillora/backoff v1.0.0 // indirect
	github.com/json-iterator/go v1.1.12
	github.com/klauspost/compress v1.18.4 // indirect
	github.com/klauspost/cpuid/v2 v2.3.0 // indirect
	github.com/leodido/go-urn v1.4.0 // indirect
	github.com/mailru/easyjson v0.9.1 // indirect
	github.com/mattn/go-colorable v0.1.14 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.3-0.20250322232337-35a7c28c31ee // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/paulmach/orb v0.12.0 // indirect
	github.com/pelletier/go-toml/v2 v2.2.4 // indirect
	github.com/pierrec/lz4/v4 v4.1.25 // indirect
	github.com/prometheus/client_golang v1.23.2 // indirect
	github.com/prometheus/client_model v0.6.2 // indirect
	github.com/prometheus/common v0.66.1 // indirect
	github.com/prometheus/procfs v0.19.2 // indirect
	github.com/quic-go/qpack v0.6.0 // indirect
	github.com/quic-go/quic-go v0.59.0 // indirect
	github.com/satori/go.uuid v1.2.0 // indirect
	github.com/segmentio/asm v1.2.1 // indirect
	github.com/sosodev/duration v1.4.0 // indirect
	github.com/tidwall/gjson v1.18.0 // indirect
	github.com/tidwall/match v1.1.1 // indirect
	github.com/tidwall/pretty v1.2.1 // indirect
	github.com/tidwall/sjson v1.2.5 // indirect
	github.com/twitchyliquid64/golang-asm v0.15.1 // indirect
	github.com/ugorji/go/codec v1.3.1 // indirect
	github.com/urfave/cli/v3 v3.7.0 // indirect
	github.com/valyala/bytebufferpool v1.0.0 // indirect
	github.com/valyala/fasthttp v1.69.0 // indirect
	github.com/valyala/fastjson v1.6.10 // indirect
	github.com/vmihailenco/msgpack/v5 v5.4.1 // indirect
	github.com/vmihailenco/tagparser/v2 v2.0.0 // indirect
	github.com/wangliang139/mow/database/cache v0.0.0-20260225012344-ae8a98a539fb
	github.com/wangliang139/mow/database/wpgx v0.0.0-20260225012344-ae8a98a539fb
	github.com/wangliang139/mow/otel/instrumentation/zerolog v0.0.0-20241112043521-479ea61af57e // indirect
	github.com/wk8/go-ordered-map/v2 v2.1.8 // indirect
	go.mongodb.org/mongo-driver/v2 v2.5.0 // indirect
	go.opentelemetry.io/auto/sdk v1.2.1 // indirect
	go.opentelemetry.io/contrib/instrumentation/runtime v0.57.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp v0.8.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp v1.32.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlptrace v1.42.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp v1.32.0 // indirect
	go.opentelemetry.io/otel/exporters/stdout/stdouttrace v1.42.0 // indirect
	go.opentelemetry.io/otel/log v0.14.0 // indirect
	go.opentelemetry.io/otel/metric v1.42.0
	go.opentelemetry.io/otel/sdk v1.42.0 // indirect
	go.opentelemetry.io/otel/sdk/log v0.14.0 // indirect
	go.opentelemetry.io/otel/sdk/log/logtest v0.15.0 // indirect
	go.opentelemetry.io/otel/sdk/metric v1.42.0 // indirect
	go.opentelemetry.io/otel/trace v1.42.0
	go.opentelemetry.io/proto/otlp v1.10.0 // indirect
	go.uber.org/atomic v1.11.0 // indirect
	go.yaml.in/yaml/v2 v2.4.3 // indirect
	go.yaml.in/yaml/v3 v3.0.4 // indirect
	golang.org/x/arch v0.24.0 // indirect
	golang.org/x/crypto v0.49.0
	golang.org/x/exp v0.0.0-20260218203240-3dfff04db8fa // indirect
	golang.org/x/mod v0.33.0 // indirect
	golang.org/x/sys v0.42.0 // indirect
	golang.org/x/tools v0.42.0 // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20260217215200-42d3e9bedb6d // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260311181403-84a4fc48630c // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
	rogchap.com/v8go v0.9.0
)
