package api

import (
	"net/http"
	"testing"
)

func TestBearerToken(t *testing.T) {
	cases := []struct {
		name    string
		headers map[string]string
		want    string
	}{
		{
			name:    "own header",
			headers: map[string]string{TokenHeader: "abc"},
			want:    "abc",
		},
		{
			name:    "authorization",
			headers: map[string]string{"Authorization": "Bearer abc"},
			want:    "abc",
		},
		{
			// A proxy that authenticates the employee leaves its own token in
			// Authorization; ours has to win.
			name:    "own header wins over a proxy's authorization",
			headers: map[string]string{TokenHeader: "abc", "Authorization": "Bearer proxy-issued-jwt"},
			want:    "abc",
		},
		{
			name:    "websocket subprotocol",
			headers: map[string]string{"Sec-WebSocket-Protocol": "estimeet.v1, bearer.abc"},
			want:    "abc",
		},
		{
			name:    "nothing",
			headers: map[string]string{"Authorization": "Basic abc"},
			want:    "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r, err := http.NewRequest(http.MethodGet, "/api/rooms/ABC123/state", nil)
			if err != nil {
				t.Fatal(err)
			}
			for k, v := range tc.headers {
				r.Header.Set(k, v)
			}
			if got := bearerToken(r); got != tc.want {
				t.Fatalf("bearerToken() = %q, want %q", got, tc.want)
			}
		})
	}
}
