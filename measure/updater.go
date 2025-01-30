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

	ctx, _ = context.WithTimeout(context.Background(), conf.QueryTimeout)
	commErr := tx.Commit(ctx)
	return commErr
}

func (up *Updater) Run(conf *config.PgClientConfig) (bool, error) {
	ctx, _ := context.WithTimeout(context.Background(), conf.ConnectionTimeout)
	conn, connErr := pgx.Connect(ctx, conf.GetConnStr())
	if connErr != nil {
		return false, connErr
	}

	defer func() {
		ctx, _ := context.WithTimeout(context.Background(), conf.ConnectionTimeout)
		conn.Close(ctx)
	}()

	ctx, _ = context.WithTimeout(context.Background(), conf.QueryTimeout)
	tx, txErr := conn.Begin(ctx)
	if txErr != nil {
		return false, txErr
	}

	lostTx := false
	//Do stuff
	if up.index > 0 {
		ctx, _ = context.WithTimeout(context.Background(), conf.QueryTimeout)
		rows, queryErr := tx.Query(ctx, fmt.Sprintf("SELECT value from %s;", up.TableName))
		if queryErr != nil {
			return false, queryErr
		}
		defer rows.Close()

		if rows.Next() {
			var scanVal int64
			scanErr := rows.Scan(&scanVal)
			if scanErr != nil {
				return false, scanErr
			}
	
			if (scanVal + 1) != up.index {
				lostTx = true
				up.index = scanVal + 1
			}
		} else {
			lostTx = true
		}
	}

	_, txErr = tx.Exec(ctx, fmt.Sprintf("UPDATE %s SET value = $1;", up.TableName), up.index)
	if txErr != nil {
		return lostTx, txErr
	}
	up.index += 1;

	ctx, _ = context.WithTimeout(context.Background(), conf.QueryTimeout)
	commErr := tx.Commit(ctx)
	if commErr != nil {
		return lostTx, commErr
	}

	return lostTx, nil
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

	//Cleanup
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