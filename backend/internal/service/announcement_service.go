package service

import (
	"context"
	"fmt"
	"html"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
)

type AnnouncementService struct {
	announcementRepo AnnouncementRepository
	readRepo         AnnouncementReadRepository
	userRepo         UserRepository
	userSubRepo      UserSubscriptionRepository
	emailService     *EmailService
	settingService   *SettingService
}

func NewAnnouncementService(
	announcementRepo AnnouncementRepository,
	readRepo AnnouncementReadRepository,
	userRepo UserRepository,
	userSubRepo UserSubscriptionRepository,
	emailService *EmailService,
	settingService *SettingService,
) *AnnouncementService {
	return &AnnouncementService{
		announcementRepo: announcementRepo,
		readRepo:         readRepo,
		userRepo:         userRepo,
		userSubRepo:      userSubRepo,
		emailService:     emailService,
		settingService:   settingService,
	}
}

type CreateAnnouncementInput struct {
	Title      string
	Content    string
	Status     string
	NotifyMode string
	Targeting  AnnouncementTargeting
	StartsAt   *time.Time
	EndsAt     *time.Time
	ActorID    *int64 // 管理员用户ID
	SendEmail  bool
}

type UpdateAnnouncementInput struct {
	Title      *string
	Content    *string
	Status     *string
	NotifyMode *string
	Targeting  *AnnouncementTargeting
	StartsAt   **time.Time
	EndsAt     **time.Time
	ActorID    *int64 // 管理员用户ID
	SendEmail  bool
}

type UserAnnouncement struct {
	Announcement Announcement
	ReadAt       *time.Time
}

type AnnouncementUserReadStatus struct {
	UserID   int64      `json:"user_id"`
	Email    string     `json:"email"`
	Username string     `json:"username"`
	Balance  float64    `json:"balance"`
	Eligible bool       `json:"eligible"`
	ReadAt   *time.Time `json:"read_at,omitempty"`
}

func (s *AnnouncementService) Create(ctx context.Context, input *CreateAnnouncementInput) (*Announcement, error) {
	if input == nil {
		return nil, ErrAnnouncementNilInput
	}

	title := strings.TrimSpace(input.Title)
	content := strings.TrimSpace(input.Content)
	if title == "" || len(title) > 200 {
		return nil, ErrAnnouncementInvalidTitle
	}
	if content == "" {
		return nil, ErrAnnouncementContentRequired
	}

	status := strings.TrimSpace(input.Status)
	if status == "" {
		status = AnnouncementStatusDraft
	}
	if !isValidAnnouncementStatus(status) {
		return nil, ErrAnnouncementInvalidStatus
	}

	targeting, err := domain.AnnouncementTargeting(input.Targeting).NormalizeAndValidate()
	if err != nil {
		return nil, err
	}

	notifyMode := strings.TrimSpace(input.NotifyMode)
	if notifyMode == "" {
		notifyMode = AnnouncementNotifyModeSilent
	}
	if !isValidAnnouncementNotifyMode(notifyMode) {
		return nil, ErrAnnouncementInvalidNotifyMode
	}

	if input.StartsAt != nil && input.EndsAt != nil {
		if !input.StartsAt.Before(*input.EndsAt) {
			return nil, ErrAnnouncementInvalidSchedule
		}
	}

	a := &Announcement{
		Title:      title,
		Content:    content,
		Status:     status,
		NotifyMode: notifyMode,
		Targeting:  targeting,
		StartsAt:   input.StartsAt,
		EndsAt:     input.EndsAt,
	}
	if input.ActorID != nil && *input.ActorID > 0 {
		a.CreatedBy = input.ActorID
		a.UpdatedBy = input.ActorID
	}

	if err := s.announcementRepo.Create(ctx, a); err != nil {
		return nil, fmt.Errorf("create announcement: %w", err)
	}
	if input.SendEmail {
		s.queueAnnouncementEmailNotifications(ctx, a)
		refreshed, err := s.announcementRepo.GetByID(ctx, a.ID)
		if err == nil {
			a = refreshed
		}
	}
	return a, nil
}

