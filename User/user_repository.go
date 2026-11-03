package User

import (
	"context"
	"database/sql"
	"strings"

	"sqlOrm/Models"
	"sqlOrm/Repository"
)

type UserRepository struct {
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
