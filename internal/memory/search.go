// internal/memory/search.go
package memory

import (
	"sort"
	"strings"
)

// SearchResult is a search hit with a normalized match score.
type SearchResult struct {
	Entry
	Rank float64 // 일치한 토큰 수 / 전체 토큰 수 (1.0 = 모든 토큰 일치)
}

// Search scores entries by case-insensitive substring containment per query
// token. 부분 문자열 매칭이므로 한국어 조사 변형("메모리를")도 "메모리"로 검색된다.
func (s *Store) Search(project, query string, limit int) ([]SearchResult, error) {
	if limit <= 0 {
		limit = 10
	}
	tokens := strings.Fields(strings.ToLower(query))
	if len(tokens) == 0 {
		return nil, nil
	}
	entries, err := s.List(project)
	if err != nil {
		return nil, err
	}
	var results []SearchResult
	for _, e := range entries {
		text := strings.ToLower(e.Key + "\n" + e.Content)
		matched := 0
		for _, tok := range tokens {
			if strings.Contains(text, tok) {
				matched++
			}
		}
		if matched == 0 {
			continue
		}
		results = append(results, SearchResult{Entry: e, Rank: float64(matched) / float64(len(tokens))})
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
