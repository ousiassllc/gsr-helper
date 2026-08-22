package logs

import (
	"regexp"
	"slices"

	dlogs "github.com/ousiassllc/gsr-helper/internal/logs"
)

// 本文へ渡す行の組み立てを集める。
//
// **入口を 2 つに分けてあるのが要点である。** applyLines は保持している行すべてから組み直し、
// pushLine は届いた 1 行だけを足す。追従中は 1 行ごとにここを通るので、その 1 行のために最大
// maxLines 行を絞り込み直し・装飾し直すと 1 行あたり O(maxLines) の文字列処理（正規表現の
// 突き合わせと lipgloss の Render）になり、出力の多いログでは購読のチャネル（lineBuffer）が
// 埋まって追従が滞る。

// filterRegexp は今のフィルタに対応する正規表現を返す（絞り込みが無ければ nil）。
//
// **解いた結果を持ち回るのが要点である。** regexp.Compile は絞り込みそのものより高く付き、1 行
// 届くたびに解き直す理由が無い。元になった文字列を添えて覚えておき、フィルタが変わったときだけ
// 解き直す。Model の初期値（filterSrc が空、filterRe が nil）は「フィルタ無し」を解いた結果そのもの
// なので初回は解き直さない。解けなかった理由（filterErr）もここで更新する。理由を状態行に出すのは
// これまで通り status の役目で、外から見えるふるまいは変えていない。
func (m *Model) filterRegexp() *regexp.Regexp {
	if s := m.body.Filter(); s != m.filterSrc {
		m.filterSrc, m.filterRe, m.filterErr = s, nil, nil
		if s != "" {
			m.filterRe, m.filterErr = regexp.Compile(s)
		}
	}
	return m.filterRe
}

// applyLines は保持している行すべてから本文を組み直す（全再構築の入口）。
//
// **絞り込みは素の行に対して行う。** 装飾済みの文字列に正規表現を当てると ANSI 列が一致に混ざる
// （pane.Log が突き合わせを持たない理由でもある）。通すのは組み直しが要るときだけで、フィルタの
// 確定・取消・解除（keys.go）と、対象の切り替え・前面への復帰で m.lines を捨てるとき（stream.go の
// open、keys.go の activate）が該当する。いずれも絞り込みの結果が丸ごと入れ替わる。1 行届いた
// だけなら pushLine を通す。
func (m *Model) applyLines() {
	re := m.filterRegexp()

	out := make([]string, 0, len(m.lines))
	for _, l := range m.lines {
		if re != nil && !re.MatchString(l.Text) {
			continue
		}
		out = append(out, m.styleLine(l))
	}
	m.styled = out
	m.body.SetContent(out)
}

// pushLine は届いた 1 行だけを絞り込み・装飾してキャッシュへ足す（増分の入口）。
func (m *Model) pushLine(l dlogs.Line) {
	re := m.filterRegexp()

	// 上限に達していれば appendLine が m.lines の先頭を落とす。キャッシュに載っているのは絞り
	// 込みを通った行だけなので、落ちる行が通っていたときに限りキャッシュの先頭も 1 行落とす。
	// 全再構築へ逃げないのは、上限に達したあと毎行 O(maxLines) に戻ってキャッシュを持つ意味が
	// 無くなるためである。切り落とし方は appendLine に揃えた（背後の配列を切る理由はその doc）。
	if len(m.lines) >= maxLines && (re == nil || re.MatchString(m.lines[0].Text)) {
		m.styled = slices.Clone(m.styled[1:])
	}
	m.lines = appendLine(m.lines, l)

	if re == nil || re.MatchString(l.Text) {
		// **ここは背後の配列を切らずに append してよい。** Model は値で複製されて回るが、書き込む
		// のは常に自分の len の位置、つまり複製元からは見えない位置だけで、既にある要素を書き換え
		// ることが無い。唯一の読み手である pane.Log.SetContent も受け取った時点で写しを取る。
		m.styled = append(m.styled, m.styleLine(l))
	}
	m.body.SetContent(m.styled)
}
