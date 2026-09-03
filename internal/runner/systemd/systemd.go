// Package systemd は actions.runner.* ユニットの列挙と状態取得だけを担う。
//
// systemctl 参照はこのパッケージに閉じている。runner はディスク走査・/proc・systemd の
// 3 経路の結果を突き合わせる側であり、外部コマンドの発行と出力の解釈を混ぜると
// 突き合わせのロジックが systemctl の出力形式に引きずられる。分離しておけば
// systemctl の出力を扱うテストを Executor の Fake だけで完結させられる。
package systemd

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/ousiassllc/gsr-helper/internal/exec"
)

// unitPattern は svc.sh install が生成するユニット名のパターン。
// 実際の名前は actions.runner.<scope>.<runner名>.service になる。
const unitPattern = "actions.runner.*"

// actionDiscover は再検出の実行につける操作の識別子。SkipAudit で記録しないため
// 監査ログには現れないが、記録する側へ倒したときに正しい値が入るよう明示しておく。
const actionDiscover = "runner.discover"

// showConcurrency は systemctl show の同時実行数の上限。想定台数（20 台程度）を
// 3 秒ごとに参照するため直列では遅く、一方でプロセス生成は無制限に増やさない。
const showConcurrency = 8

// ErrListUnits は systemctl list-units 自体が失敗し、ユニットの一覧が得られな
// かったことを表す。
//
// Scan の警告は 1 ユニットの状態が取れなかった場合にも返るため、警告の有無だけでは
// 一覧そのものが取れたかを判断できない。一覧が取れていないことは「ユニットが
// 1 件も無い」と区別しなければならず（区別しないと systemd 管理の runner が
// run.sh 直起動に見える）、呼び出し側が errors.Is で判別できるようにする。
var ErrListUnits = errors.New("systemctl list-units の実行に失敗しました")

// State は systemd ユニットの状態。
type State struct {
	Unit       string
	Load       string // loaded / not-found
	Active     string // active / inactive / failed
	Sub        string // running / dead
	FileState  string // enabled / disabled / static
	WorkingDir string // svc.sh 生成のユニットには runner ディレクトリが入る
	User       string // User=。空なら root で起動する
	MainPID    int
}

// Scan は actions.runner.* の systemd ユニットとその状態を集める。
//
// ex が nil のときは systemd を参照せず何も返さない（systemctl が無い環境での縮退）。
// 警告も出さない。3 秒ごとのポーリングで同じ警告が積み上がるためであり、systemd の
// 可否は起動時に 1 回判定した Caps としてヘッダに出る。
//
// 警告は失敗したユニット単位に返す。1 ユニットの取得失敗で全体を止めず、
// 取れた分だけで一覧と孤児ユニットの判定を続ける。ただし list-units 自体が失敗した
// 場合は一覧が無いため ErrListUnits を包んだ警告 1 件だけを返す。呼び出し側は
// これを errors.Is で見分け、「ユニットが 0 件」と「一覧が取れていない」を
// 混同しないこと。
//
// 呼び出し側は deadline 付きの ctx を渡すこと。1 コマンドのタイムアウトは exec が
// 課すが、show は showConcurrency 件ずつのバッチで発行するため、全体の所要時間は
// 1 コマンドのタイムアウト × ceil(ユニット数/showConcurrency) まで伸びうる。
// Scan 自身は全体の上限を持たないので、3 秒ごとのポーリングが溜まらないよう
// 上限は ctx で与える。ctx がキャンセルされた時点で残りの show は発行しない。
func Scan(ctx context.Context, ex exec.Executor) ([]State, []error) {
	if ex == nil {
		return nil, nil
	}

	// 発行するのは list-units / show だけで、いずれもホストの状態を読むだけの
	// コマンドである。3 秒ごとの再検出でユニット数ぶん発行されるため、記録すると
	// 監査ログが読み取りで埋まり、破壊的操作のレコードが押し流される
	// （docs/architecture/security.md の「監査ログ」）。
	ctx = exec.WithOptions(ctx, exec.Options{Action: actionDiscover, SkipAudit: true})

	res, err := ex.Run(ctx, "systemctl",
		"list-units", "--type=service", "--all", "--plain", "--no-legend", "--no-pager",
		unitPattern)
	if ferr := runFailure(res, err); ferr != nil {
		return nil, []error{fmt.Errorf("%w: %w", ErrListUnits, ferr)}
	}

	units := parseListUnits(string(res.Stdout))
	// 書き込み先をインデックス指定にすることで、結果の順序が list-units の
	// 出力順で決まり、共有スライスへの排他も要らなくなる。
	states := make([]State, len(units))
	warns := make([]error, len(units))

	sem := make(chan struct{}, showConcurrency)
	var wg sync.WaitGroup
	issued := 0 // show を発行したユニット数。キャンセル時は途中で止まる
	for i, u := range units {
		if ctx.Err() != nil {
			// キャンセル後は残りの show を発行しない。発行しても全て失敗し、
			// ユニット数と同じ件数の同じ警告が積み上がるだけである。
			break
		}
		issued = i + 1
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer func() {
				<-sem
				wg.Done()
			}()
			st, err := showUnit(ctx, ex, u)
			if err != nil {
				// 取得できなかったユニットも一覧から落とさない。孤児ユニットの
				// 判定に必要なため、ユニット名だけのプレースホルダを残す。
				states[i] = State{Unit: u}
				warns[i] = err
				return
			}
			states[i] = st
		}()
	}
	wg.Wait()

	// 発行しなかった分は書き込まれないため、ゼロ値のユニットを返さないよう切り詰める。
	return states[:issued], compactErrors(warns[:issued])
}

