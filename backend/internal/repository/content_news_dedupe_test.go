package repository

import "testing"

func TestNewsTitleSimilarity(t *testing.T) {
	if got := newsTitleSimilarity("OpenAI 发布新模型 GPT 5", "OpenAI 发布新模型 GPT 5"); got != 1 {
		t.Fatalf("exact similarity=%v", got)
	}
	if got := newsTitleSimilarity("OpenAI 发布新模型 GPT 5", "OpenAI 发布新模型 GPT 5 重大更新"); got < 0.7 {
		t.Fatalf("near duplicate similarity=%v", got)
	}
	if got := newsTitleSimilarity("OpenAI 发布新模型", "欧洲央行公布利率决定"); got >= 0.3 {
		t.Fatalf("unrelated similarity=%v", got)
	}
}
