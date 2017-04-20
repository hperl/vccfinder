package main

import (
	"database/sql"
	"fmt"
)

func SelfTest() {
	var err error

	fmt.Print("Testing redis ...  ")
	if err = Selftest(); err != nil {
		panic(err)
	}
	fmt.Print("WORKS\n")

	fmt.Print("Testing db ...     ")
	if _, err = DB.Exec("SELECT count(*) FROM repositories"); err != nil {
		panic(err)
	}
	fmt.Print("WORKS\n")
}

func DbCheck(table string) {
	var (
		err error
		cnt int64
	)

	allCommits, err := DB.SelectInt(fmt.Sprintf("SELECT count(*) FROM %s", table))
	if err != nil {
		panic(err)
	}
	other, err := DB.SelectInt(fmt.Sprintf("SELECT count(*) FROM %s WHERE type = 'other_commit'", table))
	if err != nil {
		panic(err)
	}
	blaming, err := DB.SelectInt(fmt.Sprintf("SELECT count(*) FROM %s WHERE type = 'blamed_commit'", table))
	if err != nil {
		panic(err)
	}
	fixing, err := DB.SelectInt(fmt.Sprintf("SELECT count(*) FROM %s WHERE type = 'fixing_commit'", table))
	if err != nil {
		panic(err)
	}
	fmt.Printf("Number of commits:\t %8d (o=%d, b=%d, f=%d)\n", allCommits, other, blaming, fixing)

	cnt, err = DB.SelectInt(fmt.Sprintf("SELECT count(*) FROM %s WHERE hunk_count = 0", table))
	if err != nil {
		panic(err)
	}
	fmt.Printf("Hunk count = 0:\t\t %8d (%3.2f %%)\n", cnt, float64(cnt)*float64(100)/float64(allCommits))

	cnt, err = DB.SelectInt(fmt.Sprintf("SELECT count(*) FROM %s WHERE author_email = ''", table))
	if err != nil {
		panic(err)
	}
	fmt.Printf("Author email empty:\t %8d (%3.2f %%)\n", cnt, float64(cnt)*float64(100)/float64(allCommits))

	cnt, err = DB.SelectInt(fmt.Sprintf("SELECT count(*) FROM %s WHERE future_changes = 0", table))
	if err != nil {
		panic(err)
	}
	fmt.Printf("Future changes = 0:\t %8d (%3.2f %%)\n", cnt, float64(cnt)*float64(100)/float64(allCommits))

	cnt, err = DB.SelectInt(fmt.Sprintf("SELECT count(*) FROM (SELECT sha FROM %s GROUP BY sha HAVING count(*) > 1) AS x", table))
	if err != nil {
		panic(err)
	}
	fmt.Printf("Double commits:\t\t %8d (%3.2f %%)\n", cnt, float64(cnt)*float64(100)/float64(allCommits))

	PrintProgressByCommit(table)
	PrintSizeOfStableDb()
	PrintIsHeartbleedInStable(table)
}

func PrintSizeOfStableDb() {
	fmt.Println("\nSize of stable commits")
	rows2, err := DB.Db.Query(fmt.Sprintf(`
		SELECT	'blamed_commit 2014', count(*)
		FROM	unstable.commits b
		WHERE	b.hunk_count <> 0 and b.patch != '' and b.message != ''
				and b.future_changes <> 0
				and b.type = 'blamed_commit'
				and b.id IN (SELECT blamed_commit_id FROM unstable.commits where extract(year from committer_when) >= 2014 and type = 'fixing_commit')
				and b.id in (select commit_id from unstable.functions)
	`))
	if err != nil {
		panic(err)
	}
	defer rows2.Close()
	for rows2.Next() {
		var (
			_type string
			count int
		)
		rows2.Scan(&_type, &count)
		fmt.Printf("%s %*d\n", _type, 30-len(_type), count)
	}

	rows1, err := DB.Db.Query(fmt.Sprintf(`
		SELECT type, count(*) from unstable.commits
		WHERE  hunk_count <> 0 and patch != '' and message != ''
		       and future_changes <> 0
			   and id in (select commit_id from unstable.functions)
		group by type;
	`))
	if err != nil {
		panic(err)
	}
	defer rows1.Close()
	for rows1.Next() {
		var (
			_type string
			count int
		)
		rows1.Scan(&_type, &count)
		fmt.Printf("%s %*d\n", _type, 30-len(_type), count)
	}
}

func PrintProgressByCommit(table string) {
	fmt.Println("\nBy commit:")
	rows, err := DB.Db.Query(fmt.Sprintf(`
		select r.name, r.commits_count, c.cnt as all, done.cnt, (done.cnt::float / c.cnt::float) * 100 as done from
		repositories r,
		(select repository_id, count(*) as cnt from %s group by repository_id) c,
		(select repository_id, count(*) as cnt from %s where hunk_count <> 0 group by repository_id) done
		where (c.repository_id = r.id) and (done.repository_id = r.id) order by done DESC;
	`, table, table))
	if err != nil {
		panic(err)
	}
	defer rows.Close()
	for rows.Next() {
		var (
			name    string
			commits int
			all     int
			done    int
			pct     float64
		)
		rows.Scan(&name, &commits, &all, &done, &pct)
		fmt.Printf("%s %*d of %6d in db, %6d done (%3.2f %%)\n", name, 30-len(name), all, commits, done, pct)
	}
}

func PrintIsHeartbleedInStable(table string) {
	fmt.Println("\nHeartbleed in stable:")

	rows, err := DB.Db.Query(fmt.Sprintf(`
		SELECT	id, sha, type, blamed_commit_id,
				(hunk_count <> 0 and patch != '' and message != ''
				and future_changes <> 0
				and id in (select commit_id from unstable.functions)) as complete
		FROM	%s
		WHERE	sha in ('bd6941cfaa31ee8a3f8661cb98227a5cbcc0f9f3', '96db9023b881d7cd9f379b0c154650d6c108e9a3')`,
		table))

	if err != nil {
		panic(err)
	}
	defer rows.Close()
	for rows.Next() {
		var (
			id               int64
			sha              string
			_type            string
			blamed_commit_id sql.NullInt64
			complete         bool
		)
		rows.Scan(&id, &sha, &_type, &blamed_commit_id, &complete)
		fmt.Println(id, sha, _type, blamed_commit_id, complete)
	}
}
