package exec

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"
)

// Call は Fake が記録した 1 回の呼び出し。
//
// Args は生の値であり、マスクしない。テストが発行コマンド列を完全一致で
// 検証できるようにするためである。
type Call struct {
	Name    string
	Args    []string
	Options Options
	// Deadline は呼び出し時点の ctx の deadline。HasDeadline が偽なら未設定。
	//
	// 「呼び出し側がタイムアウトを課しているか」を Fake だけで検証できるように
	// 記録している。タイムアウトを課すパッケージが記録用の Executor を
	// 別途書かずに済むようにするためである。
	Deadline    time.Time
	HasDeadline bool
}

// String はテスト失敗時に読みやすいコマンド行表記を返す。
func (c Call) String() string {
	return strings.Join(append([]string{c.Name}, c.Args...), " ")
}

// response は Push で積まれた 1 件の応答。
type response struct {
	res Result
	err error
}

// Fake はテスト用の Executor。発行されたコマンド列を記録し、あらかじめ
// 設定した結果を返す。プロセスは起動しない。
//
// mutex を持つため、値としてコピーせず必ずポインタで扱う。
type Fake struct {
	mu        sync.Mutex
	calls     []Call
	responses []response
	fn        func(name string, args []string) (Result, error)
}

// NewFake はテスト用の Executor を返す。
func NewFake() *Fake {
	return &Fake{}
}

// Run は呼び出しを記録し、設定された応答を返す。
//
// SetFunc が設定されていればそれを使い、無ければ Push で積んだ応答を FIFO で消費する。
// 積んだ応答を使い切った後はゼロ値の Result と nil を返す（成功として扱う）。
func (f *Fake) Run(ctx context.Context, name string, args ...string) (Result, error) {
	deadline, hasDeadline := ctx.Deadline()

	f.mu.Lock()
	f.calls = append(f.calls, Call{
		Name:        name,
		Args:        slices.Clone(args),
		Options:     OptionsFrom(ctx),
		Deadline:    deadline,
		HasDeadline: hasDeadline,
	})
	fn := f.fn
	var resp response
	if fn == nil && len(f.responses) > 0 {
		resp = f.responses[0]
		f.responses = f.responses[1:]
	}
	f.mu.Unlock()

	// 実実装と同じく ctx のキャンセルを尊重する。呼び出しの記録は先に済ませて
	// あるため、テストは「何を実行しようとしたか」を検証できる。
	if err := ctx.Err(); err != nil {
		return Result{ExitCode: -1}, fmt.Errorf("%s の実行がキャンセルされました: %w", name, err)
	}
	if fn != nil {
		return fn(name, slices.Clone(args))
	}
	return resp.res, resp.err
}

// Push は Run が返す応答を FIFO で積む。
func (f *Fake) Push(res Result, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.responses = append(f.responses, response{res: res, err: err})
}

// SetFunc は応答を関数で決める。設定されている間は Push で積んだ応答より優先する。
//
// コールバックは ctx を受け取らない。ctx の deadline は Call に記録されるため
// 検証はそちらで足り、シグネチャ変更は既存テスト全体に波及するためである。
// ctx に応じて応答を変えたくなった時点で別途検討する。
func (f *Fake) SetFunc(fn func(name string, args []string) (Result, error)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.fn = fn
}

// Calls は記録した呼び出しのコピーを返す。
// 返した値を書き換えても内部の記録には影響しない。
func (f *Fake) Calls() []Call {
	f.mu.Lock()
	defer f.mu.Unlock()

	out := make([]Call, len(f.calls))
	for i, c := range f.calls {
		c.Args = slices.Clone(c.Args)
		c.Options.Env = slices.Clone(c.Options.Env)
		out[i] = c
	}
	return out
}

// Reset は記録と設定した応答を破棄する。1 つの Fake を複数のケースで使い回すため。
func (f *Fake) Reset() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = nil
	f.responses = nil
	f.fn = nil
}
