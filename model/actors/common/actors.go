package common

import (
	"context"
	"fmt"

	"github.com/go-pg/pg/v10"
	"go.opentelemetry.io/otel/api/global"
)

type Actor struct {
	Height    int64  `pg:",pk,notnull,use_zero"`
	ID        string `pg:",pk,notnull"`
	StateRoot string `pg:",pk,notnull"`
	Code      string `pg:",notnull"`
	Head      string `pg:",notnull"`
	Balance   string `pg:",notnull"`
	Nonce     uint64 `pg:",use_zero"`
}

func (a *Actor) PersistWithTx(ctx context.Context, tx *pg.Tx) error {
	ctx, span := global.Tracer("").Start(ctx, "Actor.PersistWithTx")
	defer span.End()
	if _, err := tx.ModelContext(ctx, a).
		OnConflict("do nothing").
		Insert(); err != nil {
		return err
	}
	return nil
}

type Actors []*Actor

func (as Actors) PersistWithTx(ctx context.Context, tx *pg.Tx) error {
	if len(as) == 0 {
		return nil
	}
	if _, err := tx.ModelContext(ctx, &as).
		OnConflict("do nothing").
		Insert(); err != nil {
		return fmt.Errorf("persisting actors: %w", err)
	}
	return nil
}

type ActorState struct {
	Height int64  `pg:",pk,notnull,use_zero"`
	Head   string `pg:",pk,notnull"`
	Code   string `pg:",pk,notnull"`
	State  string `pg:",type:jsonb,notnull"`
}

func (s *ActorState) PersistWithTx(ctx context.Context, tx *pg.Tx) error {
	ctx, span := global.Tracer("").Start(ctx, "ActorState.PersistWithTx")
	defer span.End()
	if _, err := tx.ModelContext(ctx, s).
		OnConflict("do nothing").
		Insert(); err != nil {
		return err
	}
	return nil
}

type ActorStates []*ActorState

func (as ActorStates) PersistWithTx(ctx context.Context, tx *pg.Tx) error {
	if len(as) == 0 {
		return nil
	}
	if _, err := tx.ModelContext(ctx, &as).
		OnConflict("do nothing").
		Insert(); err != nil {
		return fmt.Errorf("persisting actorStates: %w", err)
	}
	return nil
}
