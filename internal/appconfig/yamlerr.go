package appconfig

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// yamlUnknownField は yaml.v3 が未知のキーに対して出すメッセージ。
// 例: line 2: field scan_dept not found in type appconfig.Config
var yamlUnknownField = regexp.MustCompile(`^line (\d+): field (\S+) not found in type \S+$`)

// describeYAMLError は yaml.v3 のエラーを利用者に見せられる形に整える。
//
// 設定ファイルは手編集を想定した形式なので、最も出やすい「打ち間違いによる未知の
// キー」は日本語にし、Go の内部型名（appconfig.Config）を見せない。
// それ以外の文言（型不一致など yaml.v3 側の表現が多岐にわたるもの）はそのまま通す。
// 原文を落として「解析に失敗しました」だけにすると原因に辿り着けないためである。
func describeYAMLError(err error) error {
	var te *yaml.TypeError
	if !errors.As(err, &te) {
		return err
	}
	msgs := make([]string, 0, len(te.Errors))
	for _, m := range te.Errors {
		if g := yamlUnknownField.FindStringSubmatch(m); g != nil {
			msgs = append(msgs, fmt.Sprintf("不明なキーです: %s（%s 行目）", g[2], g[1]))
			continue
		}
		msgs = append(msgs, m)
	}
	return errors.New(strings.Join(msgs, "; "))
}
