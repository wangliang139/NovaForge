package settings

import (
	"context"
	"strings"

	"github.com/wangliang139/NovaForge/server/pkg/repos"
	"github.com/wangliang139/NovaForge/server/pkg/repos/kv"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// MaxUserSettingsKeysPerGet GetSettings 单次请求的 key 数量上限。
const MaxUserSettingsKeysPerGet = 50

var kvRepo *kv.Queries

func Init(db *repos.Entity) {
	kvRepo = db.KvRepo
}

// Set 写入单条 kv
func Set(ctx context.Context, key, value string) error {
	k := strings.TrimSpace(key)
	if err := VerifyKey(k); err != nil {
		return status.Error(codes.InvalidArgument, err.Error())
	}
	storeKey := k
	_, err := kvRepo.Upsert(ctx, kv.UpsertParams{Key: k, Value: value}, &storeKey)
	return err
}

func Get(ctx context.Context, key string) (string, bool) {
	row, err := kvRepo.GetByKey(ctx, key)
	if err != nil || row == nil {
		return "", false
	}
	return row.Value, true
}

func GetSettings(ctx context.Context, keys []string) ([]SettingEntry, error) {
	if len(keys) > MaxUserSettingsKeysPerGet {
		return nil, status.Errorf(codes.InvalidArgument, "at most %d keys per request", MaxUserSettingsKeysPerGet)
	}
	out := make([]SettingEntry, 0, len(keys))
	for _, raw := range keys {
		k := strings.TrimSpace(raw)
		if err := VerifyKey(k); err != nil {
			return nil, status.Error(codes.InvalidArgument, err.Error())
		}
		row, err := kvRepo.GetByKey(ctx, k)
		if err != nil {
			return nil, err
		}
		val := ""
		if row != nil {
			val = row.Value
		}
		out = append(out, SettingEntry{Key: k, Value: val})
	}
	return out, nil
}
