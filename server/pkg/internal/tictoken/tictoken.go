package tictoken

import (
	"github.com/tiktoken-go/tokenizer"
)

var DefaultEncoding = tokenizer.O200kBase

func getTiktoken() (tokenizer.Codec, error) {
	codec, err := tokenizer.Get(DefaultEncoding)
	if err != nil {
		return nil, err
	}
	return codec, nil
}

func Count(content string) (int, error) {
	tke, err := getTiktoken()
	if err != nil {
		return 0, err
	}
	ids, _, err := tke.Encode(content)
	if err != nil {
		return 0, err
	}
	return len(ids), err
}

// Truncate Intercept text according to the number of tokens.
func Truncate(text string, num int) (string, error) {
	tke, err := getTiktoken()
	if err != nil {
		return "", err
	}
	ids, _, err := tke.Encode(text)
	if err != nil {
		return "", err
	}
	if len(ids) <= num {
		return text, nil
	}
	ids = ids[:num]
	return tke.Decode(ids)
}
