package encrypt

import (
	"encoding/base64"

	"github.com/kelseyhightower/envconfig"
	"github.com/wangliang139/NovaForge/server/pkg/utils/encdec"
	"github.com/wangliang139/mow/errors"
)

var _cfg config

type config struct {
	DecryptKeyBase64 string `split_words:"true" required:"true" envconfig:"APP_DECRYPT_KEY_BASE64" default:"foo"`
}

func init() {
	var cfg config
	envconfig.MustProcess("ENCRYPT", &cfg)
	_cfg = cfg
}

func Encrypt(plaintext []byte) ([]byte, error) {
	if _cfg.DecryptKeyBase64 == "" {
		return nil, errors.New(errors.InvalidArgument, "decrypt key is empty")
	}
	decryptKey, err := base64.StdEncoding.DecodeString(_cfg.DecryptKeyBase64)
	if err != nil {
		return nil, err
	}
	return encdec.RsaEncrypt(plaintext, string(decryptKey))
}

func Decrypt(ciphertext []byte) ([]byte, error) {
	if _cfg.DecryptKeyBase64 == "" {
		return nil, errors.New(errors.InvalidArgument, "decrypt key is empty")
	}
	decryptKey, err := base64.StdEncoding.DecodeString(_cfg.DecryptKeyBase64)
	if err != nil {
		return nil, err
	}
	return encdec.RsaDecrypt(ciphertext, string(decryptKey))
}

func EncryptString(plaintext string) (string, error) {
	bytes, err := Encrypt([]byte(plaintext))
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

func DecryptString(ciphertext string) (string, error) {
	bytes, err := Decrypt([]byte(ciphertext))
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

func DecryptBase64(base64Ciphertext string) (string, error) {
	bts, err := base64.StdEncoding.DecodeString(base64Ciphertext)
	if err != nil {
		return "", err
	}
	bytes, err := Decrypt(bts)
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

func EncryptBase64(plaintext string) (string, error) {
	bytes, err := Encrypt([]byte(plaintext))
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(bytes), nil
}
