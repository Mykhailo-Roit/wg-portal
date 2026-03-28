package config

import "testing"

func TestWebConfigValidate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		externalURL string
		wantErr     bool
	}{
		{
			name:        "allow localhost http",
			externalURL: "http://localhost:8888",
		},
		{
			name:        "allow https",
			externalURL: "https://portal.example.com",
		},
		{
			name:        "reject public http",
			externalURL: "http://portal.example.com",
			wantErr:     true,
		},
		{
			name:        "reject path in external url",
			externalURL: "https://portal.example.com/wg",
			wantErr:     true,
		},
		{
			name:        "reject relative url",
			externalURL: "/wg",
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := (WebConfig{ExternalUrl: tt.externalURL}).Validate()
			if (err != nil) != tt.wantErr {
				t.Fatalf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