func (s *AnnouncementService) Update(ctx context.Context, id int64, input *UpdateAnnouncementInput) (*Announcement, error) {
	if input == nil {
		return nil, ErrAnnouncementNilInput
	}

	a, err := s.announcementRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	if input.Title != nil {
		title := strings.TrimSpace(*input.Title)
		if title == "" || len(title) > 200 {
			return nil, ErrAnnouncementInvalidTitle
		}
		a.Title = title
	}
	if input.Content != nil {
		content := strings.TrimSpace(*input.Content)
		if content == "" {
			return nil, ErrAnnouncementContentRequired
		}
		a.Content = content
	}
	if input.Status != nil {
		status := strings.TrimSpace(*input.Status)
		if !isValidAnnouncementStatus(status) {
			return nil, ErrAnnouncementInvalidStatus
		}
		a.Status = status
	}

	if input.NotifyMode != nil {
		notifyMode := strings.TrimSpace(*input.NotifyMode)
		if !isValidAnnouncementNotifyMode(notifyMode) {
			return nil, ErrAnnouncementInvalidNotifyMode
		}
		a.NotifyMode = notifyMode
	}

	if input.Targeting != nil {
		targeting, err := domain.AnnouncementTargeting(*input.Targeting).NormalizeAndValidate()
		if err != nil {
			return nil, err
		}
		a.Targeting = targeting
	}

	if input.StartsAt != nil {
		a.StartsAt = *input.StartsAt
	}
	if input.EndsAt != nil {
		a.EndsAt = *input.EndsAt
	}

	if a.StartsAt != nil && a.EndsAt != nil {
		if !a.StartsAt.Before(*a.EndsAt) {
			return nil, ErrAnnouncementInvalidSchedule
		}
	}

	if input.ActorID != nil && *input.ActorID > 0 {
		a.UpdatedBy = input.ActorID
	}

	if err := s.announcementRepo.Update(ctx, a); err != nil {
		return nil, fmt.Errorf("update announcement: %w", err)
	}
	if input.SendEmail {
		s.queueAnnouncementEmailNotifications(ctx, a)
		refreshed, err := s.announcementRepo.GetByID(ctx, a.ID)
		if err == nil {
			a = refreshed
		}
	}
	return a, nil
}

func (s *AnnouncementService) Delete(ctx context.Context, id int64) error {
	if err := s.announcementRepo.Delete(ctx, id); err != nil {
		return fmt.Errorf("delete announcement: %w", err)
	}
	return nil
}

func (s *AnnouncementService) GetByID(ctx context.Context, id int64) (*Announcement, error) {
	return s.announcementRepo.GetByID(ctx, id)
}

func (s *AnnouncementService) List(ctx context.Context, params pagination.PaginationParams, filters AnnouncementListFilters) ([]Announcement, *pagination.PaginationResult, error) {
	return s.announcementRepo.List(ctx, params, filters)
}

func (s *AnnouncementService) ListForUser(ctx context.Context, userID int64, unreadOnly bool) ([]UserAnnouncement, error) {
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("get user: %w", err)
	}

	activeSubs, err := s.userSubRepo.ListActiveByUserID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("list active subscriptions: %w", err)
	}
	activeGroupIDs := make(map[int64]struct{}, len(activeSubs))
	for i := range activeSubs {
		activeGroupIDs[activeSubs[i].GroupID] = struct{}{}
	}

	now := time.Now()
	anns, err := s.announcementRepo.ListActive(ctx, now)
	if err != nil {
		return nil, fmt.Errorf("list active announcements: %w", err)
	}

	visible := make([]Announcement, 0, len(anns))
	ids := make([]int64, 0, len(anns))
	userTagIDs := s.userTagIDSet(ctx, user.ID)
	for i := range anns {
		a := anns[i]
		if !a.IsActiveAt(now) {
			continue
		}
		if !a.Targeting.Matches(user.Balance, activeGroupIDs, userTagIDs) {
			continue
		}
		visible = append(visible, a)
		ids = append(ids, a.ID)
	}

	if len(visible) == 0 {
		return []UserAnnouncement{}, nil
	}

	readMap, err := s.readRepo.GetReadMapByUser(ctx, userID, ids)
	if err != nil {
		return nil, fmt.Errorf("get read map: %w", err)
	}

	out := make([]UserAnnouncement, 0, len(visible))
	for i := range visible {
		a := visible[i]
		readAt, ok := readMap[a.ID]
		if unreadOnly && ok {
			continue
		}
		var ptr *time.Time
		if ok {
			t := readAt
			ptr = &t
		}
		out = append(out, UserAnnouncement{
			Announcement: a,
			ReadAt:       ptr,
		})
	}

	// 未读优先、同状态按创建时间倒序
	sort.Slice(out, func(i, j int) bool {
		ai, aj := out[i], out[j]
		if (ai.ReadAt == nil) != (aj.ReadAt == nil) {
			return ai.ReadAt == nil
		}
		return ai.Announcement.ID > aj.Announcement.ID
	})

	return out, nil
}

