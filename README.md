<p align="center">
  <img src="assets/readme/hero.png" alt="Lingo — LINQ-inspired intent. Explicit SQL behavior. A lightweight SQL helper for Go." width="100%">
</p>

<h1 align="center">Lingo</h1>
<p align="center"><strong>A LINQ-inspired SQL helper for Go and SQL Server.</strong><br>
Generic repositories, composable filters, changed-field updates, and explicit transaction control — built on <code>database/sql</code>.</p>

<p align="center">
  <a href="#quick-start">Quick start</a> ·
  <a href="#compose-your-query">Query examples</a> ·
  <a href="#update-only-what-changed">Changed-field updates</a> ·
  <a href="#explore-with-swagger">Swagger</a> ·
  <a href="#api-at-a-glance">API reference</a>
</p>

---

## Small surface. Useful building blocks.

| 🧩 Compose | 🛠️ Update | ⚙️ Control |
|---|---|---|
| Nested **AND / OR** expressions | Write explicit fields or compare **Old / New** | One shared SQL connection pool |
| Parameterized filter values | Skip identity updates | Standard `context.Context` |
| Paging and model-based sorting | Transactional batch updates | Cached model metadata |
| Typed result models | Multi-row INSERT and optional TVP | A runnable `net/http` + Swagger demo |

The idea is simple: express the query intent in Go, keep the generated SQL understandable, and make transaction boundaries visible. Result models are generic; field names and operators are checked at runtime.

<p align="center">
  <img src="assets/readme/query-flow.gif" alt="Illustrated query flow: UserRepository defines the intent, Repository builds SQL and parameters, SQL Server executes it, and typed model values are returned." width="100%">
</p>

## Quick start

### 1. Prepare the project

Clone this repository and open a terminal in its root directory. Use the Go version declared in `go.mod` and a reachable SQL Server instance.

```bash
go mod download
```

Set the connection string in your terminal or IDE run configuration:

```bash
export SQL_CONNECTION_STRING='server=localhost;port=1433;user id=demo_user;password=YOUR_PASSWORD;database=Customer;encrypt=true'
```

The demo reads this environment variable. The repository receives an open `*sql.DB`; connection credentials are not part of its query API. Keep real credentials out of source control.

<details>
<summary><strong>Optional: create a minimal table in a fresh demo database</strong></summary>

This schema is for a new demo database, not a migration for an existing application table.

```sql
CREATE TABLE dbo.GoUsers (
    Id        INT IDENTITY(1,1) PRIMARY KEY,
    Name      NVARCHAR(100) NOT NULL,
    Age       INT NOT NULL,
    Gender    NVARCHAR(20) NOT NULL,
    IsDeleted BIT NOT NULL DEFAULT 0
);

INSERT INTO dbo.GoUsers (Name, Age, Gender)
VALUES (N'Bora', 48, N'Male'),
       (N'Secil', 42, N'Female'),
       (N'Duru', 14, N'Female');
```

For a local server with a self-signed certificate, its connection settings may need `TrustServerCertificate=true`. Use a properly configured certificate for a deployed server.

</details>

### 2. Describe a model

```go
package Models

type Person struct {
    Id        int    `json:"id"`
    Name      string `json:"name"`
    Age       int    `json:"age"`
    Gender    string `json:"gender"`
    IsDeleted bool   `json:"-"`
}

func (Person) TableName() string {
    return "dbo.GoUsers"
}
```

`T` is a flat, non-pointer struct. The same repository works with another model and table, such as `Product` and `dbo.Products`.

### 3. Open once. Reuse the pool.

The following setup belongs at application startup. Imports use the project's local module name, `sqlOrm`.

```go
import (
    "context"
    "log"
    "os"
    "time"

    "sqlOrm/DB"
    "sqlOrm/Models"
    "sqlOrm/Repository"
)
```

Inside `main()`:

```go
connectionString := os.Getenv("SQL_CONNECTION_STRING")
if connectionString == "" {
    log.Fatal("SQL_CONNECTION_STRING is required")
}

connectionCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
db, err := DB.SqlOpen(connectionCtx, connectionString)
cancel()
if err != nil {
    log.Fatal(err)
}
defer db.Close()

repo, err := Repository.NewRepository[Models.Person](db)
if err != nil {
    log.Fatal(err)
}
```

The table name comes from `TableName()`. An explicit override is also supported:

```go
repo, err := Repository.NewRepository[Models.Person](db, "dbo.GoUsers")
```

Use a fresh request/query context for operations; the startup context above is already canceled after the connection check.

```go
ctx, cancelQuery := context.WithTimeout(context.Background(), 5*time.Second)
defer cancelQuery()

users, err := repo.FindMany(ctx,
    Repository.Filter{Field: "Name", Op: Repository.OpLike, Value: "%Bor%"},
)
```

