package mask

import (
	"slices"
	"strings"
	"testing"
)

func TestArgsByKey(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want []string
	}{
		{
			name: "--token の次の要素をマスクする",
			args: []string{"--url", "https://github.com/orgs/foo", "--token", "ABCDEFGH"},
			want: []string{"--url", "https://github.com/orgs/foo", "--token", "***"},
		},
		{
			name: "--pat もマスクする",
			args: []string{"--pat", "ghp_xxx"},
			want: []string{"--pat", "***"},
		},
		{
			name: "--jitconfig もマスクする",
			args: []string{"--jitconfig", "eyJ..."},
			want: []string{"--jitconfig", "***"},
		},
		{
			name: "大文字のキーもマスクする",
			args: []string{"--Token", "ABCDEFGH"},
			want: []string{"--Token", "***"},
		},
		{
			name: "単一ハイフンのキーもマスクする",
			args: []string{"-token", "ABCDEFGH"},
			want: []string{"-token", "***"},
		},
		{
			name: "= 区切りはキー名を残して値だけ置換する",
			args: []string{"--token=ABCDEFGH"},
			want: []string{"--token=***"},
		},
		{
			name: "= の後ろが空でも置換する",
			args: []string{"--token="},
			want: []string{"--token=***"},
		},
		{
			name: "最終要素がキーでも範囲外参照しない",
			args: []string{"--name", "build01", "--token"},
			want: []string{"--name", "build01", "--token"},
		},
		{
			name: "次がオプションに見えてもマスクする",
			args: []string{"--token", "--name", "build01"},
			want: []string{"--token", "***", "build01"},
		},
		{
			name: "同じキーが複数回あってもすべてマスクする",
			args: []string{"--token", "AAAAAAAA", "--token", "BBBBBBBB"},
			want: []string{"--token", "***", "--token", "***"},
		},
		{
			// gh auth token --hostname github.com の hostname を潰さない。
			name: "ハイフンなしの位置引数は次の要素をマスクしない",
			args: []string{"auth", "token", "--hostname", "github.com"},
			want: []string{"auth", "token", "--hostname", "github.com"},
		},
		{
			// gh secret set NAME のサブコマンドが読めなくならないようにする。
			name: "ハイフンなしのサブコマンド名は次の要素をマスクしない",
			args: []string{"secret", "set", "NAME"},
			want: []string{"secret", "set", "NAME"},
		},
		{
			// --name の値がたまたま token でも、その次の引数は無関係なので残す。
			name: "他のオプションの値がキー名と一致しても次の要素をマスクしない",
			args: []string{"--name", "token", "--labels", "self-hosted"},
			want: []string{"--name", "token", "--labels", "self-hosted"},
		},
		{
			// = 区切りはハイフンなしも拾う。右側が必ず値なので誤爆しない。
			name: "ハイフンなしの env 形式でも値だけ置換する",
			args: []string{"TOKEN=ghs_xxxxxxxx"},
			want: []string{"TOKEN=***"},
		},
		{
			// --token の次がもう一度 --token だった場合、飛ばすと 3 番目の
			// 本物の値が素のまま残る（監査ログへのトークン漏れ）。
			name: "キーが連続しても本物の値をマスクする",
			args: []string{"--token", "--token", "ABCDEFGH"},
			want: []string{"--token", "***", "***"},
		},
		{
			name: "別のキーが続いても本物の値をマスクする",
			args: []string{"--token", "--pat", "ABCDEFGH", "--name", "build01"},
			want: []string{"--token", "***", "***", "--name", "build01"},
		},
		{
			// 値が「秘密情報キーらしいオプション」の形をしていても、キー名から
			// マスクした値を素の文字列で書き戻してはならない。
			name: "秘密情報キーに見える値でもマスクを解除しない",
			args: []string{"--token", "--url=x-api-key"},
			want: []string{"--token", "***"},
		},
		{
			// -HAuthorization: ... のように 1 要素に密着した形。要素内の秘密情報を
			// マスクし、かつ次の要素（無関係な --url）を潰さない。
			name: "密着形のヘッダは自要素をマスクし次の要素を潰さない",
			args: []string{"api", "-HAuthorization: Bearer ABCDEFGH", "--url", "https://x"},
			want: []string{"api", "-HAuthorization: ***", "--url", "https://x"},
		},
		{
			name: "Authorization ヘッダはヘッダ名を残して値を置換する",
			args: []string{"api", "-H", "Authorization: Bearer ABCDEFGH"},
			want: []string{"api", "-H", "Authorization: ***"},
		},
		{
			// パス 1 は隣を見ずに要素単体で判定するため、-H が前置されていなくても
			// ":" の前がヘッダ名として秘密らしい要素はマスクされる
			// （docs/architecture/security.md「段 1 は単調な 2 パスで行う」）。
			name: "-H が無くてもヘッダ形の要素はマスクする",
			args: []string{"api", "Authorization: Bearer ABCDEFGH", "--url", "https://x"},
			want: []string{"api", "Authorization: ***", "--url", "https://x"},
		},
		{
			name: "--header= 形式のヘッダも値だけ置換する",
			args: []string{"--header=Authorization: token ABCDEFGH"},
			want: []string{"--header=Authorization: ***"},
		},
		{
			// -H "Accept: ..." を潰すと何を送ったのか追跡できなくなる。
			name: "秘密情報でないヘッダは変更しない",
			args: []string{"api", "-H", "Accept: application/vnd.github+json"},
			want: []string{"api", "-H", "Accept: application/vnd.github+json"},
		},
		{
			name: "部分一致のキーもマスクする",
			args: []string{"--api-key", "ABCDEFGH", "--authorization", "IJKLMNOP"},
			want: []string{"--api-key", "***", "--authorization", "***"},
		},
		{
			name: "対象外のキーは変更しない",
			args: []string{"--url", "https://github.com/orgs/foo", "--name", "build01", "--labels", "self-hosted,linux"},
			want: []string{"--url", "https://github.com/orgs/foo", "--name", "build01", "--labels", "self-hosted,linux"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Args(tt.args)
			if !slices.Equal(got, tt.want) {
				t.Errorf("Args() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestArgsByValue(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		secrets []string
		want    []string
	}{
		{
			name:    "完全一致を置換する",
			args:    []string{"--url", "https://github.com/orgs/foo", "-x", "supersecrettoken"},
			secrets: []string{"supersecrettoken"},
			want:    []string{"--url", "https://github.com/orgs/foo", "-x", "***"},
		},
		{
			name:    "URL に埋め込まれた値も置換する",
			args:    []string{"https://x-access-token:supersecrettoken@github.com/foo/bar.git"},
			secrets: []string{"supersecrettoken"},
			want:    []string{"https://x-access-token:***@github.com/foo/bar.git"},
		},
		{
			name:    "複数の secret を置換する",
			args:    []string{"aaaaaaaaaa", "bbbbbbbbbb", "cccccccccc"},
			secrets: []string{"aaaaaaaaaa", "cccccccccc"},
			want:    []string{"***", "bbbbbbbbbb", "***"},
		},
		{
			name:    "8 文字未満の secret は無視する",
			args:    []string{"--name", "build01", "abc"},
			secrets: []string{"abc", "build01"},
			want:    []string{"--name", "build01", "abc"},
		},
		{
			name:    "空文字の secret は無視する",
			args:    []string{"--name", "build01"},
			secrets: []string{""},
			want:    []string{"--name", "build01"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Args(tt.args, tt.secrets...)
			if !slices.Equal(got, tt.want) {
				t.Errorf("Args() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestArgsIsIdempotent(t *testing.T) {
	args := []string{"--token", "supersecrettoken", "--pat=supersecrettoken", "--url", "https://u:supersecrettoken@github.com"}
	secrets := []string{"supersecrettoken"}

	once := Args(args, secrets...)
	twice := Args(once, secrets...)
	if !slices.Equal(once, twice) {
		t.Errorf("二重適用で結果が変わった\n once: %q\ntwice: %q", once, twice)
	}
	for _, arg := range once {
		if arg == "supersecrettoken" {
			t.Errorf("マスク漏れがある: %q", once)
		}
	}
}

// secrets を渡さない経路（実行前プレビュー）でも冪等であることを確かめる。
// 段 1 は要素同士を見るため、二度目の適用でマスク済みの値をキーとして
// 読み直すと結果が変わり得る。
func TestArgsByKeyIsIdempotent(t *testing.T) {
	for _, args := range [][]string{
		{"--token", "ABCDEFGH"},
		{"--token", "--url=x-api-key"},
		{"api", "-HAuthorization: Bearer ABCDEFGH", "--url", "https://x"},
	} {
		once := Args(args)
		twice := Args(once)
		if !slices.Equal(once, twice) {
			t.Errorf("二重適用で結果が変わった\n once: %q\ntwice: %q", once, twice)
		}
	}
}

func TestArgsDoesNotModifyInput(t *testing.T) {
	args := []string{"--token", "supersecrettoken"}
	want := slices.Clone(args)

	Args(args, "supersecrettoken")
	if !slices.Equal(args, want) {
		t.Errorf("入力スライスが変更された: %q, want %q", args, want)
	}
}

func TestIsSecretKey(t *testing.T) {
	for _, name := range []string{
		"--token", "-token", "--Token", "--TOKEN", "pat", "--jitconfig", "--password", "--secret",
		"--authorization", "Authorization", "--api-key", "--api_key", "--apikey", "--bearer",
	} {
		if !isSecretKey(name) {
			t.Errorf("isSecretKey(%q) = false, want true", name)
		}
	}
	for _, name := range []string{"--url", "--name", "--labels", "", "--", "--tokens", "--runner-token"} {
		if isSecretKey(name) {
			t.Errorf("isSecretKey(%q) = true, want false", name)
		}
	}
}

// 段 1 のパス 1 は `strings.Cut(arg, "=")` を先に評価するため、`=` を含む
// ヘッダ 1 行が `KEY=VALUE` として処理され、`: ` から後ろの資格情報の本体が
// `=` の左に残っていた（Issue #62）。base64 のパディングを持つ
// `Authorization: Basic ...=` は現実的な入力であり、本体が監査ログと
// 実行前プレビューの両方へ素通りしていた。
//
// このテストは「マスク前の入力に資格情報の本体が含まれ、マスク後はどの要素にも
// 残っていない」ことを直接確かめる。期待値との一致だけを見ると、マスクの形が
// 変わったときに漏洩そのものを見落とすためである。
func TestArgsMasksCredentialInHeaderContainingEquals(t *testing.T) {
	tests := []struct {
		name string
		// credential はマスク後にどの要素にも残っていてはならない資格情報の本体。
		credential string
		args       []string
		want       []string
	}{
		{
			name:       "密着形 + base64 のパディング",
			credential: "dXNlcjpwYXNz",
			args:       []string{"api", "-HAuthorization: Basic dXNlcjpwYXNz="},
			want:       []string{"api", "-HAuthorization: ***"},
		},
		{
			name:       "密着形 + 値に = を含む Bearer",
			credential: "LEAKYTOKEN",
			args:       []string{"api", "-HAuthorization: Bearer LEAKYTOKEN=def"},
			want:       []string{"api", "-HAuthorization: ***"},
		},
		{
			name:       "-H 無し + base64 のパディング",
			credential: "dXNlcjpwYXNz",
			args:       []string{"api", "Authorization: Basic dXNlcjpwYXNz="},
			want:       []string{"api", "Authorization: ***"},
		},
		{
			name:       "-H 無し + 値に = を含む Bearer",
			credential: "LEAKYTOKEN",
			args:       []string{"api", "Authorization: Bearer LEAKYTOKEN=def"},
			want:       []string{"api", "Authorization: ***"},
		},
		{
			// 密着形の後ろに無関係な引数が続いても、パス 2 がそれを潰さない。
			name:       "密着形 + = を含んでも次の要素は潰さない",
			credential: "dXNlcjpwYXNz",
			args:       []string{"api", "-HAuthorization: Basic dXNlcjpwYXNz=", "--url", "https://x"},
			want:       []string{"api", "-HAuthorization: ***", "--url", "https://x"},
		},
		{
			// -H を別引数で前置した形はパス 2 の maskHeader が効くため元から
			// 正しくマスクされる。修正でこの経路が壊れないことの防壁。
			name:       "-H を別引数で前置した形",
			credential: "dXNlcjpwYXNz",
			args:       []string{"api", "-H", "Authorization: Basic dXNlcjpwYXNz="},
			want:       []string{"api", "-H", "Authorization: ***"},
		},
		{
			// --header= の右をヘッダ 1 行として扱う経路も同様の防壁。
			name:       "--header= 形式の値に = を含む",
			credential: "dXNlcjpwYXNz",
			args:       []string{"--header=Authorization: Basic dXNlcjpwYXNz="},
			want:       []string{"--header=Authorization: ***"},
		},
		{
			// ヘッダ形を先に見るようにしても、":" を含むだけの URL の
			// クエリパラメータが KEY=VALUE として拾われなくなってはならない
			// （マスクを弱める方向の退行を防ぐ）。
			name:       "URL のクエリに埋まった api_key はキー名で潰れたままである",
			credential: "SUPERSECRETVALUE",
			args:       []string{"api", "https://x.example/p?api_key=SUPERSECRETVALUE"},
			want:       []string{"api", "https://x.example/p?api_key=***"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !slices.ContainsFunc(tt.args, func(a string) bool { return strings.Contains(a, tt.credential) }) {
				t.Fatalf("テストの前提が壊れている: マスク前の %q に %q が含まれない", tt.args, tt.credential)
			}

			got := Args(tt.args)
			for _, arg := range got {
				if strings.Contains(arg, tt.credential) {
					t.Errorf("マスク後に資格情報が残っている: %q に %q が残存", got, tt.credential)
				}
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("Args() = %q, want %q", got, tt.want)
			}
			// 表示と記録の両方から呼ばれるため、二重適用でも結果が変わらない。
			if twice := Args(got); !slices.Equal(got, twice) {
				t.Errorf("二重適用で結果が変わった\n once: %q\ntwice: %q", got, twice)
			}
		})
	}
}
