package systemd

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strconv"
	"testing"

	"github.com/ousiassllc/gsr-helper/internal/exec"
)

// docs/api/external-interfaces.md に記載された発行コマンド。記載と 1 文字も変えずに
// 持ち、Scan が仕様どおりのコマンドを出すことを突き合わせる
// （ユニットパターンの引用符は仕様書のシェル表記なので取り除いてある）。
//
// これは仕様書の内容を写した定数であり、md を読んではいない。実装側が仕様から
// 逸脱した場合だけを検知する片方向のガードなので、仕様書のコマンドを変える場合は
// ここも手で直すこと（md をパースする案は書式の些細な変更で壊れるため採らない）。
const (
	wantListUnits = "systemctl list-units --type=service --all --plain --no-legend --no-pager actions.runner.*"
	wantShowFmt   = "systemctl show %s --no-pager -p Id -p LoadState -p ActiveState -p SubState -p UnitFileState -p WorkingDirectory -p MainPID -p User"
)

// listOutput は list-units の出力を組み立てる。
func listOutput(units ...string) string {
	var out string
	for _, u := range units {
		out += u + " loaded active running GitHub Actions Runner\n"
	}
	return out
}

// fakeSystemctl は list-units に units の一覧を返す Fake を作る。
// show は okUnits に含まれるユニットだけ成功させ、他は失敗させる。
func fakeSystemctl(units []string, okUnits ...string) *exec.Fake {
	f := exec.NewFake()
	f.SetFunc(func(_ string, args []string) (exec.Result, error) {
		if args[0] == "list-units" {
			return exec.Result{Stdout: []byte(listOutput(units...))}, nil
		}
		if !slices.Contains(okUnits, args[1]) {
			return exec.Result{ExitCode: 1}, errors.New("ユニットが見つかりません")
		}
		return exec.Result{Stdout: []byte("Id=" + args[1] + "\nLoadState=loaded\nActiveState=active\n")}, nil
	})
	return f
}

// showCalls は記録された show をコマンド行の昇順で返す。show は並列に発行される
// ため呼び出し順は決まらない。
func showCalls(calls []exec.Call) []string {
	var out []string
	for _, c := range calls[1:] {
		out = append(out, c.String())
	}
	sort.Strings(out)
	return out
}

var twoUnits = []string{"actions.runner.myorg.a.service", "actions.runner.myorg.b.service"}

func TestScanIssuesDocumentedCommands(t *testing.T) {
	f := fakeSystemctl(twoUnits, twoUnits...)

	if _, warns := Scan(context.Background(), f); len(warns) != 0 {
		t.Fatalf("warns = %v", warns)
	}
	calls := f.Calls()
	if len(calls) != 1+len(twoUnits) {
		t.Fatalf("呼び出し回数 = %d, want %d", len(calls), 1+len(twoUnits))
	}
	if calls[0].String() != wantListUnits {
		t.Errorf("list-units\n got: %s\nwant: %s", calls[0].String(), wantListUnits)
	}
	want := []string{fmt.Sprintf(wantShowFmt, twoUnits[0]), fmt.Sprintf(wantShowFmt, twoUnits[1])}
	if shows := showCalls(calls); !slices.Equal(shows, want) {
		t.Errorf("show\n got: %q\nwant: %q", shows, want)
	}
}

// systemctl を呼べない・呼んでも進まない縮退経路。
func TestScanDegraded(t *testing.T) {
	// systemctl が無い環境。3 秒ポーリングで同じ警告が積まれるため警告も出さない。
	if states, warns := Scan(context.Background(), nil); states != nil || warns != nil {
		t.Errorf("Executor 無し = (%v, %v), want (nil, nil)", states, warns)
	}
	// ユニットが 1 件も無ければ show を発行しない。
	f := fakeSystemctl(nil)
	if states, warns := Scan(context.Background(), f); len(states) != 0 || warns != nil || len(f.Calls()) != 1 {
		t.Errorf("ユニット 0 件 = (%v, %v)、呼び出し %d 回, want (空, nil)、1 回", states, warns, len(f.Calls()))
	}
	// キャンセル済みなら list-units の失敗として警告 1 件になる（panic しない）。
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if states, warns := Scan(ctx, fakeSystemctl(twoUnits[:1])); states != nil || len(warns) != 1 {
		t.Errorf("キャンセル済み = (%v, %v), want (nil, 警告 1 件)", states, warns)
	}
}

