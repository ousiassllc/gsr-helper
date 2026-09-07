package setup

import (
	"strconv"
	"strings"

	"charm.land/huh/v2"

	"github.com/ousiassllc/gsr-helper/internal/runner/scope"
	"github.com/ousiassllc/gsr-helper/internal/setup"
	"github.com/ousiassllc/gsr-helper/internal/setup/name"
	"github.com/ousiassllc/gsr-helper/internal/setup/valid"
	"github.com/ousiassllc/gsr-helper/internal/ui/page"
)

// formKindOf は追加フォームの種類。
type formKindOf int

const (
	// bulkForm は台数を指定した一括追加（FR-10）。
	bulkForm formKindOf = iota
	// wizardForm は 1 台ずつ個別に設定する追加（FR-12）。
	wizardForm
)

// title はフォームの見出しを返す。
func (k formKindOf) title() string {
	if k == wizardForm {
		return "runner の追加（1 台ずつ）"
	}
	return "runner の追加（一括）"
}

// formValues は入力の受け皿。
//
// **実体を Model が持ち続ける。** huh はポインタで値を束縛するため、Update の
// たびに写しを作ると入力が書き込まれる先と読み出す先が別物になる。
type formValues struct {
	kind        formKindOf
	url         string
	count       string
	namePrefix  string
	name        string
	labels      string
	workDir     string
	runnerGroup string
	installBase string
	ephemeral   bool
}

// newValues は設定ファイルの既定値を反映した入力の受け皿を作る。
func newValues(st page.StateMsg) *formValues {
	v := &formValues{
		kind: bulkForm, url: "", count: "1", namePrefix: "", name: "",
		labels: "", workDir: defaultWork, runnerGroup: "", installBase: "", ephemeral: false,
	}
	v.applyDefaults(st)
	return v
}

// defaultWork は work dir の既定値。
const defaultWork = "_work"

// applyDefaults は未入力の項目に設定ファイルとホスト名の既定を入れる。
//
// 既に入力された値は上書きしない。共有状態は 3 秒ごとに届くため、上書きすると
// 入力中の文字が消える。
func (v *formValues) applyDefaults(st page.StateMsg) {
	d := st.Setup.Defaults
	if v.installBase == "" {
		v.installBase = d.InstallBase
	}
	if v.namePrefix == "" {
		v.namePrefix = defaultPrefix(d.NamePrefix, st.Setup.Host)
	}
	if v.labels == "" {
		v.labels = strings.Join(d.Labels, ",")
	}
}

// defaultPrefix は名前の接頭辞の既定を返す。設定が空ならホスト名を使う（FR-11）。
func defaultPrefix(configured, host string) string {
	if configured != "" {
		return configured
	}
	return host
}

// reset は次に開くフォームの種類に合わせて入力を初期化する。
//
// **利用者が毎回入れる項目は必ず消す。** 受け皿は Model が持ち続ける 1 つきりで
// （formValues の doc）、閉じても値は残る。一方 spec() は種類に関わらず全項目を
// 読むため、消し忘れは「今のフォームに出ていないのに実行内容へ混ざる」形になる。
// runnerGroup がその例で、入力欄があるのは 1 台ずつのフォームだけなのに、中断して
// 一括追加を開くと画面に出ていない --runnergroup が黙って付いていた。url は両方に
// 出るので隠れはしないが、中断した入力の持ち越し自体が承認時の読み合わせを狂わせる
// ので同じ扱いにする。
//
// namePrefix / labels / installBase は消さない。設定ファイルの既定を持つ項目で、
// applyDefaults が空欄のときだけ入れ直す（applyDefaults の doc）。既定を書き換えて
// 使う運用（毎回同じラベルを付ける）で入力をやり直させる方が煩わしい。
func (v *formValues) reset(st page.StateMsg, kind formKindOf) {
	v.kind = kind
	v.url = ""
	v.count = "1"
	v.name = ""
	v.workDir = defaultWork
	v.runnerGroup = ""
	v.ephemeral = st.Setup.Defaults.Ephemeral
	v.applyDefaults(st)
}

