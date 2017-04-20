package main

import (
	"database/sql"
	"flag"
	"os"
	"strings"
	"sync"

	"runtime"
	"runtime/pprof"

	log "github.com/Sirupsen/logrus"

	_ "github.com/lib/pq"
)

var (
	commitProcs int
	//repoProcs      int
	logLevel          string
	logPath           string
	createTables      bool
	initRedis         bool
	reportProgress    bool
	doSelfTest        bool
	profilePath       string
	processname       string
	onlyOneRepo       string
	skipRedis         bool
	doStableDbCheck   bool
	doUnstableDbCheck bool
	onlyOneCommit     string
	addRepository     string
	commitsSelect     string
	KnownCVEs         *MitreCves
)

func init() {
	//flag.IntVar(&repoProcs, "repo-threads", 3, "number of repo threads")
	flag.IntVar(&commitProcs, "commit-threads", 50, "number of commit threads")
	flag.StringVar(&logLevel, "log-level", "warn", "Logging level")
	flag.StringVar(&RepoBasePath, "repo-path", "repos/", "path to repositories")
	flag.StringVar(&logPath, "log", "", "file to log to")
	flag.BoolVar(&createTables, "create-tables", false, "Whether to create the database tables")
	flag.BoolVar(&initRedis, "init-redis", false, "Just init redis")
	flag.BoolVar(&reportProgress, "progress", false, "Just report progress")
	flag.BoolVar(&doSelfTest, "self-test", false, "Just do a self test")
	flag.BoolVar(&doStableDbCheck, "check-stable-db", false, "Just do a consistency check for the stable db")
	flag.BoolVar(&doUnstableDbCheck, "check-unstable-db", false, "Just do a consistency check for the unstable db")
	flag.StringVar(&profilePath, "cpuprofile", "", "Path to write CPU profile to")
	flag.StringVar(&processname, "name", "unnamed", "Name of this process")
	flag.StringVar(&onlyOneRepo, "repo", "", "Update only one repository")
	flag.StringVar(&onlyOneCommit, "commit", "", "Update only one commit")
	flag.BoolVar(&skipRedis, "skip-redis", false, "Don't use redis")
	flag.StringVar(&addRepository, "add-repo", "", "Repo to add to the db")
	flag.StringVar(&commitsSelect, "commits-select", "empty", "Set of commits to select")

	runtime.GOMAXPROCS(runtime.NumCPU())
}

