package db

// GetDB returns the underlying DBTX interface from a Queries object.
// This allows access to raw SQL query methods like QueryRowContext, QueryContext, and ExecContext.
func (q *Queries) GetDB() DBTX {
	return q.db
}