func (s *AnnouncementService) MarkRead(ctx context.Context, userID, announcementID int64) error {
	// 安全：仅允许标记当前用户“可见”的公告
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return fmt.Errorf("get user: %w", err)
	}

	a, err := s.announcementRepo.GetByID(ctx, announcementID)
	if err != nil {
		return err
	}

	now := time.Now()
	if !a.IsActiveAt(now) {
		return ErrAnnouncementNotFound
	}

	activeSubs, err := s.userSubRepo.ListActiveByUserID(ctx, userID)
	if err != nil {
		return fmt.Errorf("list active subscriptions: %w", err)
	}
	activeGroupIDs := make(map[int64]struct{}, len(activeSubs))
	for i := range activeSubs {
		activeGroupIDs[activeSubs[i].GroupID] = struct{}{}
	}

	userTagIDs := s.userTagIDSet(ctx, user.ID)
	if !a.Targeting.Matches(user.Balance, activeGroupIDs, userTagIDs) {
		return ErrAnnouncementNotFound
	}

	if err := s.readRepo.MarkRead(ctx, announcementID, userID, now); err != nil {
		return fmt.Errorf("mark read: %w", err)
	}
	return nil
}

func (s *AnnouncementService) ListUserReadStatus(
	ctx context.Context,
	announcementID int64,
	params pagination.PaginationParams,
	search string,
) ([]AnnouncementUserReadStatus, *pagination.PaginationResult, error) {
	ann, err := s.announcementRepo.GetByID(ctx, announcementID)
	if err != nil {
		return nil, nil, err
	}

	filters := UserListFilters{
		Search: strings.TrimSpace(search),
	}

	users, page, err := s.userRepo.ListWithFilters(ctx, params, filters)
	if err != nil {
		return nil, nil, fmt.Errorf("list users: %w", err)
	}

	userIDs := make([]int64, 0, len(users))
	for i := range users {
		userIDs = append(userIDs, users[i].ID)
	}

	readMap, err := s.readRepo.GetReadMapByUsers(ctx, announcementID, userIDs)
	if err != nil {
		return nil, nil, fmt.Errorf("get read map: %w", err)
	}

	out := make([]AnnouncementUserReadStatus, 0, len(users))
	for i := range users {
		u := users[i]
		subs, err := s.userSubRepo.ListActiveByUserID(ctx, u.ID)
		if err != nil {
			return nil, nil, fmt.Errorf("list active subscriptions: %w", err)
		}
		activeGroupIDs := make(map[int64]struct{}, len(subs))
		for j := range subs {
			activeGroupIDs[subs[j].GroupID] = struct{}{}
		}

		readAt, ok := readMap[u.ID]
		var ptr *time.Time
		if ok {
			t := readAt
			ptr = &t
		}

		out = append(out, AnnouncementUserReadStatus{
			UserID:   u.ID,
			Email:    u.Email,
			Username: u.Username,
			Balance:  u.Balance,
			Eligible: domain.AnnouncementTargeting(ann.Targeting).Matches(u.Balance, activeGroupIDs, s.userTagIDSet(ctx, u.ID)),
			ReadAt:   ptr,
		})
	}

	return out, page, nil
}

