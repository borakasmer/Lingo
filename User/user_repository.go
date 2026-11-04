package User

import (
	"context"
	"database/sql"
	"sort"
	"sqlOrm/Models"
	"sqlOrm/Repository"
	"strings"
)

type UserRepository struct {
	db         *sql.DB
	repository *Repository.Repository[Models.Person]
}

func NewUserRepository(db *sql.DB) (*UserRepository, error) {
	/*repository, err := Repository.NewRepository[Models.Person](
		db,
		"dbo.GoUsers",
	)*/
	repository, err := Repository.NewRepository[Models.Person](db)
	if err != nil {
		return nil, err
	}

	return &UserRepository{
		db:         db,
		repository: repository,
	}, nil
}

func (repo *UserRepository) GetByName(
	ctx context.Context,
	name string,
) ([]Models.Person, error) {
	return repo.repository.FindMany(
		ctx,
		Repository.Filter{
			Field: "Name",
			Op:    Repository.OpLike,
			Value: "%" + name + "%",
		},
		Repository.Filter{
			Field: "IsDeleted",
			Op:    Repository.OpEq,
			Value: false,
		},
	)
}

func (repo *UserRepository) GetAllWithPaging(
	ctx context.Context,
	page int,
	pageSize int,
	sortBy string,
	ascending bool,
) ([]Models.Person, error) {
	sortBy = strings.TrimSpace(sortBy)
	if sortBy == "" {
		sortBy = "Id"
	}

	return repo.repository.GetAllWithPaging(
		ctx,
		page,
		pageSize,
		sortBy,
		ascending,
	)
}

func (repo *UserRepository) SearchUsers(
	ctx context.Context,
	gender string,
	age int,
	name string,
) ([]Models.Person, error) {
	return repo.repository.FindMany(
		ctx,
		Repository.Filter{
			Field: "IsDeleted",
			Op:    Repository.OpEq,
			Value: false,
		},
		Repository.And(
			Repository.Filter{
				Field: "Gender",
				Op:    Repository.OpEq,
				Value: gender,
			},
			Repository.Or(
				Repository.Filter{
					Field: "Age",
					Op:    Repository.OpGt,
					Value: age,
				},
				Repository.Filter{
					Field: "Name",
					Op:    Repository.OpLike,
					Value: "%" + name + "%",
				},
			),
		),
	)
}

func (repo *UserRepository) AppendXToNamesOver40(
	ctx context.Context,
	batchSize int,
) (int64, error) {
	users, err := repo.repository.FindMany(
		ctx,
		Repository.Filter{
			Field: "Age",
			Op:    Repository.OpGt,
			Value: 40,
		},
	)
	if err != nil {
		return 0, err
	}

	pairs := make(
		[]Repository.UpdatePair[Models.Person],
		0,
		len(users),
	)

	for _, oldUser := range users {
		newUser := oldUser
		newUser.Name += "X"

		pairs = append(pairs, Repository.UpdatePair[Models.Person]{
			Old: oldUser,
			New: newUser,
		})
	}

	return repo.repository.BatchUpdateChanged(
		ctx,
		pairs,
		"Id",
		batchSize,
	)
}

func (repo *UserRepository) GetUserPermissions(
	ctx context.Context,
	userID int,
	controllerID int,
) ([]string, error) {

	type permissionResult struct {
		ActionNumberTotal int
		ActionNumber      int
		ActionName        string
	}

	users, err := Repository.NewRepository[Models.PermissionUser](repo.db)
	if err != nil {
		return nil, err
	}

	userActions, err := Repository.NewRepository[Models.UserAction](repo.db)
	if err != nil {
		return nil, err
	}

	actions, err := Repository.NewRepository[Models.ActionDefinition](repo.db)
	if err != nil {
		return nil, err
	}

	query := users.Join("u").
		InnerJoin(
			userActions.JoinSource("ua"),
			Repository.EqColumn("ua.IdUser", "u.Id"),
		).
		InnerJoin(
			actions.JoinSource("a"),
			Repository.EqColumn(
				"a.IdGoController",
				"ua.IdGoController",
			),
		).
		Where(
			Repository.Eq("u.Id", userID),
			Repository.Eq("ua.IdGoController", controllerID),
			Repository.Gt("a.ActionNumber", 0),
		).
		Select(
			Repository.SelectColumn{
				Field: "ua.ActionNumberTotal",
				As:    "ActionNumberTotal",
			},
			Repository.SelectColumn{
				Field: "a.ActionNumber",
				As:    "ActionNumber",
			},
			Repository.SelectColumn{
				Field: "a.ActionName",
				As:    "ActionName",
			},
		)

	rows, err := Repository.JoinFindMany[permissionResult](ctx, query)
	if err != nil {
		return nil, err
	}

	// JOIN'de henüz OrderBy olmadığı için sonucu burada sıralıyoruz.
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].ActionNumber == rows[j].ActionNumber {
			return rows[i].ActionName < rows[j].ActionName
		}

		return rows[i].ActionNumber < rows[j].ActionNumber
	})

	names := make([]string, 0)
	seen := make(map[string]bool)

	for _, row := range rows {
		// Kullanıcının maskesi bu action'ın tüm bitlerini içeriyor mu?
		// BITWISE AND işlemi ile kontrol ediyoruz.
		if row.ActionNumberTotal&row.ActionNumber != row.ActionNumber {
			continue
		}

		if !seen[row.ActionName] {
			names = append(names, row.ActionName)
			seen[row.ActionName] = true
		}
	}

	return names, nil
}

func (repo *UserRepository) InsertSampleUsers(
	ctx context.Context,
) (int64, error) {
	type userInsert struct {
		Name     string
		Age      int
		UserName string `db:"Username"`
		Password string
		Gender   string
	}

	insertRepository, err := Repository.NewRepository[userInsert](
		repo.db,
		"dbo.GoUsers",
	)
	if err != nil {
		return 0, err
	}

	users := []userInsert{
		{
			Name:     "X",
			UserName: "sample_x",
			Password: "demo-only-not-for-login",
			Age:      25,
			Gender:   "Male",
		},
		{
			Name:     "Y",
			UserName: "sample_y",
			Password: "demo-only-not-for-login",
			Age:      30,
			Gender:   "Female",
		},
		{
			Name:     "Z",
			UserName: "sample_z",
			Password: "demo-only-not-for-login",
			Age:      35,
			Gender:   "Male",
		},
	}

	return insertRepository.BulkInsertBatched(ctx, users, 3)
}
func (repo *UserRepository) DeleteUsersWithFilter(
	ctx context.Context,
	filters ...Repository.Expr,
) (int64, error) {
	return repo.repository.DeleteWhere(ctx, filters...)
}
