package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path"
	"runtime"
	"sort"
	"strings"

	"database/sql"

	_ "github.com/lib/pq"

	log "github.com/Sirupsen/logrus"

	"time"

	"github.com/google/go-github/github"
	"github.com/libgit2/git2go"
)

type Repository struct {
	gitRepository    *git.Repository `db:"-" json"-"`
	onRamdisk        bool            `db:"-" json"-"` // whether repo is on ramdisk, false by default
	Commits          []Commit        `db:"-" json"-"`
	Id               int64           `json:"-" db:"id" table:"repositories"`
	Name             string          `json:"full_name" db:"name"`
	Description      string          `json:"description" db:"description"`
	PushedAt         time.Time       `json:"pushed_at" db:"pushed_at"`
	CreatedAt        time.Time       `json:"created_at" db:"created_at"`
	UpdatedAt        time.Time       `json:"updated_at" db:"updated_at"`
	ForksCount       int             `json:"forks_count" db:"forks_count"`
	StargazersCount  int             `json:"stargazers_count" db:"stargazers_count"`
	WatchersCount    int             `json:"watchers_count" db:"watchers_count"`
	SubscribersCount int             `json:"subscribers_count" db:"subscribers_count"`
	OpenIssuesCount  int             `json:"open_issues_count" db:"open_issues_count"`
	Size             int             `json:"size" db:"size"`
	Language         string          `json:"language" db:"language"`
	DefaultBranch    string          `json:"default_branch" db:"default_branch"`
	GitUrl           string          `json:"git_url" db:"git_url"`
}

var (
	RepoBasePath = "repos/"
)

func NewRepositoryFromDB(reponame string) (*Repository, error) {
	r := new(Repository)
	row := DB.Db.QueryRow("SELECT id, name, language, git_url FROM repositories WHERE name = $1", reponame)
	if err := row.Scan(&r.Id, &r.Name, &r.Language, &r.GitUrl); err != nil {
		log.Error("error in new repo from db")
		return nil, err
	}
	return r, nil
}

func (r *Repository) GetId() int64 {
	return r.Id
}

func (r *Repository) Update() (err error) {
	log.Debugf("%v: clone()", r)
	if err = r.clone(); err != nil {
		return
	}

	r.CopyToRamdisk()

	log.Debugf("%v: DB.Update()", r)
	if _, err = DB.Update(r); err != nil {
		return
	}
	log.Debugf("%v: addAllCommits()", r)
	if err = r.addAllCommits(); err != nil {
		return
	}
	log.Debugf("%v: addAuthorContributions()", r)
	if err = r.addAuthorContributions(); err != nil {
		return
	}
	log.Debugf("%v: Update() done", r)

	return nil
}

func (r *Repository) String() string {
	return r.Name
}

func (r *Repository) addAllCommits() (err error) {
	var (
		e              error
		sha            = new(string)
		rows           *sql.Rows
		commitShasInDB []string
	)

	for {
		rows, e = DB.Db.Query("SELECT sha FROM unstable.commits WHERE repository_id = $1", r.Id)
		if e == nil {
			break
		}
		log.Errorf("add all commits: %v", e)
		ReopenDB()
	}
	defer rows.Close()
	for rows.Next() {
		rows.Scan(sha)
		commitShasInDB = append(commitShasInDB, *sha)

	}
	if err := rows.Err(); err != nil {
		log.Errorf("scan done: %v", err)
		ReopenDB()
	}
	sort.Strings(commitShasInDB)
	commitsBeforeInsert := len(commitShasInDB)

	gitRepo, err := r.GitRepository()
	if err != nil {
		return err
	}
	ref, err := gitRepo.Head()
	if err != nil {
		return err
	}
	headCommit, err := gitRepo.LookupCommit(ref.Target())
	if err != nil {
		return err
	}
	commitShasInDB, err = r.addParents(headCommit, commitShasInDB)
	if err != nil {
		log.Warn(err)
	}

	commitShasInDB, err = r.addCveCommits(commitShasInDB)
	if err != nil {
		return err
	}

	log.Debugf("%s: %d new commits inserted", r.String(), len(commitShasInDB)-commitsBeforeInsert)
	return nil
}

func (r *Repository) addParents(co *git.Commit, ignoreCommits []string) ([]string, error) {
	var (
		err error
		p   uint
	)
	for co != nil {
		// first, add current commit
		commitsBeforeInsert := len(ignoreCommits)
		ignoreCommits, _, _ = r.addCommit(co, ignoreCommits)
		if commitsBeforeInsert == len(ignoreCommits) {
			// nothing has changed -> we already added all older commits
			return ignoreCommits, nil
		}
		// if there are multiple parents, first add all other branches
		if co.ParentCount() > 1 {
			for p = 1; p < co.ParentCount(); p++ {
				ignoreCommits, err = r.addParents(co.Parent(p), ignoreCommits)
				if err != nil {
					return ignoreCommits, err
				}
			}
		}
		// then continue with the first parent
		co = co.Parent(0)
	}

	return ignoreCommits, nil
}