func main() {
	InitDb()
	InitRedis()

	flag.Parse()

	if logPath != "" {
		logFile, err := os.OpenFile(logPath, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
		if err != nil {
			panic(err)
		}
		log.SetOutput(logFile)
	}
	switch logLevel {
	case "debug":
		log.SetLevel(log.DebugLevel)
	case "info":
		log.SetLevel(log.InfoLevel)
	case "warn":
		log.SetLevel(log.WarnLevel)
	case "error":
		log.SetLevel(log.ErrorLevel)
	default:
		log.SetLevel(log.WarnLevel)
		log.Warnf("Log level %s is not in [debug info warn error], setting to warn", logLevel)
	}

	if profilePath != "" {
		f, err := os.Create(profilePath)
		if err != nil {
			log.Errorf("Can't write profile to %s: %v", profilePath, err)
		} else {
			pprof.StartCPUProfile(f)
			defer pprof.StopCPUProfile()
		}
	}
	if createTables {
		DB.CreateTablesIfNotExists()
	}

	if doSelfTest {
		SelfTest()
		return
	}
	if doStableDbCheck {
		DbCheck("public.commits")
		return
	}
	if doUnstableDbCheck {
		DbCheck("unstable.commits")
		return
	}
	if reportProgress {
		PrintProgress()
		return
	}
	if initRedis {
		WriteReposToRedis()
		return
	}
	if addRepository != "" {
		if err := AddRepository(addRepository); err != nil {
			log.Error(err)
		}
		return
	}

	log.Debugln("gathering known CVEs")
	KnownCVEs = NewMitreCves()
	if err := KnownCVEs.Read("data/cve.xml"); err != nil {
		log.Fatal(err)
	}

	if onlyOneRepo != "" {
		handleRepo(onlyOneRepo)
		return
	}

	// main loop
	for {
		switch reponame, err := GetNextRepo(); err {
		case nil:
			handleRepo(reponame)
		case ErrNil:
			log.Info("No repositories left")
			return
		default:
			log.Errorf("Error getting repo: %v", err)
		}
	}
}

func handleRepo(reponame string) {
	log.Infof("Starting %s", reponame)
	if !skipRedis {
		MarkAsWorking(reponame, processname)
	}

	var (
		err        error
		commitRows *sql.Rows
		wg         sync.WaitGroup
	)

	commitSem := make(chan int, commitProcs)
	commitPool := &sync.Pool{New: func() interface{} { return new(Commit) }}

	log.Debugf("repository %s: querying db", reponame)
	r, err := NewRepositoryFromDB(reponame)
	if err != nil {
		log.Errorf("retrieving %s: %v", reponame, err)
		if skipRedis {
			return
		}
		ReturnRepo(reponame)
	}

	// update repository
	log.Debugf("repository %s: updating", r.Name)
	if err := r.Update(); err != nil {
		log.Errorf("Updating %s: %v", r.String(), err)
	}
	defer RemoveFromRamdisk(r)
	log.Debugf("%s: saved", r.Name)

	if onlyOneCommit != "" {
		commitRows, err = DB.Db.Query("SELECT id, sha, type, coalesce(length(patch), 0), coalesce(length(message)) FROM unstable.commits WHERE sha = $2 and repository_id = $1", r.Id, onlyOneCommit)
	} else {
		q := "SELECT id, sha, type, coalesce(length(patch), 0), coalesce(length(message)) FROM unstable.commits WHERE repository_id = $1"
		switch commitsSelect {
		case "all":
			q += ""
		case "blamed":
			q += " and (type = 'blamed_commit')"
		case "cves": // all commits that mention 'CVE'
			q += " and (message like '%CVE-____%' and type = 'other_commit')"
		case "stable": // all commits that would be in stable
			q += " and (hunk_count <> 0 and patch != '' and message != '' and future_changes <> 0 and id in (select commit_id from unstable.functions))"
		case "empty":
			q += " and (future_changes = 0 or additions = 0 or hunk_count = 0 or patch = '' or message = '' or (type = 'fixing_commit' and blamed_commit_id is null))"
		case "fixing":
			shas := KnownCVEs.Shas()
			for i, s := range shas {
				shas[i] = "'" + s + "'"
			}
			q += " and hunk_count = 0 and ( type = 'fixing_commit' or type = 'blamed_commit' or sha IN (" + strings.Join(shas, ", ") + ") )"
		default:
			log.Fatalf("invalid flag:", commitsSelect)
		}
		commitRows, err = DB.Db.Query(q, r.Id)
	}
	if err != nil {
		log.Errorf("retrieving commits for %s: %v", reponame, err)
		if skipRedis {
			return
		}
		ReturnRepo(reponame)
	}
	defer commitRows.Close()
	for commitRows.Next() {
		// update commits
		commitSem <- 1
		wg.Add(1)
		commit := commitPool.Get().(*Commit)
		commit.Clear()
		commitRows.Scan(&commit.Id, &commit.Sha, &commit.Type, &commit.PatchLengthFromDB, &commit.MessageLengthFromDB)

		go func(commit *Commit, r *Repository) {
			defer wg.Done()
			defer commitPool.Put(commit)
			defer func() { <-commitSem }()
			defer func() {
				if e := recover(); e != nil {
					log.Errorf("Go routine crashed for %+v: %v", commit.Repository, e)
				}
			}()
			commit.Repository = r
			if err := commit.Update(); err != nil {
				log.Warnf("updating %v: %v", commit, err)
			}
		}(commit, r)
	}
	if err := commitRows.Err(); err != nil {
		log.Errorf("scan done: %v", err)
		ReopenDB()
	}
	wg.Wait()

	if !skipRedis {
		MarkAsDone(reponame)
	}
	log.Infof("Finished %s", reponame)
}
