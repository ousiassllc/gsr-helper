package edit_test

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ousiassllc/gsr-helper/internal/config/edit"
	"github.com/ousiassllc/gsr-helper/internal/runner"
)

// envIndex は EnvKeys のうちキーが一致する位置を返す。
func envIndex(t *testing.T, key string) int {
	t.Helper()

	for i, spec := range edit.EnvKeys {
		if spec.Key == key {
			return i
		}
	}
	t.Fatalf("EnvKeys に %q が無い", key)

	return -1
}

// envValues は EnvKeys と同じ長さの入力を作る。
func envValues() []string { return make([]string, len(edit.EnvKeys)) }

// .env の変更が、変えたキーの行だけを差し替えること。コメントと空行、および
// フォームに出していないキーがそのまま残ることを見る（受け入れ条件）。
func TestBuildEnvReplacesOnlyChangedLine(t *testing.T) {
	t.Parallel()

	const before = "# ジョブ環境\nPATH=/usr/bin\n\n# プロキシ\nhttps_proxy=http://old:3128\nCUSTOM=残す\n"
	r := sample(t, "build01-1", before)
	ld := edit.Loader{DropInRoot: ""}

	next := envValues()
	next[envIndex(t, "PATH")] = "/usr/bin"
	next[envIndex(t, "https_proxy")] = "http://new:3128"

	was := envValues()
	was[envIndex(t, "PATH")] = "/usr/bin"
	was[envIndex(t, "https_proxy")] = "http://old:3128"

	c, err := edit.BuildEnv(ld, r, next, was)
	if err != nil {
		t.Fatalf("BuildEnv() でエラー: %v", err)
	}
	if !c.Changed() {
		t.Fatal("変更ありと判定されていない")
	}

	const want = "# ジョブ環境\nPATH=/usr/bin\n\n# プロキシ\nhttps_proxy=http://new:3128\nCUSTOM=残す\n"
	if got := writeAndRead(t, c, filepath.Join(r.Dir, ".env")); got != want {
		t.Errorf("書き込み後 =\n%q\nwant\n%q", got, want)
	}

	// 差分は変更した 1 行だけを +/- で示す。
	joined := strings.Join(c.DiffLines(), "\n")
	if !strings.Contains(joined, "- https_proxy=http://old:3128") ||
		!strings.Contains(joined, "+ https_proxy=http://new:3128") {
		t.Errorf("差分に変更行が出ていない:\n%s", joined)
	}
	if strings.Contains(joined, "- CUSTOM") || strings.Contains(joined, "+ CUSTOM") {
		t.Errorf("触っていないキーが差分に出ている:\n%s", joined)
	}
}

// 空欄にした項目は行ごと取り除き、もともと無い項目には空行を足さないこと。
func TestBuildEnvUnsetAndSkip(t *testing.T) {
	t.Parallel()

	r := sample(t, "build01-1", "PATH=/usr/bin\nLANG=ja_JP.UTF-8\n")
	ld := edit.Loader{DropInRoot: ""}

	next, was := envValues(), envValues()
	was[envIndex(t, "LANG")] = "ja_JP.UTF-8"
	next[envIndex(t, "PATH")] = "/usr/bin"
	was[envIndex(t, "PATH")] = "/usr/bin"

	c, err := edit.BuildEnv(ld, r, next, was)
	if err != nil {
		t.Fatalf("BuildEnv() でエラー: %v", err)
	}

	got := writeAndRead(t, c, filepath.Join(r.Dir, ".env"))
	if want := "PATH=/usr/bin\n"; got != want {
		t.Errorf("書き込み後 = %q, want %q", got, want)
	}
	if strings.Contains(got, "ImageOS") {
		t.Error("入力しなかったキーが書き足されている")
	}
}

// 変更が無ければ Changed が偽になり、承認も書き込みも要らないこと。
func TestBuildEnvNoChange(t *testing.T) {
	t.Parallel()

	r := sample(t, "build01-1", "PATH=/usr/bin\n")
	next, was := envValues(), envValues()
	next[envIndex(t, "PATH")] = "/usr/bin"
	was[envIndex(t, "PATH")] = "/usr/bin"

	c, err := edit.BuildEnv(edit.Loader{DropInRoot: ""}, r, next, was)
	if err != nil {
		t.Fatalf("BuildEnv() でエラー: %v", err)
	}
	if c.Changed() {
		t.Errorf("変更なしのはず: %v", c.DiffLines())
	}
}

// .path の変更。書き込み後は末尾に改行がちょうど 1 つ付く。
func TestBuildPath(t *testing.T) {
	t.Parallel()

	r := sample(t, "build01-1", "")
	write(t, filepath.Join(r.Dir, ".path"), "/usr/bin\n")

	c, err := edit.BuildPath(edit.Loader{DropInRoot: ""}, r, "/opt/bin:/usr/bin")
	if err != nil {
		t.Fatalf("BuildPath() でエラー: %v", err)
	}

	got := writeAndRead(t, c, filepath.Join(r.Dir, ".path"))
	if want := "/opt/bin:/usr/bin\n"; got != want {
		t.Errorf("書き込み後 = %q, want %q", got, want)
	}
}

