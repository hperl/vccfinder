package main

/*
func BenchmarkFindColumnsSimple(b *testing.B) {
	for i := 0; i < b.N; i++ {
		var commits []Commit
		err := FindColumns(&commits, []string{"id", "sha"}, "where repository_id = 327")
		if err != nil {
			b.Error(err)
		}
	}
}

func BenchmarkSelectWtSqlSimple(b *testing.B) {
	for i := 0; i < b.N; i++ {
		rows, err := DB.Db.Query("SELECT id, sha FROM commits WHERE repository_id = 327")
		if err != nil {
			b.Error(err)
		}
		defer rows.Close()
		for rows.Next() {
			c := new(Commit)
			if err := rows.Scan(&c.Id, &c.Sha); err != nil {
				b.Error(err)
			}
		}
		if err := rows.Err(); err != nil {
			b.Error(err)
		}
	}
}

func BenchmarkFindColumnsMessage(b *testing.B) {
	for i := 0; i < b.N; i++ {
		var commits []Commit
		err := FindColumns(&commits, []string{"id", "sha", "message"}, "where repository_id = 171")
		if err != nil {
			b.Error(err)
		}
	}
}

func BenchmarkSelectWtSqlMessage(b *testing.B) {
	for i := 0; i < b.N; i++ {
		rows, err := DB.Db.Query("SELECT id, sha, message FROM commits WHERE repository_id = 171")
		if err != nil {
			b.Errorf("initial statement: %v", err)
		}
		defer rows.Close()
		for rows.Next() {
			c := new(Commit)
			if err := rows.Scan(&c.Id, &c.Sha, &c.Message); err != nil {
				b.Error("row scan: %v", err)
			}
		}
		if err := rows.Err(); err != nil {
			b.Logf("scan done: %v", err)
			ReopenDB()
		}
	}
}
*/
