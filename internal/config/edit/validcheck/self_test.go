package validcheck

import (
	"errors"
	"strings"
	"testing"

	"github.com/ousiassllc/gsr-helper/internal/config/edit"
)

// 入力の検証が appconfig と同じ規則で通ること。
func TestSelfValidators(t *testing.T) {
	t.Parallel()

	if err := edit.ValidatePercent("0"); !errors.Is(err, edit.ErrPercentRange) {
		t.Errorf("ValidatePercent(0) = %v, want ErrPercentRange", err)
	}
	if err := edit.ValidateRefresh("x"); !errors.Is(err, edit.ErrBadNumber) {
		t.Errorf("ValidateRefresh(x) = %v, want ErrBadNumber", err)
	}
	// 監査ログの欄なのに「work dir」と出ていた回帰の防止。
	err := edit.ValidateAuditLog("relative/audit.log")
	if err == nil || !strings.Contains(err.Error(), "監査ログ") {
		t.Errorf("ValidateAuditLog() のエラー = %v, want 監査ログ を含む", err)
	}
}
