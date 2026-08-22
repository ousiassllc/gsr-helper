package exec

import (
	"os"
	"testing"
)

func TestLookPath(t *testing.T) {
	self := os.Args[0]
	if _, err := LookPath(self); err != nil {
		t.Errorf("LookPath(%q) がエラーを返した: %v", self, err)
	}
	if _, err := LookPath("gsr-helper-no-such-command"); err == nil {
		t.Error("存在しないコマンドでエラーを返していない")
	}
}
