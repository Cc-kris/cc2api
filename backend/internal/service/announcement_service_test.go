package service

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/stretchr/testify/require"
)

type announcementRepoStub struct {
	item *Announcement
}

func (s *announcementRepoStub) Create(_ context.Context, a *Announcement) error {
	s.item = a
	return nil
}

func (s *announcementRepoStub) GetByID(_ context.Context, _ int64) (*Announcement, error) {
	if s.item == nil {
		return nil, ErrAnnouncementNotFound
	}
	return s.item, nil
}

func (s *announcementRepoStub) Update(_ context.Context, a *Announcement) error {
	s.item = a
	return nil
}

func (s *announcementRepoStub) MarkEmailSentIfUnset(_ context.Context, _ int64, sentAt time.Time) (bool, error) {
	if s.item == nil || s.item.EmailSentAt != nil {
		return false, nil
	}
	s.item.EmailSentAt = &sentAt
	return true, nil
}

func (s *announcementRepoStub) QueueEmailIfNotStarted(_ context.Context, _ int64) (bool, error) {
	if s.item == nil || s.item.EmailSentAt != nil || s.item.EmailStatus == AnnouncementEmailStatusQueued || s.item.EmailStatus == AnnouncementEmailStatusSending {
		return false, nil
	}
	s.item.EmailStatus = AnnouncementEmailStatusQueued
	return true, nil
}

func (s *announcementRepoStub) UpdateEmailProgress(_ context.Context, _ int64, status string, total, sent, failed int, sentAt *time.Time) error {
	if s.item == nil {
		return ErrAnnouncementNotFound
	}
	s.item.EmailStatus = status
	s.item.EmailTotal = total
	s.item.EmailSent = sent
	s.item.EmailFailed = failed
	if sentAt != nil {
		s.item.EmailSentAt = sentAt
	}
	return nil
}

func (*announcementRepoStub) Delete(context.Context, int64) error {
	return nil
}

func (*announcementRepoStub) List(context.Context, pagination.PaginationParams, AnnouncementListFilters) ([]Announcement, *pagination.PaginationResult, error) {
	return nil, nil, nil
}

func (*announcementRepoStub) ListActive(context.Context, time.Time) ([]Announcement, error) {
	return nil, nil
}

type announcementUserRepoStub struct {
	UserRepository
	pages [][]User
}

func (s *announcementUserRepoStub) ListWithFilters(_ context.Context, params pagination.PaginationParams, _ UserListFilters) ([]User, *pagination.PaginationResult, error) {
	index := params.Page - 1
	if index < 0 || index >= len(s.pages) {
		return []User{}, &pagination.PaginationResult{Page: params.Page, PageSize: params.PageSize, Pages: len(s.pages)}, nil
	}
	return s.pages[index], &pagination.PaginationResult{Page: params.Page, PageSize: params.PageSize, Pages: len(s.pages)}, nil
}

func TestAnnouncementServiceCreateRejectsEqualStartEndTimes(t *testing.T) {
	repo := &announcementRepoStub{}
	svc := NewAnnouncementService(repo, nil, nil, nil, nil, nil)
	now := time.Unix(1776790020, 0)

	_, err := svc.Create(context.Background(), &CreateAnnouncementInput{
		Title:      "公告",
		Content:    "内容",
		Status:     AnnouncementStatusActive,
		NotifyMode: AnnouncementNotifyModePopup,
		StartsAt:   &now,
		EndsAt:     &now,
	})
	require.ErrorIs(t, err, ErrAnnouncementInvalidSchedule)
}

func TestAnnouncementServiceUpdateRejectsEqualStartEndTimes(t *testing.T) {
	repo := &announcementRepoStub{
		item: &Announcement{
			ID:         1,
			Title:      "公告",
			Content:    "内容",
			Status:     AnnouncementStatusActive,
			NotifyMode: AnnouncementNotifyModePopup,
		},
	}
	svc := NewAnnouncementService(repo, nil, nil, nil, nil, nil)
	now := time.Unix(1776790020, 0)
	startsAt := &now
	endsAt := &now

	_, err := svc.Update(context.Background(), 1, &UpdateAnnouncementInput{
		StartsAt: &startsAt,
		EndsAt:   &endsAt,
	})
	require.ErrorIs(t, err, ErrAnnouncementInvalidSchedule)
}

func TestAnnouncementEmailRecipientsUseAnnouncementTargeting(t *testing.T) {
	svc := NewAnnouncementService(&announcementRepoStub{}, nil, &announcementUserRepoStub{
		pages: [][]User{
			{
				{ID: 1, Email: "low@example.com", Balance: 1},
				{ID: 2, Email: "match@example.com", Balance: 20},
				{ID: 3, Email: "match@example.com", Balance: 30},
			},
		},
	}, nil, nil, nil)

	recipients, err := svc.listAnnouncementEmailRecipients(context.Background(), AnnouncementTargeting{
		AnyOf: []AnnouncementConditionGroup{{
			AllOf: []AnnouncementCondition{{
				Type:     AnnouncementConditionTypeBalance,
				Operator: AnnouncementOperatorGTE,
				Value:    10,
			}},
		}},
	})

	require.NoError(t, err)
	require.Equal(t, []string{"match@example.com"}, recipients)
}

func TestAnnouncementEmailBodyEscapesContentAndLinksHome(t *testing.T) {
	svc := NewAnnouncementService(&announcementRepoStub{}, nil, nil, nil, nil, nil)

	body := svc.buildAnnouncementEmailBody(&Announcement{
		Title:   `<script>alert("x")</script>`,
		Content: "第一行\n<a>第二行</a>",
	})

	require.Contains(t, body, "&lt;script&gt;")
	require.Contains(t, body, "第一行<br>&lt;a&gt;第二行&lt;/a&gt;")
	require.Contains(t, body, `href="/"`)
	require.False(t, strings.Contains(body, `<script>alert("x")</script>`))
}
