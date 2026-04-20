package postgres

import (
	"iter"

	"github.com/jackc/pgx/v5"
)

// scanRows превращает pgx.Rows в iter.Seq2[T, error].
// Ошибка сканирования или ошибка итерации уходит в yield как вторым значением, вызывающий решает, прерываться или продолжать.
func scanRows[T any](rows pgx.Rows, scan func(pgx.Rows) (T, error)) iter.Seq2[T, error] {
	return func(yield func(T, error) bool) {
		defer rows.Close()

		for rows.Next() {
			v, err := scan(rows)
			if !yield(v, err) {
				return
			}
			if err != nil {
				return
			}
		}

		if err := rows.Err(); err != nil {
			var zero T
			yield(zero, err)
		}
	}
}

// collectRows собирает все элементы iter.Seq2 в слайс.
// Прерывается на первой ошибке.
func collectRows[T any](seq iter.Seq2[T, error]) ([]T, error) {
	var out []T
	for v, err := range seq {
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, nil
}