// drop-in の変更。空欄にした項目は行ごと消え、daemon-reload が要ると印が付く。
func TestBuildDropIn(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	r := sample(t, "build01-1", "")
	ld := edit.Loader{DropInRoot: root}

	c, err := edit.BuildDropIn(ld, r, "always", "4G")
	if err != nil {
		t.Fatalf("BuildDropIn() でエラー: %v", err)
	}
	if !c.Reload() {
		t.Error("drop-in の変更に daemon-reload の印が無い")
	}

	path, ok := ld.DropInPath(r)
	if !ok {
		t.Fatal("drop-in のパスが決まらない")
	}
	got := writeAndRead(t, c, path)
	if !strings.Contains(got, "[Service]") ||
		!strings.Contains(got, "Restart=always") ||
		!strings.Contains(got, "MemoryMax=4G") {
		t.Errorf("書き込んだ drop-in = %q", got)
	}

	// MemoryMax を空にすると行ごと消える。
	c2, err := edit.BuildDropIn(ld, r, "always", "")
	if err != nil {
		t.Fatalf("BuildDropIn() でエラー: %v", err)
	}
	if got := writeAndRead(t, c2, path); strings.Contains(got, "MemoryMax") {
		t.Errorf("空欄にした項目が残っている: %q", got)
	}
}

// ユニット名の無い runner では drop-in を編集できないこと。
func TestBuildDropInWithoutUnit(t *testing.T) {
	t.Parallel()

	r := sample(t, "build01-1", "")
	r.UnitName = ""

	if _, err := edit.BuildDropIn(edit.Loader{DropInRoot: ""}, r, "always", ""); !errors.Is(err, edit.ErrNoUnit) {
		t.Fatalf("エラー = %v, want ErrNoUnit", err)
	}
}

// 複製は選ばれた runner だけを対象にし、選ばれなかった台には触れないこと（FR-40）。
func TestBuildCopy(t *testing.T) {
	t.Parallel()

	src := sample(t, "build01-1", "PATH=/opt/bin\n")
	dst := sample(t, "build01-2", "PATH=/usr/bin\n")
	skip := sample(t, "build01-3", "PATH=/keep\n")
	ld := edit.Loader{DropInRoot: ""}

	c, err := edit.BuildCopy(ld, src, []runner.Runner{dst, skip}, []string{"build01-2"})
	if err != nil {
		t.Fatalf("BuildCopy() でエラー: %v", err)
	}
	if c.Copies() != 1 {
		t.Fatalf("複製先の台数 = %d, want 1", c.Copies())
	}
	if !c.FileBacked() {
		t.Error("複製はファイルへの書き込みを伴うはず")
	}
	if err := c.Write(); err != nil {
		t.Fatalf("書き込みでエラー: %v", err)
	}

	if got := read(t, filepath.Join(dst.Dir, ".env")); got != "PATH=/opt/bin\n" {
		t.Errorf("複製先 = %q, want %q", got, "PATH=/opt/bin\n")
	}
	if got := read(t, filepath.Join(skip.Dir, ".env")); got != "PATH=/keep\n" {
		t.Errorf("選ばなかった台が書き換えられた: %q", got)
	}
	// 複製先は書き込み前に退避される（FR-38）。
	if got := read(t, filepath.Join(dst.Dir, ".env.bak")); got != "PATH=/usr/bin\n" {
		t.Errorf("複製先のバックアップ = %q, want %q", got, "PATH=/usr/bin\n")
	}
}

// 複製先を 1 つも選ばなければ組み立てを拒むこと。
func TestBuildCopyWithoutTargets(t *testing.T) {
	t.Parallel()

	src := sample(t, "build01-1", "PATH=/opt/bin\n")
	dst := sample(t, "build01-2", "")

	_, err := edit.BuildCopy(edit.Loader{DropInRoot: ""}, src, []runner.Runner{dst}, nil)
	if !errors.Is(err, edit.ErrEmptyCopyTarget) {
		t.Fatalf("エラー = %v, want ErrEmptyCopyTarget", err)
	}
}

// ラベルと runner group はファイルを書かないので、退避先も反映の対象にもならない。
func TestLabelAndGroupChangesAreNotFileBacked(t *testing.T) {
	t.Parallel()

	labels := edit.BuildLabels([]string{"gpu"}, []string{"gpu", "cuda"})
	if labels.FileBacked() || labels.BackupPath() != "" {
		t.Errorf("ラベルの変更がファイル扱いになっている: %+v", labels)
	}
	if !labels.Changed() {
		t.Error("ラベルの変更が変更なしと判定された")
	}
	if !strings.Contains(labels.Title(), "ラベル") {
		t.Errorf("見出し = %q", labels.Title())
	}

	group := edit.BuildGroup("Default", "gpu", 7)
	if group.FileBacked() {
		t.Error("runner group の変更がファイル扱いになっている")
	}
}

// writeAndRead は変更を書き込んで結果を読み返す。
func writeAndRead(t *testing.T, c edit.Change, path string) string {
	t.Helper()

	if err := c.Write(); err != nil {
		t.Fatalf("書き込みでエラー: %v", err)
	}
	return read(t, path)
}
