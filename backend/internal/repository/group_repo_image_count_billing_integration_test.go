//go:build integration

package repository

import (
	"github.com/Wei-Shaw/sub2api/internal/service"
)

// V03-T15：迁移 228 的 groups.image_count_billing 经 Create / Update / GetByID 往返。
func (s *GroupRepoSuite) TestImageCountBillingRoundTrip() {
	group := &service.Group{
		Name:                 "image-count-billing",
		Platform:             service.PlatformOpenAI,
		RateMultiplier:       1.0,
		Status:               service.StatusActive,
		SubscriptionType:     service.SubscriptionTypeStandard,
		AllowImageGeneration: true,
		ImageCountBilling:    true,
	}
	s.Require().NoError(s.repo.Create(s.ctx, group))

	got, err := s.repo.GetByID(s.ctx, group.ID)
	s.Require().NoError(err)
	s.Require().True(got.ImageCountBilling)

	got.ImageCountBilling = false
	s.Require().NoError(s.repo.Update(s.ctx, got))
	again, err := s.repo.GetByID(s.ctx, group.ID)
	s.Require().NoError(err)
	s.Require().False(again.ImageCountBilling)
}
