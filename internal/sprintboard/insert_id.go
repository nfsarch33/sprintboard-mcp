package sprintboard

// insertReturningID runs an INSERT and returns the new row id.
// PostgreSQL (pgx) does not support sql.Result.LastInsertId; use RETURNING id instead.
func (s *Store) insertReturningID(query string, args ...any) (int64, error) {
	if s.Dialect() == DialectPostgres {
		var id int64
		err := s.db.QueryRow(query+" RETURNING id", args...).Scan(&id)
		return id, err
	}
	res, err := s.db.Exec(query, args...)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}