// list-units の成功後にキャンセルされた場合、残りの show を発行しないこと。
// 発行してしまうとユニット数と同じ件数の「キャンセルされました」警告が積まれる。
func TestScanCanceledAfterListUnits(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	f := exec.NewFake()
	f.SetFunc(func(_ string, args []string) (exec.Result, error) {
		if args[0] == "list-units" {
			cancel() // list-units は成功させ、その直後にキャンセルされた状況を作る
			return exec.Result{Stdout: []byte(listOutput(twoUnits...))}, nil
		}
		return exec.Result{Stdout: []byte("Id=" + args[1] + "\n")}, nil
	})

	states, warns := Scan(ctx, f)
	// show は 1 件も発行しない。「ユニット数より少ない」では 1 件だけ漏れた場合を
	// 見逃すため、キャンセル後の発行数はぴったり 0 件で見る。
	shows := len(f.Calls()) - 1
	if shows != 0 || len(states) != 0 || len(warns) != 0 {
		t.Errorf("show %d 件 / states %d 件 / warns %d 件, want いずれも 0 件",
			shows, len(states), len(warns))
	}
}

func TestScanListUnitsFailure(t *testing.T) {
	tests := []struct {
		name string
		res  exec.Result
		err  error
	}{
		{name: "エラーを返す", res: exec.Result{ExitCode: -1}, err: errors.New("systemctl が見つかりません")},
		// Executor が非ゼロ終了をエラーにしない実装でも失敗として扱う。
		{name: "終了コードのみ非ゼロ", res: exec.Result{ExitCode: 1}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := exec.NewFake()
			f.Push(tt.res, tt.err)

			states, warns := Scan(context.Background(), f)
			if states != nil || len(warns) != 1 || len(f.Calls()) != 1 {
				t.Errorf("got (%v, %v)、呼び出し %d 回, want (nil, 警告 1 件)、1 回（show を発行しない）",
					states, warns, len(f.Calls()))
			}
		})
	}
}

// 一覧が取れなかったこと（ErrListUnits）と 1 ユニットの状態が取れなかったことを
// 呼び出し側が見分けられること。混同すると systemd 管理の runner が
// 「ユニットが無い」= run.sh 直起動として扱われる。
func TestScanErrListUnits(t *testing.T) {
	f := exec.NewFake()
	f.Push(exec.Result{ExitCode: 1}, errors.New("systemctl が見つかりません"))
	_, warns := Scan(context.Background(), f)
	if len(warns) != 1 || !errors.Is(warns[0], ErrListUnits) {
		t.Fatalf("list-units 失敗の警告 = %v, want ErrListUnits を包む 1 件", warns)
	}

	// show の失敗は 1 ユニットの状態が不明なだけで、一覧は取れている。
	_, warns = Scan(context.Background(), fakeSystemctl(twoUnits[:1]))
	if len(warns) != 1 || errors.Is(warns[0], ErrListUnits) {
		t.Fatalf("show 失敗の警告 = %v, want ErrListUnits を包まない 1 件", warns)
	}
}

func TestScanShowFailure(t *testing.T) {
	tests := []struct {
		name      string
		okUnits   []string
		wantWarns int
	}{
		{name: "1 ユニットだけ失敗", okUnits: twoUnits[1:], wantWarns: 1},
		{name: "全ユニット失敗", okUnits: nil, wantWarns: 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			states, warns := Scan(context.Background(), fakeSystemctl(twoUnits, tt.okUnits...))
			if len(warns) != tt.wantWarns || len(states) != len(twoUnits) {
				t.Fatalf("warns = %v（want %d 件）, states = %v（want %d 件）",
					warns, tt.wantWarns, states, len(twoUnits))
			}
			for i, u := range twoUnits {
				// 失敗したユニットはユニット名だけのプレースホルダとして残す（孤児判定に必要）。
				want := State{Unit: u}
				if slices.Contains(tt.okUnits, u) {
					want = State{Unit: u, Load: "loaded", Active: "active"}
				}
				if states[i] != want {
					t.Errorf("states[%d] = %+v, want %+v", i, states[i], want)
				}
			}
		})
	}
}

func TestScanMoreThanConcurrencyLimit(t *testing.T) {
	units := make([]string, 0, 20)
	for i := range 20 {
		units = append(units, "actions.runner.myorg.host-"+strconv.Itoa(i)+".service")
	}
	f := fakeSystemctl(units, units...)

	states, warns := Scan(context.Background(), f)
	if len(warns) != 0 || len(states) != len(units) || len(f.Calls()) != 1+len(units) {
		t.Fatalf("warns = %v, states = %d 件, 呼び出し %d 回, want (nil, %d 件, %d 回)",
			warns, len(states), len(f.Calls()), len(units), 1+len(units))
	}
	// 並列に発行しても結果は list-units の出力順に並ぶこと（書き込み先のインデックス指定）。
	for i, u := range units {
		if states[i].Unit != u || states[i].Load != "loaded" {
			t.Errorf("states[%d] = %+v, want %s", i, states[i], u)
		}
	}
}
