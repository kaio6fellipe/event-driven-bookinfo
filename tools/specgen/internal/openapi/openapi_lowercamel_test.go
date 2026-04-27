package openapi

import "testing"

func TestLowerCamelCase(t *testing.T) {
	tests := []struct {
		name   string
		method string
		path   string
		want   string
	}{
		{name: "POST collection", method: "POST", path: "/v1/ratings", want: "postV1Ratings"},
		{name: "GET resource", method: "GET", path: "/v1/ratings/{id}", want: "getV1RatingsId"},
		{name: "DELETE no path", method: "DELETE", path: "/", want: "delete"},
		{name: "complex admin route", method: "POST", path: "/v1/events/{id}/replay", want: "postV1EventsIdReplay"},
		{name: "batch route", method: "POST", path: "/v1/events/batch/replay", want: "postV1EventsBatchReplay"},
		{name: "multiple path params", method: "GET", path: "/v1/users/{userId}/posts/{postId}", want: "getV1UsersUserIdPostsPostId"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := lowerCamelCase(tt.method, tt.path)
			if got != tt.want {
				t.Errorf("lowerCamelCase(%q, %q) = %q, want %q", tt.method, tt.path, got, tt.want)
			}
		})
	}
}
