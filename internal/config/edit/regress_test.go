package edit_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ousiassllc/gsr-helper/internal/appconfig"
	"github.com/ousiassllc/gsr-helper/internal/config/edit"
	"github.com/ousiassllc/gsr-helper/internal/runner"
)

// drop-in の差分が「書き込みで消える行」を隠さないこと（FR-37）。
//
// **解析結果の再描画を before にしていた回帰の防止である。** Parse はコメントと
// [Service] 以外のセクションを捨てるので、再描画どうしを比べると手書きの
// override.conf から [Unit] が消えることが差分に 1 行も出ない。
func TestBuildDropInShowsLinesThatWillDisappear(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	r := sample(t, "build01-1", "")
	dir := filepath.Join(root, r.UnitName+".d")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("drop-in ディレクトリの作成に失敗: %v", err)
	}
	write(t, filepath.Join(dir, "override.conf"),
		"# 手で書いた注釈\n[Unit]\nAfter=network.target\n\n[Service]\nRestart=always\n")

	c, err := edit.BuildDropIn(edit.Loader{DropInRoot: root}, r, "on-failure", "")
	if err != nil {
		t.Fatalf("BuildDropIn() でエラー: %v", err)
	}

	diff := strings.Join(c.DiffLines(), "\n")
	for _, want := range []string{"- # 手で書いた注釈", "- [Unit]", "- After=network.target"} {
		if !strings.Contains(diff, want) {
			t.Errorf("差分に %q が無い:\n%s", want, diff)
		}
	}
}

// 現在値と同じ内容で確定したら変更なしと判定すること（GitHub 側の値）。
//
// before を捨てて組むと差分が全行 + になり、何も変えずに確定しただけで
// 置換 / 付け替えの API を呼んでしまう。
func TestGitHubChangesDetectNoOp(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		change edit.Change
		want   bool
	}{
		"ラベル同じ":    {edit.BuildLabels("build01-1", []string{"gpu"}, []string{"gpu"}), false},
		"ラベル違う":    {edit.BuildLabels("build01-1", []string{"gpu"}, []string{"cuda"}), true},
		"group 同じ": {edit.BuildGroup("build01-1", "Default", "Default", 1), false},
		"group 違う": {edit.BuildGroup("build01-1", "Default", "gpu", 2), true},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if got := tt.change.Changed(); got != tt.want {
				t.Errorf("Changed() = %v, want %v", got, tt.want)
			}
			// 対象がどの runner かを承認の見出しで見せる（対象は commit 時に決まる）。
			if !strings.Contains(tt.change.Title(), "build01-1") {
				t.Errorf("見出し = %q, want runner 名を含む", tt.change.Title())
			}
		})
	}
}

// 複製の反映対象が複製元ではなく複製先であること（FR-40）。
func TestApplyTargetsOfCopy(t *testing.T) {
	t.Parallel()

	src := sample(t, "build01-1", "PATH=/usr/bin\n")
	dst := sample(t, "build01-2", "PATH=/old\n")
	noUnit := sample(t, "build01-3", "")
	noUnit.UnitName = ""

	all := []runner.Runner{src, dst, noUnit}
	c, err := edit.BuildCopy(edit.Loader{DropInRoot: ""}, src, []runner.Runner{dst, noUnit},
		[]string{dst.Name(), noUnit.Name()})
	if err != nil {
		t.Fatalf("BuildCopy() でエラー: %v", err)
	}

	got := edit.Names(c.ApplyTargets(all, src))
	if len(got) != 1 || got[0] != dst.Name() {
		t.Errorf("反映対象 = %v, want [%s]（複製元は含まず、ユニットの無い台は落とす）", got, dst.Name())
	}
}

// 複製が途中で失敗したら、どこまで書き換えたかが文言に出ること。
func TestCopyFailureNamesWrittenTargets(t *testing.T) {
	t.Parallel()

	if os.Geteuid() == 0 {
		t.Skip("root は書き込み権限の制限を受けないため、この経路は検証できない")
	}

	src := sample(t, "build01-1", "PATH=/usr/bin\n")
	ok := sample(t, "build01-2", "PATH=/old\n")
	bad := sample(t, "build01-3", "PATH=/old\n")
	if err := os.Chmod(bad.Dir, 0o500); err != nil {
		t.Fatalf("ディレクトリの権限変更に失敗: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(bad.Dir, 0o700) })

	targets := []runner.Runner{ok, bad}
	c, err := edit.BuildCopy(edit.Loader{DropInRoot: ""}, src, targets, edit.Names(targets))
	if err != nil {
		t.Fatalf("BuildCopy() でエラー: %v", err)
	}

	werr := c.Write()
	if werr == nil {
		t.Fatal("書き込めないはずの複製先で成功した")
	}
	if !strings.Contains(werr.Error(), ok.Name()) || !strings.Contains(werr.Error(), bad.Name()) {
		t.Errorf("エラー文言 = %q, want 成功した台と失敗した台の両方を含む", werr.Error())
	}
}

// GitHub API で反映できない種類の変更を API 経路へ落とさないこと。
//
// 以前は「ラベルでなければ runner group」と書いていたため、ゼロ値の Change が
// groupID 0 のまま付け替えの API を呼びうる形になっていた。
func TestCommitRejectsNonAPIChange(t *testing.T) {
	t.Parallel()

	err := edit.Commit(context.Background(), edit.CommitInput{
		Change: edit.Change{Kind: edit.KindEnv}, Runner: sample(t, "build01-1", ""),
		Client: nil,
	})
	if err == nil {
		t.Fatal("想定外の Kind が API 経路を通ってしまった")
	}
}

// 自身の設定は正規化を通してから差分にすること（承認した内容 == 書かれる内容）。
// 警告 >= 危険 の組み合わせもここで弾く（huh の Validate は 1 欄しか見えない）。
func TestSelfValuesApply(t *testing.T) {
	t.Parallel()

	base := appconfig.Default()

	tests := map[string]struct {
		vals    edit.SelfValues
		wantErr bool
		want    string
	}{
		"走査ルートを Clean する": {
			edit.SelfValues{
				ScanRoots: " /opt//runners/ ", Refresh: "5", Warn: "80",
				Critical: "90", AuditLog: "/var/log/audit.log",
			},
			false, "/opt/runners",
		},
		"警告が危険以上なら弾く": {
			edit.SelfValues{
				ScanRoots: "", Refresh: "5", Warn: "90",
				Critical: "80", AuditLog: "/var/log/audit.log",
			},
			true, "",
		},
		"数として読めない": {
			edit.SelfValues{
				ScanRoots: "", Refresh: "x", Warn: "80",
				Critical: "90", AuditLog: "/var/log/audit.log",
			},
			true, "",
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got, err := tt.vals.Apply(base)
			if tt.wantErr {
				if err == nil {
					t.Fatal("エラーになるはずの入力が通った")
				}
				return
			}
			if err != nil {
				t.Fatalf("Apply() でエラー: %v", err)
			}
			if !strings.Contains(edit.RenderConfig(got), tt.want) {
				t.Errorf("差分の表現 = %q, want %q を含む", edit.RenderConfig(got), tt.want)
			}
		})
	}
}
