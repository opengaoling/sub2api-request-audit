package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// newStubPricingServiceFromJSON 用与生产一致的解析路径（含 above_XXXk 阶梯折算）
// 从原始目录 JSON 构造目录 stub。无 build tag：带 unit 标签与默认构建的测试文件都会用到。
func newStubPricingServiceFromJSON(t *testing.T, body string) *PricingService {
	t.Helper()
	s := &PricingService{}
	data, err := s.parsePricingData([]byte(body))
	require.NoError(t, err)
	s.pricingData = data
	return s
}
