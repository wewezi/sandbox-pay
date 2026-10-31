package db

import "strings"

// ProjectScopeSQL keeps nil (unrestricted) distinct from an empty allowed set.
// column is a trusted, hard-coded SQL identifier supplied by repository code.
func ProjectScopeSQL(column string, ids []int64) (string, []any) {
	if ids == nil {
		return "1=1", nil
	}
	if len(ids) == 0 {
		return "0=1", nil
	}
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	return column + " IN (" + strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",") + ")", args
}
func (w *Where) AddProjectScope(column string, ids []int64) {
	if ids != nil {
		clause, args := ProjectScopeSQL(column, ids)
		w.Add(clause, args...)
	}
}
func firstProjectScope(scopes [][]int64) []int64 {
	if len(scopes) > 0 {
		return scopes[0]
	}
	return nil
}
