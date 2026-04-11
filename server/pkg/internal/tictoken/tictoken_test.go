package tictoken

import (
	"testing"
)

func TestTictokenByString(t *testing.T) {
	type args struct {
		modelCode string
		content   string
	}
	tests := []struct {
		name    string
		args    args
		want    int
		wantErr bool
	}{
		{
			name: "test gpt3.5 turbo",
			args: args{
				content: "He said one day you'll leave this world behind, so live a life you will remember.",
			},
			want:    18,
			wantErr: false,
		},
		{
			name: "test gpt4",
			args: args{
				content: "He said one day you'll leave this world behind, so live a life you will remember.",
			},
			want:    18,
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Count(tt.args.content)
			if (err != nil) != tt.wantErr {
				t.Errorf("TictokenByString() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("TictokenByString() = %v, want %v", got, tt.want)
			}
		})
	}
}