func (r *Repository) addCveCommits(ignoreCommits []string) ([]string, error) {
	var shas []string
	for _, sha := range KnownCVEs.ShasForRepo(r.Name) {
		oid, err := git.NewOid(sha)
		if err != nil {
			continue
		}
		co, err := r.gitRepository.LookupCommit(oid)
		if err != nil {
			continue
		}
		ignoreCommits, _, _ = r.addCommitWithType(co, ignoreCommits, "fixing_commit")
		shas = append(shas, "'"+sha+"'")
	}
	// mark all CVE commits as fixing
	if _, err := DB.Db.Exec(`
		UPDATE	unstable.commits SET type = 'other_commit'
		WHERE	type = 'fixing_commit' AND repository_id = $1`,
		r.Id,
	); err != nil {
		log.Warn(err)
	}
	if len(shas) == 0 {
		return ignoreCommits, nil
	}
	q := `
	UPDATE	unstable.commits SET type = 'fixing_commit'
	WHERE	sha IN (` + strings.Join(shas, ", ") + `)  AND repository_id = $1;
	`
	res, err := DB.Db.Exec(q, r.Id)
	if err != nil {
		log.Warn(err, q)
		return ignoreCommits, err
	}
	if cnt, err := res.RowsAffected(); err == nil {
		log.Debugf("Marked %d commits as 'fixing'\n", cnt)
	}
	return ignoreCommits, nil
}

func (r *Repository) addCommit(co *git.Commit, ignoreCommits []string) ([]string, *Commit, error) {
	return r.addCommitWithType(co, ignoreCommits, "other_commit")
}

func (r *Repository) addCommitWithType(co *git.Commit, ignoreCommits []string, _type string) ([]string, *Commit, error) {
	// first search if sha is already in the db
	idx := sort.SearchStrings(ignoreCommits, co.Id().String())
	if idx < len(ignoreCommits) && ignoreCommits[idx] == co.Id().String() {
		return ignoreCommits, nil, nil
	}
	newCommit := &Commit{
		RepositoryId:   int64(r.Id),
		Type:           _type,
		Sha:            co.Id().String(),
		AuthorEmail:    fixInvalidUtf8(co.Author().Email),
		AuthorName:     fixInvalidUtf8(co.Author().Name),
		AuthorWhen:     co.Author().When,
		CommitterEmail: fixInvalidUtf8(co.Committer().Email),
		CommitterName:  fixInvalidUtf8(co.Committer().Name),
		CommitterWhen:  co.Committer().When,
	}
	if e := DB.Insert(newCommit); e != nil {
		log.Warnf("%s %s: inserting failed: %v", r.String(), newCommit.Sha, e)
	}
	log.Debugf("%s: inserted commit %s (%d total)", r.String(), newCommit.Sha, len(ignoreCommits))
	// housekeeping in ignoreCommits
	ignoreCommits = append(ignoreCommits, newCommit.Sha)
	sort.Strings(ignoreCommits)

	return ignoreCommits, newCommit, nil
}

func (r *Repository) CloneUrl() (string, error) {
	switch {
	case r.GitUrl != "":
		return r.GitUrl, nil
	case r.Name != "":
		return fmt.Sprintf("git://github.com/%s.git", r.Name), nil
	default:
		return "", fmt.Errorf("Repo %s does not have a git url", r.String())
	}
}

func (r *Repository) clone() error {
	dir := path.Join(RepoBasePath, r.Name)
	url, err := r.CloneUrl()
	if err != nil {
		return err
	}
	log.Debugf("git clone or pull %s from %s", r.Name, url)

	errBuf := new(bytes.Buffer)
	gitRepo, err := git.OpenRepository(dir)
	if err != nil { // repository doesn't exist -> clone
		start := time.Now()
		log.Debugf("git clone %s %s", url, dir)
		cloneCmd := exec.Command("git", "clone", url, dir)
		cloneCmd.Stderr = errBuf
		cloneCmd.Dir = "."
		if err := cloneCmd.Run(); err != nil {
			fmt.Errorf("git clone %s failed (%v):\n%s", r.Name, err, errBuf)
		}
		log.Debugf("git clone %s took %s", r.Name, time.Since(start))
		r.gitRepository, err = git.OpenRepository(dir)
		if err != nil {
			fmt.Errorf("after git clone %s failed (%v):\n%s", r.Name, err, errBuf)
		}
	} else {
		start := time.Now()
		r.gitRepository = gitRepo
		pullCmd := exec.Command("git", "pull")
		pullCmd.Dir = gitRepo.Workdir()
		pullCmd.Stderr = errBuf
		if err := pullCmd.Run(); err != nil {
			fmt.Errorf("git clone %s failed (%v):\n%s", r.Name, err, errBuf)
		}
		log.Debugf("git pull %s took %s", r.Name, time.Since(start))
	}
	return nil
}

