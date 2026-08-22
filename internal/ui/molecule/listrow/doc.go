// Package listrow は一覧の 1 行を組み立てる molecule を集める。
//
// 中身は molecule と同じ約束に従う。bubbletea / bubbles を import せず、ドメインの
// 型も受け取らず、表示に必要な値だけを落とした構造体（RunnerView / JobView /
// OrphanView）とプリミティブだけを受け取る。返すのは列定義（token.Column）と同じ
// 順・同じ数のセルである。
//
// molecule 直下から分けているのは、**行ビルダだけが一覧タブの数に比例して増える**
// ためである。一覧を持つタブは行の組み立てをここへ 1 つ足す（依存の規則により
// molecule 以下しかドメイン型を落とした行を描けない）。ヘッダ・タブ行・フッタ・
// 操作リストのように画面全体で 1 つしかない molecule と同じディレクトリに置くと、
// タブが増えるたびに 1 ディレクトリ 2000 行の上限へ近づく（Issue #35）。
//
// 依存は一方向である。listrow は molecule（列の選択に使う Columns）を参照してよいが、
// molecule は listrow を参照しない。
package listrow
