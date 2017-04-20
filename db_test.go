package main

import "testing"

func TestPersistColumnSql(t *testing.T) {
	var (
		q        string
		expected string
	)

	q = PersistColumnSql(&Commit{Id: 42}, "sha", "deadbeef")
	expected = "UPDATE unstable.commits SET sha = $1 WHERE id = 42"
	if q != expected {
		t.Errorf("%s should be %s", q, expected)
	}

	q = PersistColumnSql(&Repository{Id: 42}, "name", "foo/bar")
	expected = "UPDATE repositories SET name = $1 WHERE id = 42"
	if q != expected {
		t.Errorf("%s should be %s", q, expected)
	}

	q = PersistColumnSql(&Function{Id: 42}, "name", "foo.c")
	expected = "UPDATE functions SET name = $1 WHERE id = 42"
	if q != expected {
		t.Errorf("%s should be %s", q, expected)
	}
}

func TestPersistColumnsSql(t *testing.T) {
	var (
		q        string
		expected string
		err      error
	)

	q, _, err = PersistColumnsSql(&Commit{Id: 42, Sha: "deadbeef"}, "Sha")
	if err != nil {
		t.Error(err)
	}
	expected = "UPDATE unstable.commits SET sha = $1 WHERE id = 42"
	if q != expected {
		t.Errorf("%s should be %s", q, expected)
	}

	q, _, err = PersistColumnsSql(&Repository{Id: 42, Name: "deadbeef"}, "Name")
	if err != nil {
		t.Error(err)
	}
	expected = "UPDATE repositories SET name = $1 WHERE id = 42"
	if q != expected {
		t.Errorf("%s should be %s", q, expected)
	}
}
