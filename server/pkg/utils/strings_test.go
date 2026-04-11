package utils

import (
	"testing"
)

func TestStrings_TruncateUTF8(t *testing.T) {
	tests := []struct {
		name  string
		text  string
		limit int
		want  string
	}{
		{
			name:  "test truncate utf8",
			text:  "hello world",
			limit: 5,
			want:  "hello",
		},
		{
			name:  "test truncate utf8 with chinese",
			text:  "我们都有一个家，名字叫中国",
			limit: 10,
			want:  "我们都有一个家，名字",
		},
		{
			name:  "test truncate utf8 with chinese and big limit",
			text:  "我们都有一个家，名字叫中国",
			limit: 20,
			want:  "我们都有一个家，名字叫中国",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Strings.TruncateUTF8(tt.text, tt.limit)
			if got != tt.want {
				t.Errorf("TruncateUTF8() = %v, want %v", got, tt.want)
			}
		})
	}
}
