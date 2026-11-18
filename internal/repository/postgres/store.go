package postgres

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"

	"github.com/abc-workspace/chat-service/internal/pkg/apperr"
	"github.com/abc-workspace/chat-service/internal/repository"
)

type txCtxKey struct{}

// withTx gan transaction vao context.
func withTx(ctx context.Context, tx *gorm.DB) context.Context {
	return context.WithValue(ctx, txCtxKey{}, tx)
}

type base struct {
	root *gorm.DB
}

// db tra tx trong context neu dang o trong transaction, nguoc lai tra connection goc.
func (b base) db(ctx context.Context) *gorm.DB {
	if tx, ok := ctx.Value(txCtxKey{}).(*gorm.DB); ok && tx != nil {
		return tx.WithContext(ctx)
	}
	return b.root.WithContext(ctx)
}

type Store struct {
	gormDB *gorm.DB

	users         *UserRepo
	conversations *ConversationRepo
	messages      *MessageRepo
	bots          *BotRepo
	tasks         *TaskRepo
	teams         *TeamRepo
	notifications *NotificationRepo
	idempotency   *IdempotencyRepo
	outbox        *OutboxRepo
	tx            *TxManager
}

var _ repository.Store = (*Store)(nil)

func NewStore(db *gorm.DB) *Store {
	b := base{root: db}
	return &Store{
		gormDB:        db,
		users:         &UserRepo{base: b},
		conversations: &ConversationRepo{base: b},
		messages:      &MessageRepo{base: b},
		bots:          &BotRepo{base: b},
		tasks:         &TaskRepo{base: b},
		teams:         &TeamRepo{base: b},
		notifications: &NotificationRepo{base: b},
		idempotency:   &IdempotencyRepo{base: b},
		outbox:        &OutboxRepo{base: b},
		tx:            &TxManager{db: db},
	}
}

func (s *Store) Users() repository.UserRepository                 { return s.users }
func (s *Store) Conversations() repository.ConversationRepository { return s.conversations }
func (s *Store) Messages() repository.MessageRepository           { return s.messages }
func (s *Store) Bots() repository.BotRepository                   { return s.bots }
func (s *Store) Tasks() repository.TaskRepository                 { return s.tasks }
func (s *Store) Teams() repository.TeamRepository                 { return s.teams }
func (s *Store) Notifications() repository.NotificationRepository { return s.notifications }
func (s *Store) Idempotency() repository.IdempotencyRepository    { return s.idempotency }
func (s *Store) Outbox() repository.OutboxRepository              { return s.outbox }
func (s *Store) Tx() repository.TxManager                         { return s.tx }
func (s *Store) DB() *gorm.DB                                     { return s.gormDB }

type TxManager struct{ db *gorm.DB }

func (m *TxManager) Do(ctx context.Context, fn func(ctx context.Context) error) error {
	// Neu da o trong transaction thi dung lai (nested => savepoint khong can thiet).
	if tx, ok := ctx.Value(txCtxKey{}).(*gorm.DB); ok && tx != nil {
		return fn(ctx)
	}
	return m.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return fn(withTx(ctx, tx))
	})
}

// mapErr chuyen loi GORM/Postgres thanh apperr de transport map status dung.
func mapErr(err error, notFound *apperr.Error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, gorm.ErrRecordNotFound):
		if notFound != nil {
			return notFound
		}
		return apperr.NotFound("not_found", "Khong tim thay du lieu")
	case errors.Is(err, gorm.ErrDuplicatedKey):
		return apperr.Conflict("duplicated_key", "Du lieu da ton tai").Wrap(err)
	case errors.Is(err, gorm.ErrForeignKeyViolated):
		return apperr.Invalid("foreign_key_violated", "Du lieu tham chieu khong ton tai").Wrap(err)
	default:
		return apperr.Internal("db_error", "Loi truy cap du lieu").Wrap(err)
	}
}

func wrap(err error, op string) error {
	if err == nil {
		return nil
	}
	return mapErr(fmt.Errorf("%s: %w", op, err), nil)
}
