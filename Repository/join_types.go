package Repository

// JoinType, desteklenen tablo birleştirme türüdür.
type joinType string

const (
	joinInner joinType = "INNER JOIN"
	joinLeft  joinType = "LEFT JOIN"
	joinRight joinType = "RIGHT JOIN"
)

// joinSource, sorguya katılan tablonun bilgilerini tutar.
// Alan bilgileri mevcut modelFieldsCache üzerinden alınacak.
type joinSource struct {
	table  string
	alias  string
	fields []modelField
}

// joinClause, JOIN türünü, katılan tabloyu ve ON koşullarını tutar.
type joinClause struct {
	kind   joinType
	source joinSource
	on     []Expr
}
