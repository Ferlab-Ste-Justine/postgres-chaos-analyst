package measure

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/Ferlab-Ste-Justine/postgres-chaos-analyst/config"
)

type Updater struct {
	TableName string
	index int64
}

func (up *Updater) Initialize(conf *config.PgClientConfig) error {
	ctx, _ := context.WithTimeout(context.Background(), conf.ConnectionTimeout)
	conn, connErr := pgx.Connect(ctx, conf.GetConnStr())
	if connErr != nil {
		return connErr
	}

	defer func() {
		ctx, _ := context.WithTimeout(context.Background(), conf.ConnectionTimeout)
		conn.Close(ctx)
	}()

	ctx, _ = context.WithTimeout(context.Background(), conf.QueryTimeout)
	tx, txErr := conn.Begin(ctx)
	if txErr != nil {
		return txErr
	}

	ctx, _ = context.WithTimeout(context.Background(), conf.QueryTimeout)
	_, txErr = tx.Exec(ctx, fmt.Sprintf("CREATE TABLE %s (value bigint NOT NULL);", up.TableName))
	if txErr != nil {
		return txErr
	}

	_, txErr = tx.Exec(ctx, fmt.Sprintf("INSERT INTO %s (value) VALUES ($1);", up.TableName), up.index)
	if txErr != nil {
		return txErr
	}

	ctx, _ = context.WithTimeout(context.Background(), conf.QueryTimeout)
	commErr := tx.Commit(ctx)
	return commErr
}

func (up *Updater) Run(conf *config.PgClientConfig) (Anomaly, error) {
	ctx, _ := context.WithTimeout(context.Background(), conf.ConnectionTimeout)
	conn, connErr := pgx.Connect(ctx, conf.GetConnStr())
	if connErr != nil {
		return NoProblem, connErr
	}

	defer func() {
		ctx, _ := context.WithTimeout(context.Background(), conf.ConnectionTimeout)
		conn.Close(ctx)
	}()

	ctx, _ = context.WithTimeout(context.Background(), conf.QueryTimeout)
	tx, txErr := conn.Begin(ctx)
	if txErr != nil {
		return NoProblem, txErr
	}

	anomaly := NoProblem
	if up.index > 0 {
		queryErr := func() error {
			ctx, _ = context.WithTimeout(context.Background(), conf.QueryTimeout)
			rows, queryErr := tx.Query(ctx, fmt.Sprintf("SELECT value from %s;", up.TableName))
			if queryErr != nil {
				return queryErr
			}

			defer rows.Close()
	
			if rows.Next() {
				var scanVal int64
				scanErr := rows.Scan(&scanVal)
				if scanErr != nil {
					return scanErr
				}
		
				if (scanVal + 1) < up.index {
					anomaly = LostTransaction
					up.index = scanVal + 1
				} else if (scanVal + 1) > up.index {
					anomaly = GhostTransaction
					up.index = scanVal + 1
				}
			} else {
				rowsErr := rows.Err()
				if rowsErr != nil {
					return rowsErr
				}
				anomaly = LostTransaction
			}

			return nil
		}()

		if queryErr != nil {
			return anomaly, queryErr 
		}
	}

	_, txErr = tx.Exec(ctx, fmt.Sprintf("UPDATE %s SET value = $1;", up.TableName), up.index)
	if txErr != nil {
		return anomaly, txErr
	}

	ctx, _ = context.WithTimeout(context.Background(), conf.QueryTimeout)
	commErr := tx.Commit(ctx)
	if commErr != nil {
		return anomaly, commErr
	}

	up.index += 1;

	return anomaly, nil
}

func (up *Updater) Cleanup(conf *config.PgClientConfig) error {
	ctx, _ := context.WithTimeout(context.Background(), conf.ConnectionTimeout)
	conn, connErr := pgx.Connect(ctx, conf.GetConnStr())
	if connErr != nil {
		return connErr
	}

	defer func() {
		ctx, _ := context.WithTimeout(context.Background(), conf.ConnectionTimeout)
		conn.Close(ctx)
	}()

	ctx, _ = context.WithTimeout(context.Background(), conf.QueryTimeout)
	tx, txErr := conn.Begin(ctx)
	if txErr != nil {
		return txErr
	}

	ctx, _ = context.WithTimeout(context.Background(), conf.QueryTimeout)
	_, txErr = tx.Exec(ctx, fmt.Sprintf("DROP TABLE %s;", up.TableName))
	if txErr != nil {
		return txErr
	}

	ctx, _ = context.WithTimeout(context.Background(), conf.QueryTimeout)
	commErr := tx.Commit(ctx)
	return commErr
}

func (up *Updater) Id() string {
	return "Updater"
}