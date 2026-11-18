package d2render

import (
	"context"
	"slices"
	"strings"
	"testing"
)

func TestWrap(t *testing.T) {
	cases := []struct {
		name  string
		text  string
		limit int
		want  []string
	}{
		{"short text stays on one line", "注文データの保存", 28, []string{"注文データの保存"}},
		{"empty text has no lines", "", 28, nil},
		{"full-width runes count two columns", "東京大阪", 4, []string{"東京", "大阪"}},
		{"half-width runes count one column", "abcd efgh", 8, []string{"abcd", "efgh"}},
		{"an ASCII word is never split", "Cloud Functions", 8, []string{"Cloud", "Functions"}},
		{"an ASCII word longer than the limit overflows whole", "Authentication", 6, []string{"Authentication"}},
		{"a katakana word is never split", "利用者のプロフィールを保存する", 16, []string{"利用者の", "プロフィールを", "保存する"}},
		{"closing punctuation never starts a line", "東京大阪、福岡札幌", 10, []string{"東京大阪、", "福岡札幌"}},
		{"an opening bracket stays with the text after it", "接続設定（未照合）", 12, []string{"接続設定", "（未照合）"}},
		{"breaks after a particle instead of inside a compound", "注文DBの接続設定（未照合）", 22, []string{"注文DBの接続設定", "（未照合）"}},
		{"takes an extra line to keep a compound whole", "確認結果の送信要求の最終報告", 14, []string{"確認結果の", "送信要求の", "最終報告"}},
		{"mixes Japanese and English", "注文確認メールの送信要求（Order Confirmation Request）", 22, []string{"注文確認メールの", "送信要求（Order", "Confirmation Request）"}},
		{"keeps lines even when breaks are equally good", "在庫を確かめて決済を進める", 20, []string{"在庫を確かめて", "決済を進める"}},
		{"an existing line break is kept", "一行目\n二行目", 28, []string{"一行目", "二行目"}},
		{"spaces between words survive on a line and vanish at a break", "Order History Database", 13, []string{"Order History", "Database"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := wrap(context.Background(), c.text, c.limit)
			if err != nil || !slices.Equal(got, c.want) {
				t.Errorf("wrap(%q, %d)\n got %q, %v\nwant %q", c.text, c.limit, got, err, c.want)
			}
		})
	}
}

func TestWrapKeepsEveryRune(t *testing.T) {
	text := "注文の配送状況を画面へ通知するという運用説明（Delivery Status Update）（説明・未照合）"
	unspaced := strings.ReplaceAll(text, " ", "")
	for limit := 4; limit <= 60; limit++ {
		lines, err := wrap(context.Background(), text, limit)
		if err != nil {
			t.Fatal(err)
		}
		if got := strings.ReplaceAll(strings.Join(lines, ""), " ", ""); got != unspaced {
			t.Fatalf("limit %d changed the text: %q", limit, lines)
		}
		for _, line := range lines {
			if strings.ContainsRune(noLineStart, []rune(line)[0]) {
				t.Errorf("limit %d: line %q starts with closing punctuation", limit, line)
			}
			if displayWidth(line) > limit && len(splitUnits(line)) > 1 {
				t.Errorf("limit %d: line %q is %d columns wide and could have been broken", limit, line, displayWidth(line))
			}
		}
	}
}
