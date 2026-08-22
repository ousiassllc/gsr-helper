package exec

import "context"

// Options は 1 回の実行ごとに変わるパラメータ。
//
// Executor は仕様上 Run 1 メソッドに固定され、プロジェクト全体で interface を
// 増やさない規則があるため、これらは引数ではなく ctx に載せて渡す。
// Action / Runner は「どの操作の一部としての実行か」という request-scoped な
// メタ情報でもあり、ctx に載せる対象として素直である。
type Options struct {
	// Action は監査ログの action（svc.stop / runner.add など）。
	Action string
	// Runner は対象 runner 名。ホスト全体の操作では空。
	Runner string
	// Dir は作業ディレクトリ。監査ログの dir にもこの値を記録する。
	Dir string
	// Env は追加の環境変数（KEY=VALUE）。既存の環境に足す。
	Env []string
	// SkipAudit はこの実行を監査ログに記録しない指定。
	//
	// **既定は false（記録する）。** 記録漏れが既定にならないよう、記録しないこと
	// を呼び出し側の明示的な意思表示に限る。
	//
	// 使ってよいのは**利用者の操作を伴わない読み取り専用の定期実行**だけである
	// （Runners タブの再検出が 3 秒ごとに発行する systemctl list-units / show）。
	// 破壊的操作（svc.* / runner.* / disk.clean）には決して使わない。理由は
	// docs/architecture/security.md の「監査ログ」を参照。
	SkipAudit bool
}

// optionsKey は Options を ctx に格納するキー。
// 非公開型にして他パッケージのキーと衝突しないようにする（go vet SA1029）。
type optionsKey struct{}

// WithOptions は o を載せた派生 ctx を返す。
func WithOptions(ctx context.Context, o Options) context.Context {
	return context.WithValue(ctx, optionsKey{}, o)
}

// OptionsFrom は ctx に載っている Options を取り出す。未設定ならゼロ値を返す。
//
// 未設定を異常としないのは、監査ログのメタ情報が無くても実行自体は成立させたいためである
// （action が空のレコードとして残り、記録漏れにはならない）。
func OptionsFrom(ctx context.Context) Options {
	o, ok := ctx.Value(optionsKey{}).(Options)
	if !ok {
		return Options{}
	}
	return o
}
