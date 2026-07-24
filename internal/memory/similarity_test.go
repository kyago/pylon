// internal/memory/similarity_test.go
package memory

import "testing"

func TestIsNearDuplicate(t *testing.T) {
	cases := []struct {
		name string
		a, b string
		want bool
	}{
		{"동일 문장", "이 프로젝트는 CGO_ENABLED=0으로 빌드해야 한다", "이 프로젝트는 CGO_ENABLED=0으로 빌드해야 한다", true},
		{"공백·대소문자 차이", "이 프로젝트는  CGO_ENABLED=0으로 빌드해야 한다", "이 프로젝트는 cgo_enabled=0으로 빌드해야 한다", true},
		{"어미만 다른 재진술", "테스트는 race 플래그를 켜고 실행해야 한다는 것", "테스트는 race 플래그를 켜고 실행해야 한다", true},
		{"다른 지식", "테스트는 race 플래그를 켜고 실행해야 한다", "릴리스는 goreleaser가 태그 푸시로 트리거한다", false},
		{"짧은 문자열은 정확 일치만", "빌드 성공", "빌드 성능", false},
		{"짧아도 정규화 후 동일하면 중복", "빌드 성공", "빌드  성공", true},
		// macOS 클립보드 등에서 NFD(자모 분해)로 들어온 한글은 NFC로 정규화해 비교한다
		{"NFD/NFC 정규화", "한글 인코딩 관련 메모", "한글 인코딩 관련 메모", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := isNearDuplicate(c.a, c.b); got != c.want {
				t.Errorf("isNearDuplicate(%q, %q) = %v, want %v", c.a, c.b, got, c.want)
			}
		})
	}
}

func TestDiceSimilarityBounds(t *testing.T) {
	if got := diceSimilarity("", "아무 내용"); got != 0 {
		t.Errorf("빈 문자열 유사도 = %v, want 0", got)
	}
	if got := diceSimilarity("같은 내용 문자열", "같은 내용 문자열"); got != 1 {
		t.Errorf("동일 문자열 유사도 = %v, want 1", got)
	}
}
