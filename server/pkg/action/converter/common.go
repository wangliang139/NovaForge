package converter

import (
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"
)

func ConvertSecondPtrToTime(i *int64) *time.Time {
	if i == nil {
		return nil
	}
	t := time.Unix(*i, 0)
	return &t
}

func ConvertSecondInt64PtrToTimestampPb(i *int64) *timestamppb.Timestamp {
	if i == nil {
		return nil
	}
	return &timestamppb.Timestamp{
		Seconds: *i,
		Nanos:   int32(0),
	}
}

func ConvertSecondIntPtrToTimestampPb(i *int) *timestamppb.Timestamp {
	if i == nil {
		return nil
	}
	return &timestamppb.Timestamp{
		Seconds: int64(*i),
		Nanos:   int32(0),
	}
}