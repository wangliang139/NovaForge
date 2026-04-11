package usersvc

import "testing"

func TestParseAPIKeyRaw(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		raw       string
		wantOK    bool
		wantLen   int
		wantStart string
	}{
		{
			name:      "accepts current 32-char lookup",
			raw:       "nf_0123456789abcdef0123456789abcdef_abcdefghijklmnopqrstuvwxyz123456",
			wantOK:    true,
			wantLen:   currentAPIKeyLookupLength,
			wantStart: "01234567",
		},
		{
			name:      "accepts legacy 16-char lookup",
			raw:       "nf_0123456789abcdef_abcdefghijklmnopqrstuvwxyz123456",
			wantOK:    true,
			wantLen:   legacyAPIKeyLookupLength,
			wantStart: "01234567",
		},
		{
			name:   "rejects unexpected lookup length",
			raw:    "nf_0123456789abcde_abcdefghijklmnopqrstuvwxyz123456",
			wantOK: false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			lookup, secret, ok := parseAPIKeyRaw(tt.raw)
			if ok != tt.wantOK {
				t.Fatalf("parseAPIKeyRaw() ok = %v, want %v", ok, tt.wantOK)
			}
			if !tt.wantOK {
				return
			}
			if len(lookup) != tt.wantLen {
				t.Fatalf("lookup len = %d, want %d", len(lookup), tt.wantLen)
			}
			if lookup[:8] != tt.wantStart {
				t.Fatalf("lookup prefix = %q, want %q", lookup[:8], tt.wantStart)
			}
			if secret == "" {
				t.Fatal("secret should not be empty")
			}
		})
	}
}
