package Models

type PermissionUser struct {
	Id        int
	IsDeleted bool
}

func (PermissionUser) TableName() string {
	return "dbo.GoUsers"
}

type UserAction struct {
	IdUser            int
	IdGoController    int
	ActionNumberTotal int
	IsDeleted         bool
}

func (UserAction) TableName() string {
	return "dbo.GoUserActions"
}

type UserPermissionResult struct {
	ActionNumberTotal int
}

type UserPermissionResponse struct {
	Actions []string `json:"actions"`
}