func (s *AnnouncementService) sendAnnouncementEmailNotifications(ctx context.Context, a *Announcement) error {
	if s == nil || a == nil || a.ID <= 0 {
		return nil
	}
	if s.userRepo == nil {
		return fmt.Errorf("announcement user repository is not configured")
	}
	if s.announcementRepo == nil {
		return fmt.Errorf("announcement repository is not configured")
	}

	recipients, err := s.listAnnouncementEmailRecipients(ctx, a.Targeting)
	if err != nil {
		_ = s.announcementRepo.UpdateEmailProgress(ctx, a.ID, AnnouncementEmailStatusFailed, 0, 0, 0, nil)
		return err
	}
	total := len(recipients)
	if err := s.announcementRepo.UpdateEmailProgress(ctx, a.ID, AnnouncementEmailStatusSending, total, 0, 0, nil); err != nil {
		return fmt.Errorf("update announcement email progress: %w", err)
	}
	if len(recipients) == 0 {
		slog.Info("announcement email skipped: no matching recipients", "announcement_id", a.ID)
		now := time.Now()
		_ = s.announcementRepo.UpdateEmailProgress(ctx, a.ID, AnnouncementEmailStatusSent, 0, 0, 0, &now)
		return nil
	}
	if s.emailService == nil {
		_ = s.announcementRepo.UpdateEmailProgress(ctx, a.ID, AnnouncementEmailStatusFailed, total, 0, total, nil)
		return fmt.Errorf("announcement email service is not configured")
	}
	smtpConfig, err := s.emailService.GetSMTPConfig(ctx)
	if err != nil {
		_ = s.announcementRepo.UpdateEmailProgress(ctx, a.ID, AnnouncementEmailStatusFailed, total, 0, total, nil)
		return fmt.Errorf("get announcement email smtp config: %w", err)
	}

	subject := strings.TrimSpace(a.Title)
	if subject == "" {
		subject = "Announcement"
	}
	body := s.buildAnnouncementEmailBody(a)
	sent := 0
	failed := 0
	for _, to := range recipients {
		if err := s.emailService.SendEmailWithConfig(smtpConfig, to, subject, body); err != nil {
			slog.Error("failed to send announcement email", "announcement_id", a.ID, "to", to, "error", err)
			failed++
			_ = s.announcementRepo.UpdateEmailProgress(ctx, a.ID, AnnouncementEmailStatusSending, total, sent, failed, nil)
			continue
		}
		sent++
		slog.Info("announcement email sent", "announcement_id", a.ID, "to", to)
		_ = s.announcementRepo.UpdateEmailProgress(ctx, a.ID, AnnouncementEmailStatusSending, total, sent, failed, nil)
	}
	status := AnnouncementEmailStatusSent
	if failed > 0 {
		if sent > 0 {
			status = AnnouncementEmailStatusPartialFailed
		} else {
			status = AnnouncementEmailStatusFailed
		}
	}
	now := time.Now()
	if err := s.announcementRepo.UpdateEmailProgress(ctx, a.ID, status, total, sent, failed, &now); err != nil {
		return fmt.Errorf("finalize announcement email progress: %w", err)
	}
	return nil
}

func (s *AnnouncementService) queueAnnouncementEmailNotifications(ctx context.Context, a *Announcement) {
	if s == nil || a == nil || a.ID <= 0 || s.announcementRepo == nil {
		return
	}
	queued, err := s.announcementRepo.QueueEmailIfNotStarted(ctx, a.ID)
	if err != nil {
		slog.Error("failed to queue announcement email", "announcement_id", a.ID, "error", err)
		return
	}
	if !queued {
		return
	}
	a.EmailStatus = AnnouncementEmailStatusQueued
	go func(id int64) {
		bgCtx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		defer cancel()
		item, err := s.announcementRepo.GetByID(bgCtx, id)
		if err != nil {
			slog.Error("failed to load queued announcement email", "announcement_id", id, "error", err)
			return
		}
		if err := s.sendAnnouncementEmailNotifications(bgCtx, item); err != nil {
			slog.Error("failed to send queued announcement email", "announcement_id", id, "error", err)
		}
	}(a.ID)
}