// runFailure は Executor の実行結果から失敗の理由を返す。失敗していなければ nil。
// err と ExitCode の両方を見るのは、非ゼロ終了をエラーで返すか終了コードだけで
// 表すかが Executor の実装によって変わりうるため。
func runFailure(res exec.Result, err error) error {
	switch {
	case err != nil:
		return err
	case res.ExitCode != 0:
		return fmt.Errorf("終了コード %d で終了しました", res.ExitCode)
	default:
		return nil
	}
}

// compactErrors は nil を除いたエラーだけを返す。1 件も無ければ nil。
// errs は呼び出し元が作った作業用スライスなので、その場で詰める。
func compactErrors(errs []error) []error {
	out := slices.DeleteFunc(errs, func(e error) bool { return e == nil })
	if len(out) == 0 {
		return nil
	}
	return out
}

// parseListUnits は systemctl list-units の出力からユニット名を取り出す。
//
// --plain を付けても失敗ユニットの行頭に記号が付く場合があるため、
// 位置ではなく「actions.runner. で始まり .service で終わるフィールド」を探す。
func parseListUnits(out string) []string {
	var units []string
	for _, line := range strings.Split(out, "\n") {
		for _, f := range strings.Fields(line) {
			if strings.HasPrefix(f, "actions.runner.") && strings.HasSuffix(f, ".service") {
				units = append(units, f)
				break
			}
		}
	}
	return units
}

// showUnit は 1 ユニットの状態を systemctl show から取得する。
func showUnit(ctx context.Context, ex exec.Executor, unit string) (State, error) {
	res, err := ex.Run(ctx, "systemctl", "show", unit, "--no-pager",
		"-p", "Id", "-p", "LoadState", "-p", "ActiveState", "-p", "SubState",
		"-p", "UnitFileState", "-p", "WorkingDirectory", "-p", "MainPID", "-p", "User")
	if ferr := runFailure(res, err); ferr != nil {
		return State{}, fmt.Errorf("systemctl show %s の実行に失敗しました: %w", unit, ferr)
	}
	return parseShow(unit, string(res.Stdout)), nil
}

// parseShow は systemctl show の KEY=VALUE 出力をパースする。出力順は不定。
func parseShow(unit, out string) State {
	kv := map[string]string{}
	for _, line := range strings.Split(out, "\n") {
		k, v, ok := strings.Cut(strings.TrimSpace(line), "=")
		if ok {
			kv[k] = v
		}
	}

	st := State{
		Unit:      unit,
		Load:      kv["LoadState"],
		Active:    kv["ActiveState"],
		Sub:       kv["SubState"],
		FileState: kv["UnitFileState"],
		// WorkingDirectory= は "-/path"（存在しなければ無視する）の形を許容する。
		// 孤児判定の第二の照合キーなので、接頭辞を落として実パスに合わせる。
		WorkingDir: strings.TrimPrefix(kv["WorkingDirectory"], "-"),
		User:       kv["User"],
	}
	if id := kv["Id"]; id != "" {
		st.Unit = id
	}
	if pid, err := strconv.Atoi(kv["MainPID"]); err == nil {
		st.MainPID = pid
	}
	return st
}
