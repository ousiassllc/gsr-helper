package molecule

import (
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/ousiassllc/gsr-helper/internal/ui/token"
)

const (
	// commandIndent はコマンド 1 本目の行の字下げ。見出し（「実行するコマンド:」）の
	// 下にぶら下がって見えるようにする（screens.md の確認ダイアログ）。
	commandIndent = "  "
	// commandContinue は折り返した行の字下げ。1 本目より深くするのは、
	// 1 行に収まらなかった続きなのか次のコマンドなのかを字下げで読み分けるためである。
	commandContinue = "    "
)

// CommandBlock は確認ダイアログの「実行するコマンド」ブロックを組み立てる。
//
// 1 コマンド 1 行で字下げして並べ、幅に収まらない行は折り返して継続行を深く字下げする。
// 空の要素は飛ばす（字下げだけの空行は、コマンドの区切りに見えて誤読を招く）。
// commands が空なら空文字を返すので、呼び出し側は見出しごと省ける。
//
// **マスク済みの文字列を受け取るだけであり、マスクはしない。** トークンのマスクは
// exec 層の責務である（atomic-design.md の molecule 一覧、security.md）。UI 側にも
// 実装すると、どちらかが漏れたときに気付けない。
//
// 折り返しの幅計算を lipgloss に任せるのは、幅を数える実装を増やさないためである
// （atom の doc）。行末の余白は落とす。lipgloss は指定幅まで空白で埋めるが、
// ダイアログの本文では末尾の空白が選択・コピーのときに紛れ込む。
//
// コマンドを薄く描くのは、判断の材料が対象（Targets）と影響（Impact）であり、
// コマンドはその裏取りだからである。ここを強調すると影響の警告色と競合する。
func CommandBlock(commands []string, width int, s token.Styles) string {
	// 継続行の字下げを引いた幅で折り返す。1 本目（浅い字下げ）にも必ず収まる。
	body := max(width-len(commandContinue), 1)

	lines := make([]string, 0, len(commands))
	for _, cmd := range commands {
		if cmd == "" {
			continue
		}
		for i, l := range wrapCommand(cmd, body) {
			indent := commandIndent
			if i > 0 {
				indent = commandContinue
			}
			lines = append(lines, s.Muted.Render(indent+l))
		}
	}
	return strings.Join(lines, "\n")
}

// wrapCommand は 1 本のコマンドを幅に収まる行へ分ける。
//
// 語の切れ目を優先し、切れ目の無い長い文字列（パスなど）は幅で分割する。どちらも
// lipgloss の折り返しに委ねる。行末の余白（lipgloss が幅まで埋める空白）は落とす。
func wrapCommand(cmd string, width int) []string {
	wrapped := lipgloss.NewStyle().Width(width).Render(cmd)

	lines := strings.Split(wrapped, "\n")
	for i, l := range lines {
		lines[i] = strings.TrimRight(l, " ")
	}
	return lines
}