In an HTTP handler, pass `r.Context()` to propagate request cancellation.

## Compose your query

### Gender matches **AND** (older than the limit **OR** name contains text)

<p align="center">
  <img src="assets/readme/query-composition.png" alt="Query composition: Gender matches AND a grouped expression where Age exceeds the threshold OR Name contains the search text. Active-row filtering is added automatically." width="100%">
</p>

```go
users, err := repo.FindMany(ctx,
    Repository.And(
        Repository.Filter{Field: "Gender", Op: Repository.OpEq, Value: "Male"},
        Repository.Or(
            Repository.Filter{Field: "Age", Op: Repository.OpGt, Value: 40},
            Repository.Filter{Field: "Name", Op: Repository.OpLike, Value: "%Bor%"},
        ),
    ),
)
```

Equivalent SQL shape, with the selected model columns omitted here for readability:

```sql
SELECT ...
FROM [dbo].[GoUsers]
WHERE (([Gender] = @p1 AND ([Age] > @p2 OR [Name] LIKE @p3)))
  AND ISNULL([IsDeleted], 0) = 0
ORDER BY [Id] ASC;
```

Values are sent separately: `Male`, `40`, `%Bor%`. Multiple filters supplied directly to a read method are combined with **AND**. `%` and `_` in an `OpLike` value retain their SQL wildcard meaning.

<details>
<summary><strong>Coming from C#? Compare the query intent</strong></summary>

```csharp
var users = db.Users
    .Where(x => x.Gender == "Male" &&
        (x.Age > 40 || x.Name.Contains("Bor")))
    .ToList();
```

The Go version expresses the same predicate through `Filter`, `And`, and `Or`. Field names are strings checked against model metadata; this is LINQ-inspired composition rather than a C# expression-tree translator.

</details>

### Available operators

| Operator | SQL | Meaning |
|---|---|---|
| `OpEq` | `=` | Equal |
| `OpNe` | `<>` | Not equal |
| `OpGt` / `OpGte` | `>` / `>=` | Greater than / at least |
| `OpLt` / `OpLte` | `<` / `<=` | Less than / at most |
| `OpLike` | `LIKE` | SQL pattern matching |

For an untyped `nil` value, `OpEq` produces `IS NULL` and `OpNe` produces `IS NOT NULL`.

## Paging + sorting + filters

```go
users, err := repo.GetAllWithPaging(
    ctx,
    2,      // page: skip the first page
    10,     // pageSize
    "Age",  // sorting field
    false,  // descending
    Repository.Filter{Field: "Name", Op: Repository.OpLike, Value: "%a%"},
)
```

| Input to `GetAllWithPaging` | Behavior |
|---|---|
| `page < 1` | Use page `1` |
| `pageSize < 1` | Use `10` |
| `pageSize > 100` | Use `100` |
| No sorting field on a model with `Id` | `Id ASC` |
| Invalid sorting field | Return an error |
| `CreatedDate` sorting column missing in SQL | List/paging query retries with `Id`, keeping filters, direction, and paging |

All mapped fields may be used for sorting. For repeatable paging, prefer a unique sorting field such as `Id`; equal sort values have no guaranteed relative order.

The compact form is available too:

```go
users, err := repo.FindMany(ctx, 2, 10, "Age", false)
```

The default/cap normalization above belongs to `GetAllWithPaging`; direct `FindMany` paging arguments must be positive. One integer passed to `FindMany` means page size, with page `1`.

## Update only what changed

<p align="center">
  <img src="assets/readme/batch-update.png" alt="Changed-field update: read active users older than 40, compare Bora with BoraX, and update only Name. For 250 changed rows and batchSize 100, there are 250 UPDATE executions in one transaction and one commit." width="100%">
</p>

### Different values per user: `BatchUpdateChanged`

```go
func appendXToNamesOver40(
    ctx context.Context,
    repo *Repository.Repository[Models.Person],
    batchSize int,
) (int64, error) {
    users, err := repo.FindMany(ctx,
        Repository.Filter{Field: "Age", Op: Repository.OpGt, Value: 40},
    )
    if err != nil {
        return 0, err
    }

    pairs := make([]Repository.UpdatePair[Models.Person], 0, len(users))
    for _, oldUser := range users {
        newUser := oldUser
        newUser.Name += "X"

        pairs = append(pairs, Repository.UpdatePair[Models.Person]{
            Old: oldUser,
            New: newUser,
        })
    }

    return repo.BatchUpdateChanged(ctx, pairs, "Id", batchSize)
}
```

For each pair, the helper compares the **supplied** Old/New models. It does not fetch the old row again or perform automatic optimistic concurrency checks.

Only changed fields enter `SET`:

