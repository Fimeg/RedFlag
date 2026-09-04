package queries

import (
	"github.com/doug-martin/goqu/v9"
	"github.com/doug-martin/goqu/v9/exp"
	_ "github.com/doug-martin/goqu/v9/dialect/postgres"
	"github.com/jmoiron/sqlx"
)

func init() {
	goqu.SetDefaultPrepared(true) // always use $1, $2 style placeholders
}

// PG returns a postgres dialect for building goqu queries.
func PG() goqu.DialectWrapper {
	return goqu.Dialect("postgres")
}

// Paginated runs SELECT COUNT(*) and a paginated SELECT from the same base dataset.
// base should have From + Where applied. goqu datasets are immutable so the call
// does not mutate your builder.
// Returns total count and fills dest via sqlx.Select.
func Paginated(db sqlx.Queryer, base *goqu.SelectDataset, page, pageSize uint, order exp.OrderedExpression, columns []string, dest interface{}) (int, error) {
	var total int

	countSQL, countArgs, err := base.Select(goqu.COUNT("*")).ToSQL()
	if err != nil {
		return 0, err
	}
	if err := sqlx.Get(db, &total, countSQL, countArgs...); err != nil {
		return 0, err
	}

	cols := make([]interface{}, len(columns))
	for i, c := range columns {
		cols[i] = c
	}
	dataSQL, dataArgs, err := base.Select(cols...).Order(order).Limit(pageSize).Offset((page-1)*pageSize).ToSQL()
	if err != nil {
		return 0, err
	}
	if err := sqlx.Select(db, dest, dataSQL, dataArgs...); err != nil {
		return 0, err
	}

	return total, nil
}
