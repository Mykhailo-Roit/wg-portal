package wireguard

import (
	"context"
	"log/slog"
	"math"
	"time"

	"github.com/h44z/wg-portal/internal/config"
	"github.com/h44z/wg-portal/internal/domain"
)

// NotificationRepository persists and retrieves peer expiry notification records.
// It is used by the NotificationManager to enforce the at-most-once guarantee
// across process restarts (Requirements 6.1, 6.2).
type NotificationRepository interface {
	// SaveNotificationRecord persists a record indicating that an expiry warning
	// email has been sent for the given peer and interval.
	SaveNotificationRecord(ctx context.Context, rec domain.PeerNotificationRecord) error

	// GetNotificationRecords returns all persisted notification records for the
	// given peer identifier.
	GetNotificationRecords(ctx context.Context, peerID domain.PeerIdentifier) ([]domain.PeerNotificationRecord, error)

	// DeleteNotificationRecordsForPeer removes all notification records for the
	// given peer identifier (called when a peer is deleted or its ExpiresAt is
	// extended so that notifications are re-sent relative to the new expiry date).
	DeleteNotificationRecordsForPeer(ctx context.Context, peerID domain.PeerIdentifier) error

	// DeleteNotificationRecordsBefore removes all notification records whose
	// SentAt timestamp is strictly before cutoff (used for retention pruning).
	DeleteNotificationRecordsBefore(ctx context.Context, cutoff time.Time) error
}

// NotificationManagerDatabaseRepo is the read-only database interface used by NotificationManager.
type NotificationManagerDatabaseRepo interface {
	GetAllInterfaces(ctx context.Context) ([]domain.Interface, error)
	GetInterfacePeers(ctx context.Context, id domain.InterfaceIdentifier) ([]domain.Peer, error)
	GetUser(ctx context.Context, id domain.UserIdentifier) (*domain.User, error)
}

// NotificationMailer sends expiry notification emails.
type NotificationMailer interface {
	SendExpiryNotification(ctx context.Context, peer *domain.Peer, user *domain.User, daysLeft int) error
}

// NotificationManager is the background service that sends peer expiry warning emails.
type NotificationManager struct {
	cfg       *config.Config
	db        NotificationManagerDatabaseRepo
	notifRepo NotificationRepository
	mailer    NotificationMailer
}

// NewNotificationManager creates a new NotificationManager.
func NewNotificationManager(
	cfg *config.Config,
	db NotificationManagerDatabaseRepo,
	notifRepo NotificationRepository,
	mailer NotificationMailer,
) *NotificationManager {
	return &NotificationManager{
		cfg:       cfg,
		db:        db,
		notifRepo: notifRepo,
		mailer:    mailer,
	}
}

// Run starts the notification check loop. It exits when ctx is cancelled (Requirement 7.3).
func (nm *NotificationManager) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(nm.cfg.Advanced.ExpiryCheckInterval):
		}
		nm.checkAndNotify(ctx)
	}
}

// checkAndNotify iterates all interfaces and peers, sends notifications where due,
// then prunes old notification records (Requirements 3.4, 3.5, 3.7, 4.1–4.7, 7.1).
func (nm *NotificationManager) checkAndNotify(ctx context.Context) {
	// Early-return when notifications are disabled or no intervals configured (Requirements 3.4, 3.5).
	if !nm.cfg.Core.Peer.ExpiryNotificationEnabled {
		return
	}
	if len(nm.cfg.Core.Peer.ExpiryNotificationIntervals) == 0 {
		return
	}

	interfaces, err := nm.db.GetAllInterfaces(ctx)
	if err != nil {
		slog.Error("notification manager: failed to fetch interfaces", "error", err)
		return
	}

	for _, iface := range interfaces {
		peers, err := nm.db.GetInterfacePeers(ctx, iface.Identifier)
		if err != nil {
			slog.Error("notification manager: failed to fetch peers for interface",
				"interface", iface.Identifier,
				"error", err)
			continue
		}

		for i := range peers {
			nm.processPeerNotifications(ctx, &peers[i])
		}
	}

	// Prune old notification records (Requirement 3.7).
	cutoff := time.Now().Add(-nm.cfg.Core.Peer.NotificationCleanupAfter)
	if err := nm.notifRepo.DeleteNotificationRecordsBefore(ctx, cutoff); err != nil {
		slog.Warn("notification manager: failed to prune old notification records", "error", err)
	}
}

