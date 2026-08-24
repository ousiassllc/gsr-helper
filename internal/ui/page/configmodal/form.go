package configmodal

import (
	"charm.land/huh/v2"

	"github.com/ousiassllc/gsr-helper/internal/config/edit"
)

// 入力欄（huh.Field）の組み立て。入力先そのものは edit.Values が持つ
// （画面が huh を、ドメインが値と組み立てを受け持つ）。

// restartChoices は Restart= に選べる値。systemd の取り得る値のうち runner で
// 意味のあるものだけを出す。空は「本体の設定のまま」を表す。
var restartChoices = []string{"", "always", "on-failure", "no"}

// buildForm は種類に応じた huh のフォームを組み立てる。
func buildForm(v *edit.Values, theme huh.Theme) *huh.Form {
	return huh.NewForm(huh.NewGroup(fields(v)...)).WithTheme(theme).WithShowHelp(true)
}

// fields は種類ごとの入力欄を返す。
func fields(v *edit.Values) []huh.Field {
	switch v.Kind {
	case edit.KindEnv:
		return envFields(v)
	case edit.KindPath:
		return []huh.Field{
			huh.NewInput().Title(".path").
				Description("ジョブの PATH を上書きする 1 行。空なら .path を空にします").
				Value(&v.Path).Validate(edit.ValidateLine),
		}
	case edit.KindDropIn:
		return dropInFields(v)
	case edit.KindLabels:
		return []huh.Field{
			huh.NewInput().Title("ラベル").
				Description("カンマ区切り。self-hosted / linux / x64 は自動で付きます").
				Value(&v.Labels).Validate(edit.ValidateLabelInput),
		}
	case edit.KindGroup:
		return []huh.Field{groupField(v)}
	case edit.KindCopy:
		return []huh.Field{copyField(v)}
	case edit.KindSelf:
		return selfFields(v)
	case edit.KindReregister:
		return nil
	default:
		return nil
	}
}

// envFields は .env の入力欄を返す。
func envFields(v *edit.Values) []huh.Field {
	out := make([]huh.Field, 0, len(edit.EnvKeys))
	for i, k := range edit.EnvKeys {
		check := edit.ValidateLine
		if k.Hook {
			check = edit.ValidateHook
		}

		in := huh.NewInput().Title(k.Title).Value(&v.Env[i]).Validate(check)
		if k.Desc != "" {
			in = in.Description(k.Desc)
		}
		out = append(out, in)
	}
	return out
}

// dropInFields は drop-in の入力欄を返す。
func dropInFields(v *edit.Values) []huh.Field {
	opts := make([]huh.Option[string], 0, len(restartChoices))
	for _, c := range restartChoices {
		label := c
		if c == "" {
			label = "（本体の設定のまま）"
		}
		opts = append(opts, huh.NewOption(label, c))
	}

	return []huh.Field{
		huh.NewSelect[string]().Title("Restart").Options(opts...).Value(&v.Restart),
		huh.NewInput().Title("MemoryMax").
			Description("例: 4G。空なら設定しません").
			Value(&v.MemoryMax).Validate(edit.ValidateLine),
	}
}

// groupField は runner group の選択欄を返す。
//
// 一覧が空の場合は考えなくてよい。付け替えの API は ID を取るため名前を直接
// 入力させても必ず弾かれる（行き止まり）ので、一覧を取れなかった時点で
// フォームを開かないようにしてある（openGroupForm）。
func groupField(v *edit.Values) huh.Field {
	opts := make([]huh.Option[string], 0, len(v.Groups))
	for _, g := range v.Groups {
		opts = append(opts, huh.NewOption(g, g))
	}

	return huh.NewSelect[string]().Title("runner group").Options(opts...).Value(&v.Group)
}

// copyField は複製先の複数選択欄を返す（FR-40）。
func copyField(v *edit.Values) huh.Field {
	opts := make([]huh.Option[string], 0, len(v.CopyCandidates))
	for _, n := range v.CopyCandidates {
		opts = append(opts, huh.NewOption(n, n))
	}

	return huh.NewMultiSelect[string]().
		Title("複製先の runner").
		Description("選んだ runner の .env を、この runner の .env で置き換えます").
		Options(opts...).Value(&v.CopyTo)
}

// selfFields は自身の設定の入力欄を返す。
func selfFields(v *edit.Values) []huh.Field {
	s := &v.Self
	return []huh.Field{
		huh.NewInput().Title("追加の走査ルート").
			Description("カンマ区切りの絶対パス。空なら既定の場所だけを探します").
			Value(&s.ScanRoots).Validate(edit.ValidateRoots),
		huh.NewInput().Title("一覧の自動更新間隔（秒）").
			Description("1〜3600").Value(&s.Refresh).Validate(edit.ValidateRefresh),
		huh.NewInput().Title("ディスク使用率の警告閾値（%）").
			Description("1〜99").Value(&s.Warn).Validate(edit.ValidatePercent),
		huh.NewInput().Title("ディスク使用率の危険閾値（%）").
			Description("1〜100。警告より大きくします").Value(&s.Critical).Validate(edit.ValidatePercent),
		huh.NewInput().Title("監査ログの出力先").
			Description("絶対パス").Value(&s.AuditLog).Validate(edit.ValidateAuditLog),
	}
}
