// internal/memory/search.go

package memory

import (
	"sort"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// SearchResult is a search hit with a normalized match score.
type SearchResult struct {
	Entry
	Rank float64 // 토큰별 점수 합 / 전체 토큰 수 (1.0 = 모든 토큰이 부분 문자열로 일치)
}

// fuzzyTokenScore: 첫 bigram만 일치한 토큰의 점수. 정확 일치(1.0)보다 낮게 둬 순위에서 밀린다.
const fuzzyTokenScore = 0.5

// Search scores entries by case-insensitive substring containment per query
// token over key, category and content (양쪽 NFC 정규화). 부분 문자열로 안 맞는
// 한글 토큰은 첫 두 글자(첫 bigram)가 있으면 약하게 일치로 본다 — 한국어 조사·어미는
// 뒤에 붙으므로 "인증을"·"세션으로"가 "인증은"·"세션"과 맞고, "하기로"는 "쓰기로"와 맞지 않는다.
func (s *Store) Search(project, query string, limit int) ([]SearchResult, error) {
	if limit <= 0 {
		limit = 10
	}
	tokens := strings.Fields(strings.ToLower(norm.NFC.String(query)))
	if len(tokens) == 0 {
		return nil, nil
	}
	entries, err := s.List(project)
	if err != nil {
		return nil, err
	}
	var results []SearchResult
	for _, e := range entries {
		text := strings.ToLower(norm.NFC.String(e.Key + "\n" + e.Category + "\n" + e.Content))
		score := 0.0
		for _, tok := range tokens {
			score += tokenScore(text, tok)
		}
		if score == 0 {
			continue
		}
		results = append(results, SearchResult{Entry: e, Rank: score / float64(len(tokens))})
	}
	sort.Slice(results, func(i, j int) bool {
		if results[i].Rank != results[j].Rank {
			return results[i].Rank > results[j].Rank
		}
		if results[i].Confidence != results[j].Confidence {
			return results[i].Confidence > results[j].Confidence
		}
		return results[i].CreatedAt.After(results[j].CreatedAt)
	})
	if len(results) > limit {
		results = results[:limit]
	}
	return results, nil
}

// tokenScore returns 1 for a substring match, fuzzyTokenScore when a Hangul
// token's first bigram appears, 0 otherwise. 영문 토큰은 노이즈를 피하려 폴백하지 않는다.
func tokenScore(text, tok string) float64 {
	if strings.Contains(text, tok) {
		return 1
	}
	runes := []rune(tok)
	if len(runes) < 3 || !containsHangul(runes) {
		return 0
	}
	if strings.Contains(text, string(runes[:2])) {
		return fuzzyTokenScore
	}
	return 0
}

func containsHangul(runes []rune) bool {
	for _, r := range runes {
		if unicode.Is(unicode.Hangul, r) {
			return true
		}
	}
	return false
}
