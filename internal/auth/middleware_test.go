package auth

import "testing"

func TestBearerToken(t *testing.T) {
	tests := []struct {
		name   string
		header string
		want   string
		ok     bool
	}{
		{name: "valid", header: "Bearer abc.def", want: "abc.def", ok: true},
		{name: "case insensitive", header: "bearer token", want: "token", ok: true},
		{name: "missing", header: "", ok: false},
		{name: "wrong scheme", header: "Basic token", ok: false},
		{name: "extra fields", header: "Bearer token extra", ok: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, ok := bearerToken(test.header)
			if got != test.want || ok != test.ok {
				t.Fatalf("bearerToken(%q) = %q, %v; want %q, %v", test.header, got, ok, test.want, test.ok)
			}
		})
	}
}
