package main

import (
	"fmt"
	"io"
	"sync"
)

// auditSink は監査記録の失敗を集約し、TUI の終了後にまとめて報告する。
//
// 記録の失敗（ディスク満杯、ローテーション後の書き込み不能など）は「コマンドの
// 失敗」ではないため Executor の戻り値には載らない。一方で黙って捨てると操作の
// 追跡可能性（docs/architecture/security.md の監査ログ）が失われるため、
// 必ず利用者の目に触れる形で残す必要がある。
//
// その場で stderr に書かないのは、bubbletea が代替スクリーンを掌握している間に
// stderr へ書くと画面が壊れるためである。Program.Run が戻った後に出せば、
// 監査ログを開けなかったときの警告と同じ位置（終了後の画面）に並ぶ。
//
// Executor は複数の goroutine から実行されるため、通知も並行して届く。
type auditSink struct {
	mu    sync.Mutex
	count int
	first error
}

// add は失敗を 1 件受け取る。Executor に渡す通知先。
func (s *auditSink) add(err error) {
	if err == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.count++
	if s.first == nil {
		s.first = err
	}
}

// report は失敗があれば w に 1 行で報告する。
//
// 件数と最初の 1 件だけを出す。同じ原因（書き込み先が壊れている）で全件が失敗する
// のが普通であり、全文を並べても読み手の判断は変わらないためである。
func (s *auditSink) report(w io.Writer) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.count == 0 {
		return
	}
	_, _ = fmt.Fprintf(w, "警告: 監査ログの記録に %d 件失敗しました（最初の失敗: %v）\n", s.count, s.first)
}

// reportAuditClose は監査ログのクローズの失敗を w に 1 行で報告する。
//
// **クローズの失敗を捨てると、利用者が唯一見られない監査ログのエラーになる。**
// 開けなかった場合（openAudit）と記録に失敗した場合（auditSink.report）はどちらも
// 警告として出るのに、閉じ損ないだけが誰の目にも触れない状態だった。
// audit.Logger.Close はまさに報告されるためにエラーを包んでいる。
//
// 終了コードは変えない。ここに至る時点で TUI は正常に終わっており、記録の失敗を
// 操作の失敗として扱わない方針（同上の 2 つと同じ）に揃える。
func reportAuditClose(err error, w io.Writer) {
	if err == nil {
		return
	}
	_, _ = fmt.Fprintf(w, "警告: %v\n", err)
}
