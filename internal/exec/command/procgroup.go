package command

import (
	"errors"
	"fmt"
	"os"
	"syscall"
)

// killProcessGroup はプロセス p のプロセスグループ全体に SIGKILL を送る。
//
// 直接の子だけを kill しても足りない。runner のスクリプト（./config.sh や
// svc.sh）は sudo や systemctl を起動するため、子だけを殺すと孫が runner
// ディレクトリを掴んだまま孤児として残る。cmd.WaitDelay は Run が戻らなくなる
// ことを防ぐだけで、孫の始末はしない。
func killProcessGroup(p *os.Process) error {
	if p == nil {
		return nil
	}

	// 負の PID はプロセスグループ全体を指す。Setpgid で子をグループリーダに
	// しているため、子の PID がそのままグループ ID になる。
	if err := syscall.Kill(-p.Pid, syscall.SIGKILL); err != nil {
		// 既に全員終了していただけなので異常ではない。os.ErrProcessDone を返すのは
		// 既定の Cancel（Process.Kill）と同じ扱いにするためで、こうすると
		// 中断と自然終了が競合しただけの実行に偽のエラーが付かない。
		if errors.Is(err, syscall.ESRCH) || errors.Is(err, os.ErrProcessDone) {
			return os.ErrProcessDone
		}
		return fmt.Errorf("プロセスグループ %d の停止に失敗しました: %w", p.Pid, err)
	}
	return nil
}
