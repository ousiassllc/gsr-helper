package edit

import (
	"errors"

	"github.com/ousiassllc/gsr-helper/internal/runner"
)

// Values はフォームの入力先（データだけ）。
//
// **実体を画面が持ち続ける。** huh はポインタで値を束縛するため、Update のたびに
// 写しを作ると入力の書き込み先と読み出し先が別物になる（page/setup と同じ）。
// 入力欄そのものの組み立て（huh.Field）は画面側にあり、ここは「何を入れる箱が
// あるか」「その箱から何を組み立てるか」だけを持つ。
type Values struct {
	// Kind は今どの項目を編集しているか。
	Kind Kind
	// Env は EnvKeys と同じ添字で並ぶ入力欄の値。
	Env []string
	// EnvBefore は開いた時点の値。空欄にした項目を「消す」と判断するために持つ。
	EnvBefore []string
	// Path は .path の入力。
	Path string
	// Restart / MemoryMax は drop-in の入力。
	Restart   string
	MemoryMax string
	// Labels はカンマ区切りのラベルの入力。
	Labels string
	// LabelsBefore は取得した現在のカスタムラベル。差分の before に使う。
	//
	// **フォームが書き換える Labels とは別に持つ。** 同じ箱を使うと、確定した
	// 時点で before が after に上書きされ、差分が常に空になる。
	LabelsBefore []string
	// Group は選ばれた runner group の名前。
	Group string
	// GroupBefore はフォームを開いた時点で選ばれていた runner group。
	GroupBefore string
	// Groups は選べる runner group の名前。API から取る。
	Groups []string
	// GroupIDs は Groups と同じ添字で並ぶ runner group の ID。
	// API は名前ではなく ID を取るため、選ばれた名前から引き当てるために持つ。
	GroupIDs []int64
	// CopyTo は複製先に選ばれた runner 名（FR-40）。
	CopyTo []string
	// CopyCandidates は複製先の候補に出す runner 名。
	CopyCandidates []string
	// Self は本ツール自身の設定の入力先（FR-41〜FR-42）。
	Self SelfValues
}

// NewValues は空の入力の受け皿を作る。
func NewValues() *Values {
	return &Values{
		Kind: KindEnv, Env: make([]string, len(EnvKeys)), EnvBefore: make([]string, len(EnvKeys)),
		Path: "", Restart: "", MemoryMax: "",
		Labels: "", LabelsBefore: nil, Group: "", GroupBefore: "",
		Groups: nil, GroupIDs: nil, CopyTo: nil, CopyCandidates: nil,
		Self: SelfValues{ScanRoots: "", Refresh: "", Warn: "", Critical: "", AuditLog: ""},
	}
}

// ErrUnknownGroup は一覧に無い runner group を指定した場合のエラー。
var ErrUnknownGroup = errors.New("runner group の ID が分からないため変更できません")

// GroupID は選ばれた runner group の名前から ID を引く。
func (v *Values) GroupID() (int64, bool) {
	for i, n := range v.Groups {
		if n == v.Group && i < len(v.GroupIDs) {
			return v.GroupIDs[i], true
		}
	}
	return 0, false
}

// Fill は現在の設定を読んでフォームの初期値を入れる。
//
// GitHub 側の値（ラベル / runner group）は取得の完了時に呼び出し側が入れる。
// ここが読むのはホスト内のファイルだけである。
func (v *Values) Fill(k Kind, ld Loader, r runner.Runner, others []runner.Runner) error {
	v.Kind = k

	switch k {
	case KindEnv:
		f, err := ld.Env(r)
		if err != nil {
			return err
		}
		for i, spec := range EnvKeys {
			cur, _ := f.Get(spec.Key)
			v.Env[i], v.EnvBefore[i] = cur, cur
		}
	case KindPath:
		p, err := ld.PathFile(r)
		if err != nil {
			return err
		}
		v.Path = p.Value
	case KindDropIn:
		d, err := ld.DropIn(r)
		if err != nil {
			return err
		}
		v.Restart, _ = d.Get("Restart")
		v.MemoryMax, _ = d.Get("MemoryMax")
	case KindCopy:
		v.CopyTo = nil
		v.CopyCandidates = Names(others)
	case KindLabels, KindGroup, KindReregister, KindSelf:
		return nil
	}
	return nil
}

