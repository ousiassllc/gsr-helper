package gh

import (
	"errors"
	"fmt"
	"github.com/ousiassllc/gsr-helper/internal/gh/ghtoken"
	"net/http"
	"time"

	"github.com/google/go-github/v83/github"

	"github.com/ousiassllc/gsr-helper/internal/runner/scope"
)

// ErrUnknownScope はスコープを判定できない runner に対して API を呼ぼうとした場合のエラー。
var ErrUnknownScope = errors.New("runner の登録先（repo / org / enterprise）が判定できません")

// ErrNoToken はトークンを 1 つも取得できなかった場合のエラー。
//
// 実体は internal/gh/ghtoken にある（トークンの取得を分けたため）。ここで
// 別名を残すのは、errors.Is(err, gh.ErrNoToken) と書いている既存の判定を
// そのまま通すためである。
var ErrNoToken = ghtoken.ErrNoToken

// ErrNoDownload は対象の OS / アーキテクチャ向けの tarball が見つからない場合のエラー。
var ErrNoDownload = errors.New("この OS / アーキテクチャ向けの runner tarball が見つかりません")

// ErrNoVersion は最新バージョンのタグが空だった場合のエラー。
var ErrNoVersion = errors.New("runner の最新バージョンを判定できません")

// ErrNoRunnerGroups は repo スコープの runner に runner group を問い合わせた場合のエラー。
var ErrNoRunnerGroups = errors.New("runner group は org / enterprise スコープでのみ使えます")

// APIError は GitHub API の失敗を、利用者が次に何をすればよいかまで含めて表す。
//
// docs/api/external-interfaces.md の「レート制限とエラー」の表に対応する。
// 自動リトライはしない（レート制限を再消費しないため）。判断材料を Hint に載せ、
// 再試行するかどうかは利用者に委ねる。
type APIError struct {
	// Op は失敗した操作の名前（registration_token / list_runners など）。
	Op string
	// Scope は対象スコープの表示名。ホスト全体の操作では空。
	Scope string
	// Status は HTTP ステータス。取得できなかった場合は 0。
	Status int
	// Hint は次の一手（不足スコープ、待機時間、確認コマンド）。
	Hint string
	// Err は基のエラー。
	Err error
}

// Error はエラー文言を組み立てる。トークンは元から含まれない。
func (e *APIError) Error() string {
	msg := e.Op + " に失敗しました"
	if e.Scope != "" {
		msg += "（対象: " + e.Scope + "）"
	}
	if e.Status != 0 {
		msg += fmt.Sprintf(" [HTTP %d]", e.Status)
	}
	if e.Err != nil {
		msg += ": " + e.Err.Error()
	}
	if e.Hint != "" {
		msg += "。" + e.Hint
	}
	return msg
}

// Unwrap は errors.Is / errors.As のために基のエラーを返す。
func (e *APIError) Unwrap() error { return e.Err }

// wrap は go-github の戻り値を APIError に変換する。err が nil なら nil を返す。
func wrap(op string, sc scope.Scope, resp *github.Response, err error) error {
	if err == nil {
		return nil
	}

	status := 0
	if resp != nil && resp.Response != nil {
		status = resp.StatusCode
	}

	return &APIError{
		Op:     op,
		Scope:  scopeLabel(sc),
		Status: status,
		Hint:   hintFor(status, sc, resp, err),
		Err:    err,
	}
}

// scopeLabel は表示用のスコープ名を返す。Unknown は空にする。
func scopeLabel(sc scope.Scope) string {
	if sc.Kind == scope.Unknown {
		return ""
	}
	return sc.String()
}

// hintFor は状況に応じた次の一手を返す。
func hintFor(status int, sc scope.Scope, resp *github.Response, err error) string {
	var rateErr *github.RateLimitError
	if errors.As(err, &rateErr) {
		return rateLimitHint(rateErr.Rate.Reset.Time)
	}

	var abuseErr *github.AbuseRateLimitError
	if errors.As(err, &abuseErr) {
		return "GitHub から一時的に制限されています。時間をおいて再試行してください"
	}

	switch status {
	case http.StatusUnauthorized:
		return "トークンが無効です。gh auth status で認証状態を確認してください"
	case http.StatusForbidden:
		if resp != nil && resp.Rate.Remaining == 0 {
			return rateLimitHint(resp.Rate.Reset.Time)
		}
		return "権限が不足しています。" + refreshHint(sc)
	case http.StatusNotFound:
		return "スコープの指定が誤っているか、権限不足で存在が隠されています。" + refreshHint(sc)
	default:
		return ""
	}
}

// rateLimitHint はレート制限の解除時刻から待機時間を組み立てる。
func rateLimitHint(reset time.Time) string {
	if reset.IsZero() {
		return "レート制限に達しました。時間をおいて再試行してください"
	}
	return fmt.Sprintf(
		"レート制限に達しました。%s（あと約 %d 分）まで待って再試行してください",
		reset.Local().Format("15:04"),
		minutesUntil(reset),
	)
}

// minutesUntil は now から reset までの分数を切り上げで返す。過去なら 0。
func minutesUntil(reset time.Time) int {
	d := time.Until(reset)
	if d <= 0 {
		return 0
	}
	return int((d + time.Minute - 1) / time.Minute)
}

// refreshHint は不足しているスコープを補うコマンド例を返す。
//
// docs/api/external-interfaces.md の「必要なトークンスコープ」の表に対応する。
func refreshHint(sc scope.Scope) string {
	need := RequiredScope(sc)
	if need == "" {
		return "必要な権限は docs/api/external-interfaces.md を参照してください"
	}
	return "必要なスコープは " + need + " です（例: gh auth refresh -h github.com -s " + need + "）"
}
