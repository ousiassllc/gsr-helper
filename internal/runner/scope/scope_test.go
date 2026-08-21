package scope

import "testing"

func TestParse(t *testing.T) {
	tests := []struct {
		name, in string
		want     Scope
		wantErr  bool
	}{
		{name: "repo", in: "https://github.com/foo/bar", want: Scope{Kind: Repo, Owner: "foo", Repo: "bar"}},
		{name: "repo 末尾スラッシュ", in: "https://github.com/foo/bar/", want: Scope{Kind: Repo, Owner: "foo", Repo: "bar"}},
		{name: "repo 余剰要素は無視", in: "https://github.com/foo/bar/settings/actions", want: Scope{Kind: Repo, Owner: "foo", Repo: "bar"}},
		{name: "org（orgs 形式）", in: "https://github.com/orgs/foo", want: Scope{Kind: Org, Owner: "foo"}},
		{name: "org（1 要素形式）", in: "https://github.com/foo", want: Scope{Kind: Org, Owner: "foo"}},
		{name: "enterprise", in: "https://github.com/enterprises/foo", want: Scope{Kind: Enterprise, Owner: "foo"}},
		{name: "GHES の org", in: "https://ghe.example.com/orgs/foo", want: Scope{Kind: Org, Owner: "foo"}},
		{name: "余分な空白は無視", in: "  https://github.com/foo/bar  ", want: Scope{Kind: Repo, Owner: "foo", Repo: "bar"}},
		{name: "連続スラッシュ", in: "//foo//bar//", want: Scope{Kind: Org, Owner: "bar"}},
		{name: "パスが空", in: "https://github.com", wantErr: true},
		{name: "空文字", in: "", wantErr: true},
		{name: "空白のみ", in: "   ", wantErr: true},
		{name: "スキーム不正", in: "://bad", wantErr: true},
		// 以下は回帰テスト。いずれも誤ったスコープを返していた。
		{name: "orgs 単独", in: "https://github.com/orgs", wantErr: true},
		{name: "enterprises 単独", in: "https://github.com/enterprises", wantErr: true},
		{name: "スキーム無し", in: "github.com/foo/bar", wantErr: true},
		{name: "URL でない文字列", in: "not a url", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Parse(tt.in)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr = %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestScopeString(t *testing.T) {
	tests := []struct {
		name  string
		scope Scope
		want  string
	}{
		{name: "repo", scope: Scope{Kind: Repo, Owner: "foo", Repo: "bar"}, want: "foo/bar"},
		{name: "org", scope: Scope{Kind: Org, Owner: "foo"}, want: "org:foo"},
		{name: "enterprise", scope: Scope{Kind: Enterprise, Owner: "foo"}, want: "ent:foo"},
		{name: "unknown", scope: Scope{}, want: "-"},
		{name: "範囲外の Kind", scope: Scope{Kind: Kind(99), Owner: "foo"}, want: "-"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.scope.String(); got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}
