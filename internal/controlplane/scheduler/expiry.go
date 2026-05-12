package scheduler

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/zanel1u/cloud-cli-proxy/internal/agentapi"
	"github.com/zanel1u/cloud-cli-proxy/internal/store/repository"
)

type ExpiryStore interface {
	ListExpiredActiveUsers(context.Context) ([]repository.User, error)
	UpdateUserStatus(context.Context, string, string) (repository.User, error)
	ListRunningHostsByUserID(context.Context, string) ([]repository.Host, error)
	RecordEvent(context.Context, repository.RecordEventParams) (repository.Event, error)
}

type HostActionQueuer interface {
	QueueHostAction(ctx context.Context, hostID string, action agentapi.HostAction, requestedBy string, bypassSnapshotID string) (repository.Task, error)
}

type ExpiryScanner struct {
	logger *slog.Logger
	store  ExpiryStore
	queue  HostActionQueuer
}

func NewExpiryScanner(logger *slog.Logger, store ExpiryStore, queue HostActionQueuer) *ExpiryScanner {
	return &ExpiryScanner{logger: logger, store: store, queue: queue}
}

func (s *ExpiryScanner) Scan(ctx context.Context) error {
	expired, err := s.store.ListExpiredActiveUsers(ctx)
	if err != nil {
		return fmt.Errorf("list expired users: %w", err)
	}

	if len(expired) == 0 {
		return nil
	}

	for _, user := range expired {
		if err := s.expireUser(ctx, user); err != nil {
			s.logger.Error("expire user failed", "user_id", user.ID, "username", user.Username, "error", err)
			continue
		}
	}

	s.logger.Info("expiry scan completed", "expired_count", len(expired))
	return nil
}

func (s *ExpiryScanner) expireUser(ctx context.Context, user repository.User) error {
	if _, err := s.store.UpdateUserStatus(ctx, user.ID, "expired"); err != nil {
		return fmt.Errorf("mark user expired: %w", err)
	}

	metadata := map[string]any{
		"operator": "system",
		"reason":   "expires_at reached",
	}
	if user.ExpiresAt != nil {
		metadata["expires_at"] = user.ExpiresAt.String()
	}
	s.store.RecordEvent(ctx, repository.RecordEventParams{
		UserID:   &user.ID,
		Level:    "info",
		Type:     "user.expired",
		Message:  fmt.Sprintf("user %s expired", user.Username),
		Metadata: metadata,
	})

	hosts, err := s.store.ListRunningHostsByUserID(ctx, user.ID)
	if err != nil {
		return fmt.Errorf("list running hosts: %w", err)
	}

	for _, host := range hosts {
		if _, err := s.queue.QueueHostAction(ctx, host.ID, agentapi.ActionStopHost, "system:expiry", ""); err != nil {
			s.logger.Error("stop expired user host failed", "host_id", host.ID, "user_id", user.ID, "error", err)
			continue
		}

		s.store.RecordEvent(ctx, repository.RecordEventParams{
			HostID: &host.ID,
			UserID: &user.ID,
			Level:  "info",
			Type:   "host.stop.expired",
			Message: fmt.Sprintf("host %s stopped due to user expiry", host.ID),
			Metadata: map[string]any{
				"operator": "system",
				"user_id":  user.ID,
				"host_id":  host.ID,
				"reason":   "user expired",
			},
		})
	}

	return nil
}
