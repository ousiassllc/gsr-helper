package gh

import "sync"

// Secrets はマスク対象の秘密文字列をメモリ上だけで保持する。
//
// internal/exec/command.New の secrets 引数（値一致マスクの提供元）にそのまま渡せる
// よう Values をメソッド値として使う。保持するのは gh のトークンと、登録・登録解除の
// 短命トークンである。これらが監査ログや ExitError の stderr に平文で載るのを防ぐ。
//
// 制約（command.New の契約）:
//   - 並行安全であること。Run のたびに呼ばれる。
//   - 外部コマンドを起動しないこと。起動すると gh auth token が無限再帰する。
//
// どちらも満たすため、ここでは保持済みの値を複製して返すだけにしてある。
type Secrets struct {
	mu   sync.RWMutex
	vals map[string]struct{}
}

// NewSecrets は空の Secrets を作る。
func NewSecrets() *Secrets {
	return &Secrets{mu: sync.RWMutex{}, vals: make(map[string]struct{})}
}

// Add はマスク対象を追加する。空文字は無視する。
//
// 短すぎる値は internal/exec/mask 側で無視される（誤爆を避けるため）。ここでは
// 長さを問わず受け取り、マスクの判断は mask に委ねる。
func (s *Secrets) Add(v string) {
	if s == nil || v == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.vals[v] = struct{}{}
}

// Forget はマスク対象から取り除く。使い終わった短命トークンを溜めないために使う。
func (s *Secrets) Forget(v string) {
	if s == nil || v == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.vals, v)
}

// Values は現在のマスク対象を返す。command.New の secrets に渡す。
//
// nil レシーバでも nil を返す（未初期化のまま渡しても落ちない）。
func (s *Secrets) Values() []string {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]string, 0, len(s.vals))
	for v := range s.vals {
		out = append(out, v)
	}
	return out
}
