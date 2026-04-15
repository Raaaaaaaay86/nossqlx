# nossqlx

`nossqlx` 是 nos 生態系的 SQL 客戶端擴充套件。它封裝了 [pgx](https://github.com/jackc/pgx)（PostgreSQL）與 [sqlx](https://github.com/jmoiron/sqlx)（MySQL），並提供統一的 `Session` / `BeginTx` API，能在 context 中已有進行中的事務時，自動將普通連線提升為同一事務連線。

## 安裝

```bash
go get github.com/raaaaaaaay86/nossqlx
```

## 支援的資料庫

| 資料庫     | Driver                          |
|------------|---------------------------------|
| PostgreSQL | `pgx/v5`                        |
| MySQL      | `go-sql-driver/mysql` + `sqlx`  |

## 快速開始

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

`Session` 從 context 中取得執行器（普通連線或已存在的 `Tx`），並以 `ClientConfig.SQLTimeout` 建立一個帶有逾時的 context。每個 repository 方法都應呼叫它。

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

> `runner` 滿足 **sqlc** 生成程式碼所需的 `MySQLRunner` / `PostgreRunner` 介面。

## 事務

在 **use-case 層**使用 `BeginTx`，將多個 repository 操作包裝成原子事務。`BeginTx` 會將 `*Transaction` 注入 context；所有下游的 `Session` 呼叫都會偵測到它並重用同一個 `Tx`。

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
    // 發生任何錯誤 → 自動 rollback
    // 成功          → 自動 commit
}
```

### 巢狀事務

`BeginTx` 支援巢狀呼叫。內層事務會重用最外層 `BeginTx` 所開啟的連線。任何一層回傳錯誤，整個事務都會 rollback。

```go
nossqlx.BeginTx(ctx, func(ctx context.Context) error {
    _ = repoA.Insert(ctx, ...)

    return nossqlx.BeginTx(ctx, func(ctx context.Context) error {
        return repoB.Insert(ctx, ...)
    })
})
```

## API 說明

### `ClientConfig`

| 欄位         | 型別            | 說明                  |
|--------------|-----------------|-----------------------|
| `Host`       | `string`        | 資料庫主機            |
| `Port`       | `int`           | 資料庫埠號            |
| `Database`   | `string`        | 資料庫 / schema 名稱  |
| `Username`   | `string`        | 使用者名稱            |
| `Password`   | `string`        | 密碼                  |
| `SQLTimeout` | `time.Duration` | 每次 Session 的逾時   |

### `PostgreClient`

| 方法           | 回傳值                              | 說明                           |
|----------------|-------------------------------------|--------------------------------|
| `Session(ctx)` | `ctx, cancel, PostgreRunner, error` | 開啟 Session（自動感知事務）   |
| `Pool()`       | `*pgxpool.Pool`                     | 取得原始 pgx 連線池            |

### `MySQLClient`

| 方法           | 回傳值                            | 說明                           |
|----------------|-----------------------------------|--------------------------------|
| `Session(ctx)` | `ctx, cancel, MySQLRunner, error` | 開啟 Session（自動感知事務）   |
| `DB()`         | `*sqlx.DB`                        | 取得原始 sqlx DB               |

### `BeginTx`

```go
func BeginTx(ctx context.Context, fn func(ctx context.Context) error) error
```

在資料庫事務中執行 `fn`。成功時 commit，失敗時 rollback。
