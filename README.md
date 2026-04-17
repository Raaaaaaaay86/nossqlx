# nossqlx

`nossqlx` is an extended SQL client for the nos ecosystem. It wraps [pgx](https://github.com/jackc/pgx) (PostgreSQL) and [sqlx](https://github.com/jmoiron/sqlx) (MySQL) and provides a unified `Session` / `BeginTx` API that transparently promotes a plain connection into a transaction when one is already in flight on the context.

## Installation

```bash
go get github.com/raaaaaaaay86/nossqlx
```

## Supported Databases

| Database   | Driver                          |
|------------|---------------------------------|
| PostgreSQL | `pgx/v5`                        |
| MySQL      | `go-sql-driver/mysql` + `sqlx`  |

## Quick Start

### PostgreSQL

```go
client, err := nossqlx.NewSqlxPostgreClient(nossqlx.ClientConfig{
    Host:       "localhost",
    Port:       5432,
    Database:   "mydb",
    Username:   "postgres",
    Password:   "secret",
    SQLTimeout: 5 * time.Second,
})
if err != nil {
    panic(err)
}
```

### MySQL

```go
client, err := nossqlx.NewSqlxMySQLClient(nossqlx.ClientConfig{
    Host:       "localhost",
    Port:       3306,
    Database:   "mydb",
    Username:   "root",
    Password:   "secret",
    SQLTimeout: 5 * time.Second,
})
```

## Session

`Session` acquires a runner (plain connection or an existing `Tx` from the context) and wraps it in a timeout context derived from `ClientConfig.SQLTimeout`. Use it in every repository method.

```go
func (r *MemberMySQL) Create(ctx context.Context, member entity.Member) error {
    sessCtx, cancel, runner, err := r.driver.Session(ctx)
    if err != nil {
        return err
    }
    defer cancel()

    return memberc.New(runner).CreateMember(sessCtx, po.MemberToPO(member))
}
```

> The `runner` satisfies the `MySQLRunner` / `PostgreRunner` interface expected by **sqlc**-generated code.

## Transactions

Use `BeginTx` at the **use-case layer** to wrap multiple repository calls into an atomic transaction. `BeginTx` injects a `*Transaction` into the context; every downstream `Session` call detects it and reuses the same `Tx`.

```go
func (uc *RegisterUserUsecase) Execute(ctx context.Context, cmd RegisterCommand) error {
    return nossqlx.BeginTx(ctx, func(ctx context.Context) error {
        if err := uc.usersRepo.Create(ctx, cmd.User); err != nil {
            return err
        }
        if err := uc.profileRepo.Create(ctx, cmd.Profile); err != nil {
            return err
        }
        return nil
    })
    // On any error → automatic rollback
    // On success  → automatic commit
}
```

### Nested Transactions

`BeginTx` supports nested calls. Inner transactions reuse the connection opened by the outermost `BeginTx`. If any level returns an error the whole transaction rolls back.

```go
nossqlx.BeginTx(ctx, func(ctx context.Context) error {
    _ = repoA.Insert(ctx, ...)

    return nossqlx.BeginTx(ctx, func(ctx context.Context) error {
        return repoB.Insert(ctx, ...)
    })
})
```

## API Reference

### `ClientConfig`

| Field        | Type            | Description                   |
|--------------|-----------------|-------------------------------|
| `Host`       | `string`        | Database host                 |
| `Port`       | `int`           | Database port                 |
| `Database`   | `string`        | Database / schema name        |
| `Username`   | `string`        | Username                      |
| `Password`   | `string`        | Password                      |
| `SQLTimeout` | `time.Duration` | Per-session execution timeout |

### `PostgreClient`

| Method         | Returns                             | Description                        |
|----------------|-------------------------------------|------------------------------------|
| `Session(ctx)` | `ctx, cancel, PostgreRunner, error` | Open a session (auto-tx-aware)     |
| `Pool()`       | `*pgxpool.Pool`                     | Access the raw pgx connection pool |

### `MySQLClient`

| Method         | Returns                           | Description                     |
|----------------|-----------------------------------|---------------------------------|
| `Session(ctx)` | `ctx, cancel, MySQLRunner, error` | Open a session (auto-tx-aware)  |
| `DB()`         | `*sqlx.DB`                        | Access the raw sqlx DB          |

### `BeginTx`

```go
func BeginTx(ctx context.Context, fn func(ctx context.Context) error) error
```

Executes `fn` inside a database transaction. Commits on success, rolls back on error. 
