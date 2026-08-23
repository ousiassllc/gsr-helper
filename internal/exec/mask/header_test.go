package mask

import (
	"slices"
	"strings"
	"testing"
)

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