// build は huh のフォームを組み立てる。
func (v *formValues) build(theme huh.Theme) *huh.Form {
	fields := []huh.Field{
		huh.NewInput().Title("登録先の URL").
			Description("例: https://github.com/orgs/foo").
			Value(&v.url).Validate(validateURL),
	}

	if v.kind == bulkForm {
		fields = append(fields,
			huh.NewInput().Title("台数").Value(&v.count).Validate(validateCount),
			huh.NewInput().Title("名前の接頭辞").
				Description("<接頭辞>-<連番> で命名します").
				Value(&v.namePrefix).Validate(validateName),
		)
	} else {
		fields = append(fields,
			huh.NewInput().Title("runner 名").Value(&v.name).Validate(validateName),
			huh.NewInput().Title("work dir").Value(&v.workDir).Validate(validateWork),
			huh.NewInput().Title("runner group").
				Description("空欄なら既定のグループを使います").
				Value(&v.runnerGroup).Validate(validateOptional),
		)
	}

	fields = append(fields,
		huh.NewInput().Title("ラベル").
			Description("カンマ区切り。self-hosted / linux / x64 は自動で付きます").
			Value(&v.labels).Validate(validateLabels),
		huh.NewInput().Title("インストール先").Value(&v.installBase).Validate(validateBase),
		huh.NewConfirm().Title("ephemeral（ジョブ 1 件で登録解除）").Value(&v.ephemeral),
	)

	return huh.NewForm(huh.NewGroup(fields...)).WithTheme(theme).WithShowHelp(true)
}

// spec は入力から追加の指定を組み立てる。
func (v *formValues) spec(st page.StateMsg) (setup.AddSpec, error) {
	sc, err := scope.Parse(v.url)
	if err != nil {
		return setup.AddSpec{}, err
	}

	count := 1
	prefix := v.name
	if v.kind == bulkForm {
		if count, err = strconv.Atoi(strings.TrimSpace(v.count)); err != nil {
			return setup.AddSpec{}, valid.ErrBadCount
		}
		prefix = v.namePrefix
	}

	existing := runnerNames(st)

	// 1 台ずつの追加は入力した名前をそのまま使う（FR-12）。一括追加は接頭辞と
	// 連番で命名する（FR-11）。
	var names []string
	work := defaultWork
	if v.kind == wizardForm {
		names, work = []string{v.name}, v.workDir
	}

	return setup.AddSpec{
		URL: v.url, Scope: sc, NamePrefix: prefix, Count: count,
		StartIndex: name.NextIndex(existing, prefix), Names: names,
		Labels: splitLabels(v.labels), WorkDir: work, RunnerGroup: v.runnerGroup,
		Ephemeral: v.ephemeral, DisableUpdate: false, InstallBase: v.installBase,
		RunAsUser: "", Version: "", Existing: existing, Busy: busyNames(st),
		Root: st.Caps.Root,
	}, nil
}

// splitLabels はカンマ区切りのラベルを分解する。
func splitLabels(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return strings.Split(s, ",")
}

// runnerNames は検出済みの runner 名を返す。
func runnerNames(st page.StateMsg) []string {
	out := make([]string, 0, len(st.Result.Runners))
	for _, r := range st.Result.Runners {
		out = append(out, r.Name())
	}
	return out
}

// busyNames はジョブ実行中の runner 名を返す。
func busyNames(st page.StateMsg) []string {
	out := make([]string, 0)
	for _, r := range st.Result.Runners {
		if r.Busy() {
			out = append(out, r.Name())
		}
	}
	return out
}

// 入力ごとの検証。ドメイン層（internal/setup/valid）へ委ね、UI で規則を持たない。
func validateURL(s string) error  { _, err := valid.URL(s); return err }
func validateBase(s string) error { _, err := valid.Dir("インストール先", s); return err }
func validateName(s string) error { return valid.Name(s, nil) }
func validateWork(s string) error { return validateOptional(s) }
func validateLabels(s string) error {
	_, err := valid.Labels(splitLabels(s))
	return err
}

// validateOptional は空欄を許しつつ先頭の - を拒む。
//
// - で始まる値は config.sh の引数としてオプションと解釈される
// （docs/architecture/security.md「オプションインジェクションを防ぐ」）。
func validateOptional(s string) error {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	if strings.HasPrefix(s, "-") {
		return valid.ErrLeadingDash
	}
	return nil
}

// validateCount は台数の入力を検証する。
func validateCount(s string) error {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return valid.ErrBadCount
	}
	return valid.Count(n)
}

// firstLine はエラー文言の 1 行目を返す。状態行は 1 行しか使えない。
func firstLine(s string) string {
	head, rest, found := strings.Cut(s, "\n")
	if found && rest != "" {
		return head + " …"
	}
	return head
}
