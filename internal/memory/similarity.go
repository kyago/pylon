// internal/memory/similarity.go
package memory

import (
	"strings"

	"golang.org/x/text/unicode/norm"
)

// nearDuplicateThreshold: 이 값 이상의 Dice 유사도는 같은 지식의 재진술로
// 간주한다 (D4 확장 — Stop hook이 매 턴 표현만 바뀐 학습을 다시 보내는 문제).
// 0.90에서는 "30초→60초"처럼 단일 토큰만 바뀐 값 정정도 근사 중복으로 잡혀
// 정정본이 조용히 버려지는 문제가 있어 0.95로 올렸다. char-bigram Dice는
// 여전히 근사치라 단일 토큰 정정과 순수 재진술을 완벽히 구분하지 못하는
// 잔여 위험이 있으며, 숫자가 관여하는 경우는 digitsDiffer로 별도 방어한다.
const nearDuplicateThreshold = 0.95

// nearDuplicateMinRunes: 이보다 짧은 문자열은 bigram 집합이 작아 유사도
// 신호가 불안정하므로 정규화 후 정확 일치만 중복으로 본다.
const nearDuplicateMinRunes = 20

// normalizeForCompare canonicalizes content for duplicate comparison:
// NFC(한글 자모 결합) → 소문자 → 연속 공백 축약.
func normalizeForCompare(s string) string {
	s = norm.NFC.String(s)
	s = strings.ToLower(s)
	return strings.Join(strings.Fields(s), " ")
}

// bigramSet returns the set of adjacent-rune pairs in s.
func bigramSet(s string) map[string]struct{} {
	runes := []rune(s)
	set := make(map[string]struct{}, len(runes))
	for i := 0; i+1 < len(runes); i++ {
		set[string(runes[i:i+2])] = struct{}{}
	}
	return set
}

// diceSimilarity computes the Sørensen–Dice coefficient of two strings'
// bigram sets. 문서-문서처럼 크기가 비슷한 두 집합 비교에 적합하다.
func diceSimilarity(a, b string) float64 {
	sa, sb := bigramSet(a), bigramSet(b)
	if len(sa) == 0 || len(sb) == 0 {
		return 0
	}
	inter := 0
	for g := range sa {
		if _, ok := sb[g]; ok {
			inter++
		}
	}
	return 2 * float64(inter) / float64(len(sa)+len(sb))
}

// isNearDuplicate reports whether two contents carry the same knowledge.
func isNearDuplicate(a, b string) bool {
	na, nb := normalizeForCompare(a), normalizeForCompare(b)
	if na == nb {
		return true
	}
	if len([]rune(na)) < nearDuplicateMinRunes || len([]rune(nb)) < nearDuplicateMinRunes {
		return false
	}
	if digitsDiffer(na, nb) {
		return false // 숫자만 바뀐 값 정정은 별개 항목으로 보존
	}
	return diceSimilarity(na, nb) >= nearDuplicateThreshold
}

// digitsDiffer reports whether the two strings' digit sequences differ.
// "30초" vs "60초"처럼 숫자만 바뀐 값 정정을 근사 중복에서 제외해 정정본이
// 조용히 버려지지 않게 한다.
func digitsDiffer(a, b string) bool {
	return extractDigits(a) != extractDigits(b)
}

func extractDigits(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteByte(byte(r))
		}
	}
	return b.String()
}
