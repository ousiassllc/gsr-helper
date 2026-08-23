package logs_test

import (
	"path/filepath"
	"testing"

	"github.com/ousiassllc/gsr-helper/internal/logs"
)

// Worker ログ本文の解析（Issue #68）を実ログの形式に対して検証する。
//
// testdata の行は actions/runner のソース（HEAD 258d6c857）で確認した書式に
// 揃えてある。すなわち `[{UTC} {LEVEL 4 文字} {Category}] {message}` で、
// LEVEL は `INFO` / `WARN` / `ERR `（末尾に空白）などの 4 文字固定幅である。

// testdataDir は testdata の Worker ログを置いたディレクトリ。
const testdataDir = "testdata"

func TestParseWorker(t *testing.T) {
	const (
		wantRepo = "ousiassllc/gsr-helper"
		wantWork = "/opt/runners/build01-1/_work/gsr-helper/gsr-helper"
	)

	tests := []struct {
		name     string
		file     string
		wantRepo string
		wantWork string
	}{
		{
			// 1) tracking config 探索行のパスから取れる（最も確実な経路）。
			name: "tracking config の _PipelineMapping から取れる",
			file: "worker_tracking_config.log", wantRepo: wantRepo, wantWork: wantWork,
		},
		{
			// 2) Job message の JSON ダンプの "k":"repository" から取れる。
			// 作業ディレクトリは ProcessInvokerWrapper の行から取る。
			name: "Job message の JSON から取れる",
			file: "worker_job_message_json.log", wantRepo: wantRepo, wantWork: wantWork,
		},
		{
			// 3) multi-repo checkout の行から取れる。最初の 1 件を採る。
			name: "multi-repo checkout の行から取れる",
			// このログには作業ディレクトリを出す行が無いので Workspace は空になる
			// （フォールバックの組み立ては WorkspaceFallback の担当）。
			file: "worker_multi_repo.log", wantRepo: wantRepo,
		},
		{
			// 取り出し口がどれも無いログ。空へ縮退し、失敗にはしない。
			name: "どの取り出し口も無ければ空になる",
			file: "worker_no_match.log",
		},
		{
			// ジョブが始まったばかりで途中までしか書かれていないログ。
			name: "途中で切れていても壊れない",
			file: "worker_truncated.log",
		},
		{
			// 存在しないログ。開けない場合も空へ縮退する（Jobs タブを失敗させない）。
			name: "ファイルが無ければ空になる",
			file: "worker_does_not_exist.log",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := logs.ParseWorker(testdataDir, tt.file)
			if err != nil {
				t.Fatalf("ParseWorker がエラーを返した: %v", err)
			}
			if got.Repository != tt.wantRepo {
				t.Errorf("Repository = %q, want %q", got.Repository, tt.wantRepo)
			}
			if got.Workspace != tt.wantWork {
				t.Errorf("Workspace = %q, want %q", got.Workspace, tt.wantWork)
			}
		})
	}
}

// ファイル名で _diag の外へ出ようとしても読まない。
//
// ParseWorker は exported で呼び出し側を選べないため、閉じ込めをコメントの約束に
// せず io/fs の側で担保している（os.DirFS が fs.ValidPath を要求する）。
func TestParseWorkerDoesNotEscapeDirectory(t *testing.T) {
	escapes := []string{
		filepath.Join("..", "logs.go"),
		filepath.Join("..", "testdata", "worker_tracking_config.log"),
		"/etc/hostname",
	}

	for _, name := range escapes {
		t.Run(name, func(t *testing.T) {
			got, err := logs.ParseWorker(testdataDir, name)
			if err != nil {
				t.Fatalf("ParseWorker がエラーを返した: %v", err)
			}
			if got != (logs.JobInfo{}) {
				t.Errorf("ParseWorker(%q, %q) = %+v, want 空（ディレクトリの外を読んでいる）",
					testdataDir, name, got)
			}
		})
	}
}

func TestWorkspaceFallback(t *testing.T) {
	tests := []struct {
		name    string
		workDir string
		repo    string
		want    string
	}{
		{
			// TrackingConfig.cs の WorkspaceDirectory は <repo>/<repo> で owner を含まない。
			name:    "owner を含まない <repo>/<repo> を組み立てる",
			workDir: "/opt/runners/build01-1/_work", repo: "ousiassllc/gsr-helper",
			want: "/opt/runners/build01-1/_work/gsr-helper/gsr-helper",
		},
		{name: "owner/repo の形でなければ空", workDir: "/w", repo: "gsr-helper", want: ""},
		{name: "repo が空なら空", workDir: "/w", repo: "ousiassllc/", want: ""},
		{name: "リポジトリ名が空なら空", workDir: "/w", repo: "", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := logs.WorkspaceFallback(tt.workDir, tt.repo); got != tt.want {
				t.Errorf("WorkspaceFallback(%q, %q) = %q, want %q", tt.workDir, tt.repo, got, tt.want)
			}
		})
	}
}
