package gateio

import (
	"context"
	"testing"
)

func Test_GetFutureList(t *testing.T) {
	client := NewClient("")
	resp, err := client.GetFutureList(
		context.Background(),
		1,
		10,
		1719849600,
		1736169600,
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Log(resp)
}
