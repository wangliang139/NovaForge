package gateio

type Config struct {
	ApiHost string `split_words:"true" envconfig:"GATEIO_API_HOST" default:"https://www.gate.com"`
}