// Build はフォームの入力から変更を組み立てる。
//
// **差分に出す内容と実際に書き込む内容を同じ値から作る**という約束（Change の
// doc）を守るため、画面ではなくここで組む。before の受け渡しを画面に任せると、
// 現在値を捨てたまま Build* を呼ぶ経路が生まれる。
func (v *Values) Build(ld Loader, r runner.Runner, others []runner.Runner) (Change, error) {
	switch v.Kind {
	case KindEnv:
		return BuildEnv(ld, r, v.Env, v.EnvBefore)
	case KindPath:
		return BuildPath(ld, r, v.Path)
	case KindDropIn:
		return BuildDropIn(ld, r, v.Restart, v.MemoryMax)
	case KindLabels:
		after, err := LabelList(v.Labels)
		if err != nil {
			return Change{}, err
		}
		return BuildLabels(r.Name(), v.LabelsBefore, after), nil
	case KindGroup:
		id, ok := v.GroupID()
		if !ok {
			return Change{}, ErrUnknownGroup
		}
		return BuildGroup(r.Name(), v.GroupBefore, v.Group, id), nil
	case KindCopy:
		return BuildCopy(ld, r, others, v.CopyTo)
	case KindSelf, KindReregister:
		return Change{}, ErrNoUnit
	default:
		return Change{}, ErrNoUnit
	}
}

// SetGroups は取得した runner group の一覧を入れ、開いた時点の選択を控える。
//
// **現在の group は GitHub の一覧 API では分からない。** せめて「開いたときの
// まま確定した」場合に付け替えの API を呼ばないよう、控えた値を差分の before に
// 使う。huh の Select は値が選択肢に無いと先頭を選ぶので、こちらも先頭に合わせる。
func (v *Values) SetGroups(names []string, ids []int64) {
	v.Groups, v.GroupIDs = names, ids

	found := false
	for _, n := range names {
		if n == v.Group {
			found = true
		}
	}
	if !found && len(names) > 0 {
		v.Group = names[0]
	}
	v.GroupBefore = v.Group
}

// OtherRunners は dir 以外の runner を返す（複製先の候補）。
func OtherRunners(all []runner.Runner, dir string) []runner.Runner {
	out := make([]runner.Runner, 0, len(all))
	for _, r := range all {
		if r.Dir != dir {
			out = append(out, r)
		}
	}
	return out
}

// ApplyTargets は反映（再起動）の対象を返す。
//
// **複製では複製元ではなく複製先を再起動する。** .env が書き換わったのは複製先で
// あり、複製元の設定は 1 バイトも変わっていない。取り違えると、変更が効いていない
// 複製先を放置したまま、無関係な複製元のジョブを（強制再起動なら）中断してしまう。
//
// systemd 管理下でない runner は落とす。再起動の手段が無く、反映方法を尋ねても
// 何も起こらないためである。
func (c Change) ApplyTargets(all []runner.Runner, src runner.Runner) []runner.Runner {
	cands := []runner.Runner{src}
	if c.Kind == KindCopy {
		cands = named(all, c.CopyNames())
	}

	out := make([]runner.Runner, 0, len(cands))
	for _, r := range cands {
		if r.UnitName != "" {
			out = append(out, r)
		}
	}
	return out
}

// named は名前で runner を引く。
func named(all []runner.Runner, names []string) []runner.Runner {
	out := make([]runner.Runner, 0, len(names))
	for _, r := range all {
		for _, n := range names {
			if r.Name() == n {
				out = append(out, r)
				break
			}
		}
	}
	return out
}
