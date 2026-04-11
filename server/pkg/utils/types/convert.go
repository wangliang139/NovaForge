package types

import (
	"strconv"
)

func PIntToPInt64(i *int) *int64 {
	if i == nil {
		return nil
	}
	i64 := int64(*i)
	return &i64
}

func PInt64ToPInt(i *int64) *int {
	if i == nil {
		return nil
	}
	i32 := int(*i)
	return &i32
}

func PInt32ToPInt(i *int32) *int {
	if i == nil {
		return nil
	}
	i32 := int(*i)
	return &i32
}

func PIntToPInt32(i *int) *int32 {
	if i == nil {
		return nil
	}
	i32 := int32(*i)
	return &i32
}

func PInt64ToPString(i *int64) *string {
	if i == nil {
		return nil
	}
	s := strconv.FormatInt(*i, 10)
	return &s
}

func PStringToPInt64(s *string) (*int64, error) {
	if s == nil {
		return nil, nil
	}
	i, err := strconv.ParseInt(*s, 10, 64)
	if err != nil {
		return nil, err
	}
	return &i, nil
}

func StringToInt64(s string) (int64, error) {
	i, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, err
	}
	return i, nil
}
