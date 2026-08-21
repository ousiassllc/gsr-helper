package runner

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// unitPattern は svc.sh install が生成するユニット名のパターン。
// 実際の名前は actions.runner.<scope>.<runner名>.service になる。
const unitPattern = "actions.runner.*"

// SvcState は systemd ユニットの状態。
type SvcState struct {
	Unit       string
	Load       string // loaded / not-found
	Active     string // active / inactive / failed
	Sub        string // running / dead
	FileState  string // enabled / disabled / static
	WorkingDir string // svc.sh 生成のユニットには runner ディレクトリが入る
	MainPID    int
}

// Label はテーブル表示用の状態ラベルを返す。
func (s SvcState) Label() string {
	if s.Active == "" {
		return "-"
	}
	if s.Sub != "" && s.Sub != s.Active {
		return s.Active + "/" + s.Sub
	}
	return s.Active
}

// SystemdAvailable は systemctl が使える環境かを返す。
func SystemdAvailable() bool {
	_, err := exec.LookPath("systemctl")
	return err == nil
}

// ScanUnits は actions.runner.* の systemd ユニットとその状態を集める。
// systemctl が無い環境では空スライスを返す（エラーにしない）。
func ScanUnits(ctx context.Context) ([]SvcState, error) {
	if !SystemdAvailable() {
		return nil, nil
	}

	out, err := exec.CommandContext(ctx, "systemctl",
		"list-units", "--type=service", "--all", "--plain", "--no-legend", "--no-pager",
		unitPattern).Output()
	if err != nil {
		return nil, fmt.Errorf("systemctl list-units の実行に失敗しました: %w", err)
	}

	units := parseListUnits(string(out))
	states := make([]SvcState, 0, len(units))
	for _, u := range units {
		st, err := showUnit(ctx, u)
		if err != nil {
			// 1 ユニットの取得失敗で全体を落とさない。
			states = append(states, SvcState{Unit: u})
			continue
		}
		states = append(states, st)
	}
	return states, nil
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
func showUnit(ctx context.Context, unit string) (SvcState, error) {
	//nolint:gosec // unit は parseListUnits が検証した actions.runner.*.service のみ。#3 で internal/exec.Executor 経由に置き換えて本抑制を除去する。
	out, err := exec.CommandContext(ctx, "systemctl", "show", unit, "--no-pager",
		"-p", "Id", "-p", "LoadState", "-p", "ActiveState", "-p", "SubState",
		"-p", "UnitFileState", "-p", "WorkingDirectory", "-p", "MainPID").Output()
	if err != nil {
		return SvcState{}, fmt.Errorf("systemctl show %s の実行に失敗しました: %w", unit, err)
	}
	return parseShow(unit, string(out)), nil
}

// parseShow は systemctl show の KEY=VALUE 出力をパースする。出力順は不定。
func parseShow(unit, out string) SvcState {
	kv := map[string]string{}
	for _, line := range strings.Split(out, "\n") {
		k, v, ok := strings.Cut(strings.TrimSpace(line), "=")
		if ok {
			kv[k] = v
		}
	}

	st := SvcState{
		Unit:       unit,
		Load:       kv["LoadState"],
		Active:     kv["ActiveState"],
		Sub:        kv["SubState"],
		FileState:  kv["UnitFileState"],
		WorkingDir: kv["WorkingDirectory"],
	}
	if id := kv["Id"]; id != "" {
		st.Unit = id
	}
	if pid, err := strconv.Atoi(kv["MainPID"]); err == nil {
		st.MainPID = pid
	}
	return st
}
