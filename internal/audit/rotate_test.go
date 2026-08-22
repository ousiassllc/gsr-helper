package audit

import (
	"os"
	"strings"
	"testing"
)

func TestOpenReopensAfterRotation(t *testing.T) {
	// logrotate 既定の create モードは rename でファイルを退避させる。fd を
	// 握り続けると以降のレコードがリンク切れの inode に消えるため、書き込み時に
	// パスを開き直せていることを固定する。
	dir := t.TempDir()
	path := dir + "/audit.jsonl"

	lg, err := Open(path)
	if err != nil {
		t.Fatalf("Open がエラーを返した: %v", err)
	}
	if err := lg.Write(Record{Action: "svc.stop"}); err != nil {
		t.Fatalf("1 件目の Write がエラーを返した: %v", err)
	}

	rotated := path + ".1"
	if err := os.Rename(path, rotated); err != nil {
		t.Fatalf("ローテーションの再現に失敗した: %v", err)
	}

	if err := lg.Write(Record{Action: "svc.start"}); err != nil {
		t.Fatalf("2 件目の Write がエラーを返した: %v", err)
	}
	if err := lg.Close(); err != nil {
		t.Fatalf("Close がエラーを返した: %v", err)
	}

	current, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ローテーション後のファイルが読めない: %v", err)
	}
	if !strings.Contains(string(current), "svc.start") {
		t.Errorf("%s の内容 = %q, want svc.start を含む", path, current)
	}
	if strings.Contains(string(current), "svc.stop") {
		t.Errorf("%s に退避済みのレコードが混ざっている: %q", path, current)
	}

	old, err := os.ReadFile(rotated)
	if err != nil {
		t.Fatalf("退避したファイルが読めない: %v", err)
	}
	if !strings.Contains(string(old), "svc.stop") {
		t.Errorf("%s の内容 = %q, want svc.stop を含む", rotated, old)
	}
	if strings.Contains(string(old), "svc.start") {
		t.Errorf("ローテーション後のレコードが退避済みファイルに書かれている: %q", old)
	}

	if got := mode(t, path); got != 0o600 {
		t.Errorf("開き直したファイルのモード = %#o, want 0o600", got)
	}
}

func TestOpenReopensAfterUnlink(t *testing.T) {
	// logrotate の設定次第ではファイルが消えるだけの場合もある。
	path := t.TempDir() + "/audit.jsonl"

	lg, err := Open(path)
	if err != nil {
		t.Fatalf("Open がエラーを返した: %v", err)
	}
	defer func() { _ = lg.Close() }()

	if err := os.Remove(path); err != nil {
		t.Fatalf("削除に失敗した: %v", err)
	}
	if err := lg.Write(Record{Action: "svc.start"}); err != nil {
		t.Fatalf("Write がエラーを返した: %v", err)
	}

	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("再作成されたファイルが読めない: %v", err)
	}
	if !strings.Contains(string(b), "svc.start") {
		t.Errorf("内容 = %q, want svc.start を含む", b)
	}
}

func TestOpenRejectsRotationIntoForeignFile(t *testing.T) {
	// ローテーション直後は攻撃者が新しい audit.jsonl を先置きできる。開き直しでも
	// 初回と同じ検証を通すことを固定する。
	dir := t.TempDir()
	path := dir + "/audit.jsonl"

	lg, err := Open(path)
	if err != nil {
		t.Fatalf("Open がエラーを返した: %v", err)
	}
	defer func() { _ = lg.Close() }()

	if err := os.Rename(path, path+".1"); err != nil {
		t.Fatalf("ローテーションの再現に失敗した: %v", err)
	}
	planted := dir + "/planted"
	if err := os.WriteFile(planted, []byte("root:x:0:0:\n"), 0o644); err != nil {
		t.Fatalf("先置きファイルの作成に失敗した: %v", err)
	}
	if err := os.Symlink(planted, path); err != nil {
		t.Fatalf("シンボリックリンクの作成に失敗した: %v", err)
	}

	if err := lg.Write(Record{Action: "svc.start"}); err == nil {
		t.Fatal("先置きされたリンクへ書き込んでしまった")
	}
	b, err := os.ReadFile(planted)
	if err != nil {
		t.Fatalf("先置きファイルの読み込みに失敗した: %v", err)
	}
	if string(b) != "root:x:0:0:\n" {
		t.Errorf("先置きファイルの内容 = %q, want 変更なし", b)
	}
	if got := mode(t, planted); got != 0o644 {
		t.Errorf("先置きファイルのモード = %#o, want 0o644（変更してはいけない）", got)
	}
}
