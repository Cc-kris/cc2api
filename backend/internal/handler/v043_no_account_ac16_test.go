//go:build unit

package handler

import (
	"errors"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestV043NoAccountAC16Preserves404AgainstModelCooldown(t *testing.T) {
	key := &service.APIKey{GroupID: ptrInt64(42)}
	for _, hasSupport := range []bool{false, true} {
		d := &fakeDiagnoser{resp: service.ModelAvailabilityDiagnosis{HasAccountsInPool: true, HasModelSupport: hasSupport}}
		cls := classifyNoAccountErrorFromGin(newTestGinContextWithRequest(), d, key, "model-a", "alias-a", service.PlatformOpenAI)
		cls = classifySelectionFailureError(errors.New("no accounts: model_rate_limited=1"), cls)
		if hasSupport {
			require.Equal(t, 429, cls.Status)
			require.False(t, cls.ModelNotFound)
		} else {
			require.Equal(t, 404, cls.Status)
			require.Equal(t, "model_not_found", cls.ErrType)
			require.Contains(t, cls.Message, "alias-a")
		}
	}
}