func (s *AnnouncementService) listAnnouncementEmailRecipients(ctx context.Context, targeting AnnouncementTargeting) ([]string, error) {
	const pageSize = 1000
	recipients := make([]string, 0)
	seen := make(map[string]struct{})
	loadSubs := true

	for page := 1; ; page++ {
		users, result, err := s.userRepo.ListWithFilters(ctx, pagination.PaginationParams{
			Page:      page,
			PageSize:  pageSize,
			SortBy:    "id",
			SortOrder: pagination.SortOrderAsc,
		}, UserListFilters{IncludeSubscriptions: &loadSubs})
		if err != nil {
			return nil, fmt.Errorf("list announcement email recipients: %w", err)
		}
		if len(users) == 0 {
			break
		}

		for i := range users {
			u := users[i]
			activeGroupIDs := make(map[int64]struct{}, len(u.Subscriptions))
			for j := range u.Subscriptions {
				if u.Subscriptions[j].Status == SubscriptionStatusActive {
					activeGroupIDs[u.Subscriptions[j].GroupID] = struct{}{}
				}
			}
			if !targeting.Matches(u.Balance, activeGroupIDs, s.userTagIDSet(ctx, u.ID)) {
				continue
			}
			email := strings.TrimSpace(u.Email)
			if email == "" {
				continue
			}
			key := strings.ToLower(email)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			recipients = append(recipients, email)
		}

		if result == nil || page >= result.Pages || len(users) < pageSize {
			break
		}
	}

	return recipients, nil
}

func (s *AnnouncementService) userTagIDSet(ctx context.Context, userID int64) map[int64]struct{} {
	out := make(map[int64]struct{})
	if s == nil || s.userRepo == nil || userID <= 0 {
		return out
	}
	repo, ok := s.userRepo.(interface {
		GetUserTagsByUserID(context.Context, int64) ([]UserTag, error)
	})
	if !ok {
		return out
	}
	tags, err := repo.GetUserTagsByUserID(ctx, userID)
	if err != nil {
		slog.Warn("announcement user tags unavailable", "user_id", userID, "error", err)
		return out
	}
	for i := range tags {
		out[tags[i].ID] = struct{}{}
	}
	return out
}

func (s *AnnouncementService) buildAnnouncementEmailBody(a *Announcement) string {
	homeURL := "/"
	if s != nil && s.settingService != nil {
		if value := strings.TrimSpace(s.settingService.GetFrontendURL(context.Background())); value != "" {
			homeURL = value
		}
	}
	title := html.EscapeString(a.Title)
	content := html.EscapeString(a.Content)
	content = strings.ReplaceAll(content, "\r\n", "\n")
	content = strings.ReplaceAll(content, "\n", "<br>")
	return fmt.Sprintf(announcementEmailTemplate, title, content, html.EscapeString(homeURL))
}

const announcementEmailTemplate = `<!doctype html>
<html>
<head>
  <meta charset="UTF-8">
  <style>
    body { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif; line-height: 1.6; color: #111827; background: #f3f4f6; margin: 0; padding: 24px; }
    .card { max-width: 640px; margin: 0 auto; background: #ffffff; border-radius: 14px; padding: 28px; box-shadow: 0 10px 30px rgba(15, 23, 42, 0.08); }
    h1 { margin: 0 0 18px; font-size: 22px; color: #111827; }
    .content { color: #374151; font-size: 15px; margin-bottom: 24px; }
    .button { display: inline-block; padding: 11px 18px; border-radius: 10px; background: #2563eb; color: #ffffff !important; text-decoration: none; font-weight: 600; }
  </style>
</head>
<body>
  <div class="card">
    <h1>%s</h1>
    <div class="content">%s</div>
    <a class="button" href="%s">马上查看</a>
  </div>
</body>
</html>`

func isValidAnnouncementStatus(status string) bool {
	switch status {
	case AnnouncementStatusDraft, AnnouncementStatusActive, AnnouncementStatusArchived:
		return true
	default:
		return false
	}
}

func isValidAnnouncementNotifyMode(mode string) bool {
	switch mode {
	case AnnouncementNotifyModeSilent, AnnouncementNotifyModePopup:
		return true
	default:
		return false
	}
}