```sql
UPDATE [dbo].[GoUsers]
SET [Name] = @p1
WHERE ([Id] = @p2)
  AND ISNULL([IsDeleted], 0) = 0;
```

**Know exactly what `batchSize` means:**

- It groups the UPDATE commands processed by the loop.
- Every changed record still uses its own UPDATE execution.
- Groups share one transaction and a prepared-statement cache.
- Commit happens once, after every group succeeds; a failure rolls back the transaction.
- `batchSize = 1` and `batchSize = 100` do not combine commands or reduce SQL round trips.
- Omitted or non-positive batch size means one group. Unchanged records issue no UPDATE.

For **250 changed records**, both sizes still mean **250 UPDATE executions, one transaction, one commit**. Grouping alone is not a network-performance optimization.

### Same operation for all matches: `BulkUpdateWithFilter`

Append `X` to all active users older than 40 with **one SQL UPDATE**, without reading them first:

```go
affected, err := repo.BulkUpdateWithFilter(
    ctx,
    map[string]any{
        "Name": Models.RawValue{Expr: "[Name] + N'X'"},
    },
    Repository.Filter{Field: "Age", Op: Repository.OpGt, Value: 40},
)
```

```sql
UPDATE [dbo].[GoUsers]
SET [Name] = [Name] + N'X'
WHERE ([Age] > @p1)
  AND ISNULL([IsDeleted], 0) = 0;
```

`RawValue` is an explicit SQL escape hatch for expressions defined by the developer. Never populate its expression from request input. Re-running either append example adds another `X`.

<details>
<summary><strong>More write examples: Insert, Update, UpdateChanged, and BatchUpdate</strong></summary>

**Insert one model.** The Go field named `Id` is skipped as an identity field.

```go
affected, err := repo.Insert(ctx, Models.Person{
    Name: "New user", Age: 28, Gender: "Female",
})
```

**Update explicitly supplied fields.** Zero, false, and nil values are retained.

```go
affected, err := repo.Update(ctx,
    map[string]any{"Age": 0, "Name": "Updated name"},
    Repository.Filter{Field: "Id", Op: Repository.OpEq, Value: 16},
)
```

**Compare one pair.**

```go
oldUser, err := repo.GetByID(ctx, 16)
if err != nil {
    return err
}
newUser := *oldUser
newUser.Name += "X"

affected, err := repo.UpdateChanged(ctx, *oldUser, newUser,
    Repository.Filter{Field: "Id", Op: Repository.OpEq, Value: oldUser.Id},
)
```

**Write different fields/filters in one transaction.**

```go
affected, err := repo.BatchUpdate(ctx, []Repository.UpdateItem{
    {
        Values: map[string]any{"Age": 49},
        Filters: []Repository.Expr{
            Repository.Filter{Field: "Id", Op: Repository.OpEq, Value: 16},
        },
    },
    {
        Values: map[string]any{"Name": "Updated name"},
        Filters: []Repository.Expr{
            Repository.Filter{Field: "Id", Op: Repository.OpEq, Value: 17},
        },
    },
}, 100)
```

`BatchUpdate` writes what you explicitly provide. `BatchUpdateChanged` first computes changes from Old/New pairs, then uses the batch-update pipeline. Neither silently re-reads the old database state.

</details>

## Insert multiple rows

```go
affected, err := repo.BulkInsertBatched(ctx, []Models.Person{
    {Name: "First user", Age: 25, Gender: "Female"},
    {Name: "Second user", Age: 32, Gender: "Male"},
}, 100)
```

This method builds **multi-row INSERT statements**, splitting them to respect SQL Server's parameter/row limits. All INSERT groups share one transaction. This differs from batch UPDATE, which executes one UPDATE per changed record.

<details>
<summary><strong>Advanced: TVP / stored-procedure insert</strong></summary>

`BulkInsert` is the optional table-valued parameter path:

```go
err := repo.BulkInsert(ctx, rows, "dbo.UserTableType", "dbo.InsertUsersBulk", 100)
```

Create the SQL table type and stored procedure first. Use a dedicated row model if needed: TVP struct fields must match the SQL table type's column order and types. This path passes the full `T`; normal INSERT's automatic `Id` omission does not apply. The procedure determines the target table and its internal SQL behavior.

</details>

## Explore with Swagger

The included demo uses `net/http` and your generic repository through a thin `UserRepository` / `UserHandler` layer.

```bash
go run github.com/swaggo/swag/cmd/swag@latest init -g main.go -o docs
go run .
```

