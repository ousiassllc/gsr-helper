package tarball

import (
	"errors"
	"testing"
)

func TestSafeName(t *testing.T) {
	tests := []struct {
		name string
		// entry は tar のヘッダに入っている名前。
		entry string
		// want は正規化後の名前。空はエントリを読み飛ばす合図。
		want string
		// wantUnsafe は ErrUnsafePath で拒否されることを期待するか。
		wantUnsafe bool
	}{
		{name: "先頭の ./ を落とす", entry: "./bin/config.sh", want: "bin/config.sh", wantUnsafe: false},
		{name: "そのままの相対パス", entry: "bin/config.sh", want: "bin/config.sh", wantUnsafe: false},
		{name: "末尾の / を落とす", entry: "./bin/", want: "bin", wantUnsafe: false},
		{name: "アーカイブの根は読み飛ばす", entry: "./", want: "", wantUnsafe: false},
		{name: "空の名前は読み飛ばす", entry: "", want: "", wantUnsafe: false},
		{name: "先頭の ..", entry: "../evil", want: "", wantUnsafe: true},
		{name: "途中の ..", entry: "bin/../../evil", want: "", wantUnsafe: true},
		{name: "絶対パス", entry: "/etc/passwd", want: "", wantUnsafe: true},
		// ".." で始まるだけの名前は正規のファイル名であり、拒否してはいけない。
		{name: "..foo は通す", entry: "./..foo", want: "..foo", wantUnsafe: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := safeName(tt.entry)
			if tt.wantUnsafe {
				if !errors.Is(err, ErrUnsafePath) {
					t.Fatalf("エラーが ErrUnsafePath ではない: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("safeName(%q) がエラーを返した: %v", tt.entry, err)
			}
			if got != tt.want {
				t.Errorf("safeName(%q) が %q（期待 %q）", tt.entry, got, tt.want)
			}
		})
	}
}

func TestCheckLinkTarget(t *testing.T) {
	tests := []struct {
		name string
		// entry はリンク自身の（正規化済みの）名前。
		entry string
		// linkname はリンクの向き先。
		linkname string
		// wantUnsafe は ErrUnsafePath で拒否されることを期待するか。
		wantUnsafe bool
	}{
		{name: "同じディレクトリを指す", entry: "bin/alias.sh", linkname: "runsvc.sh", wantUnsafe: false},
		{name: "上の階層だが展開先の中", entry: "bin/x/alias.sh", linkname: "../../config.sh", wantUnsafe: false},
		{name: "展開先そのものを指す", entry: "bin/here", linkname: "..", wantUnsafe: false},
		{name: "展開先の外へ出る", entry: "bin/alias.sh", linkname: "../../../etc/passwd", wantUnsafe: true},
		{name: "絶対パス", entry: "bin/alias.sh", linkname: "/etc/passwd", wantUnsafe: true},
		{name: "空のリンク先", entry: "bin/alias.sh", linkname: "", wantUnsafe: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := checkLinkTarget(tt.entry, tt.linkname)
			if tt.wantUnsafe {
				if !errors.Is(err, ErrUnsafePath) {
					t.Fatalf("エラーが ErrUnsafePath ではない: %v", err)
				}
				return
			}
			if err != nil {
				t.Errorf("checkLinkTarget(%q, %q) がエラーを返した: %v", tt.entry, tt.linkname, err)
			}
		})
	}
}

// 先頭要素の完全一致で判定すること。前方一致にすると _workspace のような
// 正規のディレクトリまで展開されなくなる。
func TestIsPreserved(t *testing.T) {
	keep := PreservedNames()

	tests := map[string]bool{
		"_work":            true,
		"_work/repo/a.txt": true,
		".env":             true,
		".credentials":     true,
		"_workspace":       false,
		"_work2/x":         false,
		".envrc":           false,
		"bin/config.sh":    false,
	}
	for name, want := range tests {
		t.Run(name, func(t *testing.T) {
			if got := isPreserved(name, keep); got != want {
				t.Errorf("isPreserved(%q) が %v（期待 %v）", name, got, want)
			}
		})
	}
}
