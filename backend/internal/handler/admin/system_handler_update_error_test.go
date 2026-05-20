package admin

import (
	"errors"
	"testing"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
)

func TestSystemUpdateFailedErrorExposesSafeCause(t *testing.T) {
	err := systemUpdateFailedError(errors.New("backup failed: rename /opt/sub2api/sub2api /opt/sub2api/sub2api.backup: permission denied"))

	require.Equal(t, 500, infraerrors.Code(err))
	require.Equal(t, "SYSTEM_UPDATE_FAILED", infraerrors.Reason(err))
	require.Contains(t, infraerrors.Message(err), "System update failed: backup failed")
	require.Contains(t, infraerrors.Message(err), "permission denied")
}
