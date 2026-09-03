package limit

import "testing"

func TestBufferRetainsLastBytes(t *testing.T) {
	// 小さな書き込みの積み上げでも 1 回の巨大な書き込みでも、残るのは末尾の
	// limit バイトだけで、保持量が limit を超えない。
	b := NewBuffer(8)
	for i := range 10 {
		if _, err := b.Write([]byte{byte('0' + i)}); err != nil {
			t.Fatalf("Write が失敗した: %v", err)
		}
	}
	if got := b.String(); got != "23456789" {
		t.Errorf("小さな書き込みの積み上げ = %q, want %q", got, "23456789")
	}

	if n, err := b.Write([]byte("ABCDEFGHIJKL")); n != 12 || err != nil {
		t.Fatalf("Write = (%d, %v), want (12, nil)", n, err)
	}
	if got := b.String(); got != "EFGHIJKL" {
		t.Errorf("上限を 1 回で埋める書き込み = %q, want %q", got, "EFGHIJKL")
	}

	zero := NewBuffer(0)
	if n, err := zero.Write([]byte("x")); n != 1 || err != nil || len(zero.Bytes()) != 0 {
		t.Errorf("limit 0 で (%d, %v, %d バイト保持), want (1, nil, 0 バイト)", n, err, len(zero.Bytes()))
	}
}