func (r *Repository) IsGithubRepo() bool {
	return r.GitUrl == ""
}

func (r *Repository) Basename() string {
	return strings.Split(r.Name, "/")[1]
}

func (r *Repository) Owner() string {
	return strings.Split(r.Name, "/")[0]
}

func (r *Repository) GitRepository() (repo *git.Repository, err error) {
	if r.gitRepository != nil {
		return r.gitRepository, nil
	}
	repo, err = git.OpenRepository(path.Join(RepoBasePath, r.Name))
	if err != nil {
		return nil, err
	}
	r.gitRepository = repo
	return
}

func (r *Repository) Dir() string {
	return path.Join(RepoBasePath, r.Name)
}

func (r *Repository) CopyToRamdisk() (err error) {
	errBuf := new(bytes.Buffer)
	shm := "/run/shm"
	// check if /run/shm exists
	if _, err = os.Stat(shm); err != nil {
		return fmt.Errorf("%s not found: %v", shm, err)
	}
	src := path.Join(RepoBasePath, r.Name)
	dest := path.Join(shm, r.Owner())
	exec.Command("rm", "-rf", "dest").Run()
	exec.Command("mkdir", "-p", path.Join(shm, r.Name)).Run()
	cp := exec.Command("cp", "-a", src, dest)
	cp.Stderr = errBuf
	if err = cp.Run(); err != nil {
		return fmt.Errorf("Error copying from %s to %s: %v\n%s", src, dest, err, errBuf)
	}
	log.Infof("%v: copied %s to %s", r, src, dest)
	// we copied successfully, set finalizer here (will call removeFromRamdisk(r) if r get GC'd
	r.onRamdisk = true
	runtime.SetFinalizer(r, RemoveFromRamdisk)
	// free previous git repository
	if r.gitRepository != nil {
		r.gitRepository.Free()
	}
	// try to open the repository, since it is in a non-standard place
	// if all went wrong this will be nil and the standard place will be there
	r.gitRepository, _ = git.OpenRepository(path.Join(shm, r.Name))
	return nil
}

// no need to run this manually, uses SetFinalizer
func RemoveFromRamdisk(r *Repository) {
	if !r.onRamdisk {
		return
	}
	// free previous git repository
	if r.gitRepository != nil {
		r.gitRepository.Free()
		r.gitRepository = nil
	}
	dest := path.Join("/run/shm/", r.Owner())
	rm := exec.Command("rm", "-rf", dest)
	rm.Run()
}

func (r *Repository) addAuthorContributions() (err error) {
	var (
		e             error
		rows          *sql.Rows
		emptyCommits  int64 = 1
		allCommits    int64 = 0
		contribByName       = make(map[string][]int64)
	)

	cntRow := DB.Db.QueryRow("SELECT count(*) FROM unstable.commits WHERE repository_id = $1 and (author_contributions_percent IS NULL or author_contributions_percent = 0)", r.Id)
	err = cntRow.Scan(&emptyCommits)
	if err != nil {
		log.Errorf("Getting count of commits with empty author contrib: %v", err)
		ReopenDB()
	} else if emptyCommits == 0 {
		log.Infof("%v: skipping addAuthorContributions, db up to date", r)
		return nil
	}
	log.Debugf("%s: %d commits with empty author contrib", r, emptyCommits)

	for {
		rows, e = DB.Db.Query("SELECT id, author_email FROM unstable.commits WHERE repository_id = $1", r.Id)
		if err == nil {
			break
		}
		log.Errorf("addAuthorContributions(): %v", e)
		ReopenDB()
	}
	defer rows.Close()
	for rows.Next() {
		var (
			id    int64
			email string
		)
		allCommits++
		if err := rows.Scan(&id, &email); err != nil {
			return fmt.Errorf("row scan: %v", err)
		}
		contribByName[email] = append(contribByName[email], id)
	}
	if err := rows.Err(); err != nil {
		log.Errorf("scan done: %v", err)
		ReopenDB()
	}

	for _, ids := range contribByName {
		contrib := float64(len(ids)) / float64(allCommits)
		for _, id := range ids {
			commit := &Commit{Id: id}
			err = PersistColumn(commit, "author_contributions_percent", contrib)
		}
	}

	return
}

func AddRepository(reponame string) error {
	r := new(Repository)
	fmt.Printf("adding %s\n", reponame)
	ghClient := github.NewClient(nil)
	req, err := ghClient.NewRequest("GET", "repos/"+reponame, nil)
	if err != nil {
		return fmt.Errorf("connecting to github: %v", err)
	}
	res, err := ghClient.Do(req, r)
	if err != nil {
		return fmt.Errorf("retrieving repo from github: %v %v %v", req, res, err)
	}
	if e := DB.Insert(r); e != nil {
		log.Warnf("%v: inserting failed: %v", r, e)
	}

	return nil
}