Open **[Swagger UI](http://localhost:8080/swagger/index.html)**, select **Try it out**, enter values, and execute.

| Method | Endpoint | Demonstrates |
|---|---|---|
| `GET` | `/api/users?name=Bor` | Name contains text |
| `GET` | `/api/users/paging?page=1&pageSize=4&sortBy=Age&ascending=false` | Paging and sorting |
| `GET` | `/api/users/search?gender=Male&age=40&name=Bor` | **Gender AND (Age OR Name)** |
| `POST` | `/api/users/append-x-over-40?batchSize=100` | Changed-field batch update |

Example write response:

```json
{
  "updatedCount": 2
}
```

The POST endpoint modifies the demo data. A zero affected-row count is valid and does not itself mean an error.

## API at a glance

| Method | Purpose | Result |
|---|---|---|
| `NewRepository[T]` | Bind a model, pool, and optional table name | `*Repository[T], error` |
| `Get` / `FindMany` | Read with filters, order, optional paging | `[]T, error` |
| `GetAll` | Read active rows | `[]T, error` |
| `FindOne` / `GetByID` | Read one active row | `*T, error` |
| `GetAllWithPaging` | Normalize paging inputs and read | `[]T, error` |
| `Insert` | Insert one model | `int64, error` |
| `BulkInsertBatched` | Transactional multi-row INSERT groups | `int64, error` |
| `BulkInsert` | Optional TVP / procedure insert | `error` |
| `Update` / `BulkUpdateWithFilter` | Explicit fields and predicates | `int64, error` |
| `UpdateChanged` | Compare one Old/New pair | `int64, error` |
| `BatchUpdate` | Transactional explicit update items | `int64, error` |
| `BatchUpdateChanged` | Transactional Old/New pairs | `int64, error` |

## Behavior you can rely on

- **Active rows:** SELECT and UPDATE add `ISNULL([IsDeleted], 0) = 0`. Target tables need that column. An extra `IsDeleted = false` filter would exclude NULL rows, so it is unnecessary for normal active-row queries.
- **Models and tags:** Go field names are the default SQL names. `db:"ColumnName"` overrides them; `db:"-"` skips mapping. `json` controls JSON serialization only. Nullable SQL values need compatible pointer or `sql.Null*` model fields.
- **Identity protection:** The Go field named `Id` is skipped during normal INSERT and UPDATE. In changed-pair batches, the matching identifier is excluded from SET and the original key locates the row.
- **Explicit predicates:** Update methods reject an empty WHERE expression. The automatic soft-delete condition does not count as the explicit filter. A broad predicate can still match many rows.
- **Results:** Empty list reads return `[]`; single reads return `sql.ErrNoRows`. Write counts are affected rows, not generated identity values.
- **Reflection:** Field metadata is cached by model type. Scanning values and comparing models still use reflection. The cache is not a database schema cache.
- **Scope:** This project demonstrates repository/query helpers. It does not provide migrations, relationship loading, joins, a general expression translator, or an automatic entity-tracking session. Comparisons are explicit Old/New operations.

<details>
<summary><strong>Single-record error handling</strong></summary>

```go
user, err := repo.GetByID(ctx, 16)
if errors.Is(err, sql.ErrNoRows) {
    // Return HTTP 404, for example.
    return nil
}
if err != nil {
    return err
}
// Use user here.
```

This fragment assumes an error-returning function and imports `errors` and `database/sql`.

</details>

## Project map

| Location | Responsibility |
|---|---|
| `DB/sql.go` | Open and verify a SQL Server connection pool |
| `Models/` | Demo `Person` model and `RawValue` |
| `Repository/operator.go` | Supported comparison operators |
| `Repository/filter.go` | Field/operator/value filters |
| `Repository/expression.go` | AND / OR groups |
| `Repository/model_helpers.go` | Field mapping, metadata cache, row scanning |
| `Repository/query_helpers.go` | Parameterized WHERE construction |
| `Repository/repository.go` | Reads, writes, paging, transactions |
| `User/` | Demo repository and HTTP handlers |
| `docs/` | Generated Swagger specification |
| `main.go` | Application startup, pool lifecycle, HTTP routes |

### Publishing and local development

Lingo is the project name. Examples retain the current local module name `sqlOrm`. When publishing under a GitHub module path, update `go.mod` and internal imports together. The demo API lives in the same module; import its `Repository` subpackage from other applications after publishing.

The local `run-local.sh` setup can source your shell environment before `go run .`. It is a convenience for launching the demo, not a Go debugger configuration. Rebuild Swagger files after editing endpoint annotations.

For the legacy SQL Server certificate error `x509: negative serial number`, the demo's local compatibility setting is `GODEBUG=x509negativeserial=1`. That setting permits the certificate to be parsed; it does not repair the certificate.

---

Built by **[Bora Kaşmer](https://github.com/borakasmer)** · [borakasmer.com](https://borakasmer.com/)

*Typed models. Readable intent. Explicit SQL behavior.*
