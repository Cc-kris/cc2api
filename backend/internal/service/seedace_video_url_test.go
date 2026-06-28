package service

import "testing"

func TestExtractSeedaceVideoURL(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{name: "nested video_url", body: `{"data":{"video_url":"https://cdn.example.com/video.mp4"}}`, want: "https://cdn.example.com/video.mp4"},
		{name: "nested result_url", body: `{"result":{"result_url":"http://cdn.example.com/video.mp4"}}`, want: "http://cdn.example.com/video.mp4"},
		{name: "reject javascript", body: `{"video_url":"javascript:alert(1)"}`, want: ""},
		{name: "reject relative", body: `{"video_url":"/video.mp4"}`, want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ExtractSeedaceVideoURL([]byte(tt.body)); got != tt.want {
				t.Fatalf("ExtractSeedaceVideoURL() = %q, want %q", got, tt.want)
			}
		})
	}
}
