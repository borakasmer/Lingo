package Models

type Person struct {
	Id        int    `json:"id"`
	Name      string `json:"name"`
	Age       int    `json:"age"`
	Gender    string `json:"gender"`
	IsDeleted bool   `json:"isDeleted"`
}

func (Person) TableName() string {
	return "dbo.GoUsers"
}

type RawValue struct {
	Expr string
}
