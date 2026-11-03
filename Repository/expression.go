package Repository

// Expr, tek bir filtreyi veya filtre grubunu temsil eder.
type Expr interface {
	isExpr()
}

// Filter da bir sorgu ifadesidir.
func (Filter) isExpr() {}

type Logic string

const (
	LogicAnd Logic = "AND"
	LogicOr  Logic = "OR"
)

type Group struct {
	Logic Logic
	Items []Expr
}

func (Group) isExpr() {}

func And(items ...Expr) Expr {
	return Group{
		Logic: LogicAnd,
		Items: items,
	}
}

func Or(items ...Expr) Expr {
	return Group{
		Logic: LogicOr,
		Items: items,
	}
}