// processPeerNotifications checks whether any notification intervals are due for the given peer
// and sends emails accordingly, enforcing the at-most-once guarantee (Requirements 4.1–4.7).
func (nm *NotificationManager) processPeerNotifications(ctx context.Context, peer *domain.Peer) {
	// Skip peers with no expiry (Requirement 4.5).
	if peer.ExpiresAt == nil {
		return
	}

	// Skip disabled peers (Requirement 4.4).
	if peer.IsDisabled() {
		return
	}

	// Skip peers with no linked user or no email (Requirement 4.3).
	if peer.UserIdentifier == "" {
		slog.Debug("notification manager: skipping peer with no linked user",
			"peer", peer.Identifier)
		return
	}

	user, err := nm.db.GetUser(ctx, peer.UserIdentifier)
	if err != nil {
		slog.Debug("notification manager: failed to resolve user for peer, skipping",
			"peer", peer.Identifier,
			"user", peer.UserIdentifier,
			"error", err)
		return
	}

	if user.Email == "" {
		slog.Debug("notification manager: skipping peer whose user has no email",
			"peer", peer.Identifier,
			"user", peer.UserIdentifier)
		return
	}

	// Load existing notification records for this peer once (used for at-most-once check).
	existingRecords, err := nm.notifRepo.GetNotificationRecords(ctx, peer.Identifier)
	if err != nil {
		slog.Error("notification manager: failed to fetch notification records for peer",
			"peer", peer.Identifier,
			"error", err)
		return
	}

	now := time.Now()

	for _, interval := range nm.cfg.Core.Peer.ExpiryNotificationIntervals {
		// Notification is due once we have passed the "D-before-expiry" moment.
		// Using a threshold (not a window) means a notification missed in one cycle
		// (e.g. due to a restart) is sent in the next cycle. The hasRecord guard
		// below still enforces the at-most-once guarantee.
		if now.Before(peer.ExpiresAt.Add(-interval)) {
			continue // too early — not yet due
		}

		// At-most-once guard: check if a record already exists for (peer, interval) (Requirement 4.2).
		if hasRecord(existingRecords, interval) {
			continue
		}

		// Compute days left for the email content (ceiling division so 23h59m → 1 day, not 0).
		hoursLeft := time.Until(*peer.ExpiresAt).Hours()
		daysLeft := int(math.Ceil(hoursLeft / 24))
		if daysLeft < 0 {
			daysLeft = 0
		}

		// Send the notification email (Requirement 4.6).
		if err := nm.mailer.SendExpiryNotification(ctx, peer, user, daysLeft); err != nil {
			// Log error and continue with remaining peers/intervals (Requirement 4.7).
			slog.Error("notification manager: failed to send expiry notification",
				"peer", peer.Identifier,
				"interval", interval,
				"error", err)
			continue
		}

		// Persist the record so we don't re-send (Requirement 6.1).
		rec := domain.PeerNotificationRecord{
			PeerIdentifier:  peer.Identifier,
			IntervalSeconds: int64(interval.Seconds()),
			SentAt:          now,
		}
		if err := nm.notifRepo.SaveNotificationRecord(ctx, rec); err != nil {
			slog.Warn("notification manager: failed to save notification record after successful send",
				"peer", peer.Identifier,
				"interval", interval,
				"error", err)
			// The email was sent; the record may be re-sent on the next cycle.
			// This is an acceptable trade-off per the design's error-handling table.
		}

		// Add to local cache so subsequent intervals in this same cycle see the record.
		existingRecords = append(existingRecords, rec)

		// Only send one notification per peer per cycle. Remaining intervals will
		// be picked up in subsequent cycles, so the user receives emails spread
		// over time rather than all at once.
		break
	}
}

// hasRecord returns true if existingRecords contains a record for the given interval.
func hasRecord(records []domain.PeerNotificationRecord, interval time.Duration) bool {
	target := int64(interval.Seconds())
	for _, r := range records {
		if r.IntervalSeconds == target {
			return true
		}
	}
	return false
}
