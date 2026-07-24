package admin

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestGrokPlatformBindings(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name string
		body string
		new  func() any
	}{
		{
			name: "group create",
			body: `{"name":"Grok","platform":"grok"}`,
			new:  func() any { return &CreateGroupRequest{} },
		},
		{
			name: "group update",
			body: `{"platform":"grok"}`,
			new:  func() any { return &UpdateGroupRequest{} },
		},
		{
			name: "monitor create",
			body: `{"name":"Grok","provider":"grok","endpoint":"https://api.x.ai/v1","api_key":"test","primary_model":"grok-4.5","interval_seconds":60}`,
			new:  func() any { return &channelMonitorCreateRequest{} },
		},
		{
			name: "monitor update",
			body: `{"provider":"grok"}`,
			new:  func() any { return &channelMonitorUpdateRequest{} },
		},
		{
			name: "monitor template create",
			body: `{"name":"Grok","provider":"grok"}`,
			new:  func() any { return &channelMonitorTemplateCreateRequest{} },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Request = httptest.NewRequest("POST", "/", strings.NewReader(tt.body))
			ctx.Request.Header.Set("Content-Type", "application/json")

			require.NoError(t, ctx.ShouldBindJSON(tt.new()))
		})
	}
}
