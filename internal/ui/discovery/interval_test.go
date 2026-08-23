package discovery

import (
	"testing"
	"time"
)

// Interval はフラグ → 設定ファイル → 既定値の順に採り、下限（MinRefresh）を割らない。

// フラグが指定されていれば、設定ファイルの値より優先する。
func TestIntervalPrefersFlagOverConfig(t *testing.T) {
	const flag, conf = 5 * time.Second, 10 * time.Second
	if got := Interval(flag, conf); got != flag {
		t.Errorf("Interval(%v, %v) = %v, want %v", flag, conf, got, flag)
	}
}

// フラグが未指定（0 以下）なら設定ファイルの値を使う。
func TestIntervalFallsBackToConfig(t *testing.T) {
	const conf = 10 * time.Second
	if got := Interval(0, conf); got != conf {
		t.Errorf("Interval(0, %v) = %v, want %v", conf, got, conf)
	}
}

// フラグにも設定ファイルにも値が無ければ既定値を使う。
func TestIntervalFallsBackToDefault(t *testing.T) {
	if got := Interval(0, 0); got != DefaultRefresh {
		t.Errorf("Interval(0, 0) = %v, want %v（既定値）", got, DefaultRefresh)
	}
}

// 下限（MinRefresh）より短い指定は切り上げる。1 秒より短い間隔は画面の更新として
// 意味が無く、検出の Cmd を無駄に発行し続けるだけになるためである。
func TestIntervalClampsToMinimum(t *testing.T) {
	if got := Interval(1, 0); got != MinRefresh {
		t.Errorf("Interval(1ns, 0) = %v, want %v（下限）", got, MinRefresh)
	}
}

// 検出の deadline（Budget）は自動更新間隔から切り離す。
//
// 間隔と同値だと --refresh 1 のような短い指定でほぼ毎周期が期限切れになり、
// runner.Discover が部分結果しか返せなくなる（Budget の doc）。
func TestBudgetIsDecoupledFromInterval(t *testing.T) {
	if got := Interval(MinRefresh, 0); Budget <= got {
		t.Errorf("Budget（%v）が自動更新間隔（%v）以下になっている", Budget, got)
	}
}
