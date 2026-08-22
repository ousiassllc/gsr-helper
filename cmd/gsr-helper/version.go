package main

import "runtime/debug"

// appName はツール名。使い方の表示とバージョン表示で使う。
const appName = "gsr-helper"

// unknownVersion はバージョンを特定できない場合の表記。
const unknownVersion = "(unknown)"

// develVersion はローカルの go build / go run が付けるモジュールのバージョン。
// バージョンとして意味を持たないため、埋め込みが無い場合と同じ扱いにする。
const develVersion = "(devel)"

// version はリリース時に ldflags で埋め込むバージョン。
//
//	go build -ldflags "-X main.version=1.2.3" ./cmd/gsr-helper
//
// 空のまま（go build / go install だけで作った場合）はビルド情報から補う。
var version = ""

// versionString はバージョン表記を返す。
func versionString() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		info = nil
	}
	return resolveVersion(version, info)
}

// resolveVersion は埋め込みのバージョンとビルド情報からバージョン表記を組む。
//
// ldflags の値を優先し、無ければモジュールのバージョン（go install で付く）を使う。
// どちらも無い場合は (devel) や空文字ではなく (unknown) と示す。ローカルの go build では
// ビルド情報の Main.Version が (devel) になるため、ここで除外して (unknown) に倒す。
// 純粋関数にして 4 通りの入力を単体で検証できるようにしている。
func resolveVersion(embedded string, info *debug.BuildInfo) string {
	switch {
	case embedded != "":
		return appName + " " + embedded
	case info != nil && info.Main.Version != "" && info.Main.Version != develVersion:
		return appName + " " + info.Main.Version
	default:
		return appName + " " + unknownVersion
	}
}
