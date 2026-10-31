package db

import "time"

// addDateRange applies local calendar dates with an exclusive next-midnight
// upper bound. Explicit database timestamps retain the historical UTC behavior.
// column is an internal SQL identifier, never user input.
func addDateRange(w *Where, column, dateFrom, dateTo string, utcOffset int) {
	if dateFrom != "" {
		start := parseDateStart(dateFrom)
		if _, err := time.Parse("2006-01-02", dateFrom); err == nil {
			start = start.Add(-time.Duration(utcOffset) * time.Minute)
			w.Add("julianday("+column+") >= julianday(?)", toSQL(start))
		} else {
			w.Add(column+" >= ?", toSQL(start))
		}
	}
	if dateTo != "" {
		if end, err := time.Parse("2006-01-02", dateTo); err == nil {
			end = end.AddDate(0, 0, 1).Add(-time.Duration(utcOffset) * time.Minute)
			w.Add("julianday("+column+") < julianday(?)", toSQL(end))
		} else {
			w.Add(column+" <= ?", toSQL(parseDateEnd(dateTo)))
		}
	}
}
