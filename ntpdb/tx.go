package ntpdb

import (
	"context"
	"database/sql"
)

type QuerierTx interface {
	Querier

	Begin(ctx context.Context) (QuerierTx, error)
	Commit(ctx context.Context) error
	Rollback(ctx context.Context) error
}

type Beginner interface {
	Begin(context.Context) (sql.Tx, error)
}

type Tx interface {
	Begin(context.Context) (sql.Tx, error)
	Commit(ctx context.Context) error
	Rollback(ctx context.Context) error
}

func (q *Queries) Begin(ctx context.Context) (QuerierTx, error) {
	tx, err := q.db.(Beginner).Begin(ctx)
	if err != nil {
		return nil, err
	}
	return &Queries{db: &tx}, nil
}

func (q *Queries) Commit(ctx context.Context) error {
	tx, ok := q.db.(Tx)
	if !ok {
		return sql.ErrTxDone
	}
	return tx.Commit(ctx)
}

func (q *Queries) Rollback(ctx context.Context) error {
	tx, ok := q.db.(Tx)
	if !ok {
		return sql.ErrTxDone
	}
	return tx.Rollback(ctx)
}

type WrappedQuerier struct {
	QuerierTxWithTracing
}

func NewWrappedQuerier(q QuerierTx) QuerierTx {
	return &WrappedQuerier{NewQuerierTxWithTracing(q, "")}
}

func (wq *WrappedQuerier) Begin(ctx context.Context) (QuerierTx, error) {
	q, err := wq.QuerierTxWithTracing.Begin(ctx)
	if err != nil {
		return nil, err
	}
	return NewWrappedQuerier(q), nil
}
