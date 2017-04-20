package main

import (
	"database/sql"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	log "github.com/Sirupsen/logrus"
	"github.com/google/go-github/github"
	"github.com/lib/pq/hstore"
	"github.com/libgit2/git2go"

	"time"

	"tools.net.cs.uni-bonn.de/social-aspects-of-vulnerabilities/github-data/ds"
	"tools.net.cs.uni-bonn.de/social-aspects-of-vulnerabilities/github-data/tools"
)

var (
	StandardColumns = []string{"Type", "CVE", "BlamedCommitId", "PastChanges", "FutureChanges", "PastDifferentAuthors", "FutureDifferentAuthors", "HunkCount", "Additions", "Deletions"}
	PatchColumns    = []string{"Patch", "PatchKeywords"}
	MessageColumns  = []string{"Message"}
)

// Commit type represents commits from git with additional meta information
type Commit struct {
	githubCommit   *github.Commit `db:"-"`
	gitCommit      *git.Commit    `db:"-"`
	Repository     *Repository    `db:"-"`
	Id             int64          `json:"-" db:"id" table:"unstable.commits"`
	RepositoryId   int64          `db:"repository_id"`
	BlamedCommitId sql.NullInt64  `db:"blamed_commit_id"`
	Type           string         `db:"type"`
	Sha            string         `db:"sha"`
	Url            sql.NullString `db:"url"` // TODO is this still being used?
	AuthorEmail    string         `db:"author_email"`
	AuthorName     string         `db:"author_name"`
	AuthorWhen     time.Time      `db:"author_when"`
	CommitterEmail string         `db:"committer_email"`
	CommitterName  string         `db:"committer_name"`
	CommitterWhen  time.Time      `db:"committer_when"`
	Additions      int64          `db:"additions"`
	Deletions      int64          `db:"deletions"`
	//RelativeCodeChurn          sql.NullFloat64 `db:"relative_code_churn"`
	PastChanges                int64          `db:"past_changes"`
	FutureChanges              int64          `db:"future_changes"`
	PastDifferentAuthors       int64          `db:"past_different_authors"`
	FutureDifferentAuthors     int64          `db:"future_different_authors"`
	AuthorContributionsPercent float64        `db:"author_contributions_percent"`
	Message                    string         `db:"message"`
	Patch                      string         `db:"patch"`
	HunkCount                  int64          `db:"hunk_count"`
	FilesChanged               int64          `db:"files_changed"`
	CVE                        string         `db:"cve"`
	MessageLengthFromDB        int            `db:"-"` // used to determine if we need to update this field
	PatchLengthFromDB          int            `db:"-"` // used to determine if we need to update this field
	Functions                  []*Function    `db:"-"` // Function information
	ToolResults                []tools.Result `db:"-"` // Tool Results information
	PatchKeywords              hstore.Hstore  `db:"patch_keywords"`
}

var (
	userCache = make(map[string]sql.NullInt64)
	// CvePattern regexp matches CVE ids in unstructured data
	CvePattern = regexp.MustCompile(`CVE-\d{4}-\d{4,7}`)
)

func (c *Commit) GetId() int64 {
	return c.Id
}

// Update gathers and updates meta information for each commit
func (c *Commit) Update() (err error) {

	log.Debugf("%v get git metadata", c)
	if err = c.GetGitMetadata(); err != nil {
		return
	}
	// skip large commits
	if c.Additions+c.Deletions > 2000 {
		log.Infof("%v: ignoring commit with %d changes\n", c, c.Additions+c.Deletions)
		return
	}

	log.Debugf("%v fixCommit", c)
	err = c.fixCommit()

	log.Debugf("%v blameCommit", c)
	if err = c.blameCommit(); err != nil {
		return
	}

	log.Debugf("%v DB.Update", c)
	// Only update columns that are different from db version
	cols := StandardColumns
	if c.MessageLengthFromDB == 0 {
		cols = append(cols, MessageColumns...)
		log.Debugf("adding message")
	}
	if c.PatchLengthFromDB == 0 {
		c.SetPatchKeywords()
		cols = append(cols, PatchColumns...)
		log.Debugf("adding patch")
	}
	if err = PersistColumns(c, cols...); err != nil {
		return
	}
	err = PersistFunctions(c)
	err = PersistToolResults(c)

	log.Debugf("%v Done", c)
	return
}

func (c *Commit) Clear() {
	c.gitCommit = nil
	c.githubCommit = nil
	c.Repository = nil
	c.Functions = nil
	c.ToolResults = nil
	c.Patch = ""
	c.Message = ""
	c.BlamedCommitId.Valid = false
	c.Type = "other_commit"
	c.CVE = ""
}

func (c *Commit) fixCommit() (err error) {
	// first look into list of known CVEs
	if cve, found := KnownCVEs.LookupCommit(c); found {
		log.Debugf("%v contains %v", c, cve)
		c.Type = "fixing_commit"
		c.CVE = cve
		return
	}
	// next check if the commit message mentions CVE-____-____
	if c.Message == "" {
		return fmt.Errorf("fixCommit: message of %v is empty", c)
	}
	cves := CvePattern.FindAllString(c.Message, -1)
	cveStr := strings.Join(cves, ", ")

	if cves != nil {
		log.Debugf("%v contains %v", c, cveStr)
		if len(cves) > 1 {
			log.Debugf("%s contains more than one CVE: %s", c.Sha, cveStr)
		}
		c.Type = "fixing_commit"
		c.CVE = cveStr
	}

	return
}

func (c *Commit) blameCommit() (err error) {
	// only blame for fixing commits
	if c.Type != "fixing_commit" {
		return
	}

	blamedSha, err := c.getBlameCommitSha()
	if err != nil {
		return
	}
	for {
		row := DB.Db.QueryRow(
			"UPDATE unstable.commits SET type = 'blamed_commit' WHERE sha = $1 RETURNING id",
			blamedSha,
		)
		if err = row.Scan(&c.BlamedCommitId); err != nil {
			log.Warnf("%v: Updating blamed commit %s: %v", c, blamedSha, err)

			oid, err := git.NewOid(blamedSha)
			if err != nil {
				return err
			}
			blamed, err := c.Repository.gitRepository.LookupCommit(oid)
			if err != nil {
				return err
			}
			_, blamedCommit, err := c.Repository.addCommit(blamed, nil)
			if err != nil {
				return err
			}
			blamedCommit.gitCommit = blamed
			blamedCommit.Repository = c.Repository
			err = blamedCommit.Update()
			if err != nil {
				return err
			}
			// then retry
		} else {
			break
		}
	}

	return
}

func (c *Commit) getBlameCommitSha() (blamedCommit string, err error) {
	blamedCommits := ds.NewMaxMap()
	repo, err := c.Repository.GitRepository()
	if err != nil {
		return
	}

	diff, parent, err := c.diff()
	if err != nil {
		return
	}
	defer diff.Free()

	err = diff.ForEach(func(delta git.DiffDelta, num float64) (git.DiffForEachHunkCallback, error) {
		var blame *Blame

		log.Debugf("%v: %s is code -> %v", c, delta.OldFile.Path, IsCodeFile(delta.OldFile.Path))
		if delta.Status != git.DeltaAdded && IsCodeFile(delta.OldFile.Path) {
			blame, err = NewBlame(repo, parent.Id().String(), delta.OldFile.Path, BlameBackward)
		}

		additionBlock := false
		return func(hunk git.DiffHunk) (git.DiffForEachLineCallback, error) {
			return func(line git.DiffLine) error {
				// only consider deleted lines
				if blame == nil {
					return nil
				}
				var lineToBlame int
				switch {
				case line.Origin == git.DiffLineAddition:
					if !additionBlock {
						// first added line in addition block -> blame previous line
						additionBlock = true
						lineToBlame = line.NewLineno - 1
					}
				case line.Origin == git.DiffLineDeletion || additionBlock:
					// Blame on deleted lines OR if addition block ended
					additionBlock = false
					lineToBlame = line.OldLineno
				}
				if lineToBlame > 0 {
					bl, err := blame.ForLine(lineToBlame)
					if err != nil {
						log.Errorf("%v: could not get blame for line %d", c, lineToBlame)
						return nil
					}
					log.Debugf("%v: blame line %d -> %s", c, lineToBlame, bl.Sha)
					blamedCommits.Add(bl.Sha)
				}

				return nil
			}, nil
		}, err
	}, git.DiffDetailLines)

	blamedCommit, _ = blamedCommits.MaxString()
	if blamedCommit == "" {
		err = fmt.Errorf("no blamed commit found (%+v)", blamedCommits)
	} else {
		log.Infof("%s: blame %s", c.String(), blamedCommit)
	}

	return
}

func (c *Commit) String() string {
	if c.Url.Valid {
		return c.Url.String
	}
	return fmt.Sprintf("%s %s", c.Repository.Name, c.Sha)
}

func (c *Commit) diff() (diff *git.Diff, parent *git.Commit, err error) {
	var pTree *git.Tree
	gitCommit, err := c.GitCommit()
	if err != nil {
		return
	}
	repo, err := c.Repository.GitRepository()
	if err != nil {
		return
	}
	diffOpts, _ := git.DefaultDiffOptions()
	diffOpts.Flags = git.DiffIgnoreFilemode
	parent = gitCommit.Parent(0)
	if parent == nil {
		//return nil, nil, fmt.Errorf("Initial commit")
		pTree = new(git.Tree) // use empty tree
	} else {
		pTree, err = parent.Tree()
		if err != nil {
			return
		}
	}
	defer pTree.Free()
	cTree, err := gitCommit.Tree()
	defer cTree.Free()
	if err != nil {
		return
	}
	diff, err = repo.DiffTreeToTree(pTree, cTree, &diffOpts)

	return
}

// GitCommit returns the git commit for the commit (requires that the repository has been cloned)
func (c *Commit) GitCommit() (commit *git.Commit, err error) {
	if c.gitCommit != nil {
		return c.gitCommit, nil
	}

	oid, err := git.NewOid(c.Sha)
	if err != nil {
		return
	}
	repo, err := c.Repository.GitRepository()
	if err != nil {
		return
	}
	commit, err = repo.LookupCommit(oid)
	if err != nil {
		return
	}
	c.gitCommit = commit

	return
}

func (c *Commit) GetGitMetadata() (err error) {
	diff, _, err := c.diff()
	if err != nil {
		return fmt.Errorf("creating diff: %v", err)
	}
	defer diff.Free()

	var (
		totalLinesInFiles = 0
	)
	totalChanges := ChangeStatistic{}
	repo, err := c.Repository.GitRepository()
	if err != nil {
		return fmt.Errorf("getting repo: %v", err)
	}
	gitCommit, err := c.GitCommit()
	if err != nil {
		return fmt.Errorf("getting commit: %v", err)
	}

	c.AuthorEmail = fixInvalidUtf8(gitCommit.Author().Email)
	c.AuthorName = fixInvalidUtf8(gitCommit.Author().Name)
	c.AuthorWhen = gitCommit.Author().When
	c.CommitterEmail = fixInvalidUtf8(gitCommit.Committer().Email)
	c.CommitterName = fixInvalidUtf8(gitCommit.Committer().Name)
	c.CommitterWhen = gitCommit.Committer().When
	c.Message = fixInvalidUtf8(gitCommit.Message())

	// reset statistics
	c.HunkCount = 0

	// don't re-add all the data if the db already knows
	if c.PatchLengthFromDB > 0 {
		log.Debugf("%s: patch in db already has length %d", c, c.PatchLengthFromDB)
	}

	emptyEachHunkCB := func(hunk git.DiffHunk) (git.DiffForEachLineCallback, error) {
		return func(line git.DiffLine) error {
			return nil
		}, nil
	}

	diff.ForEach(func(delta git.DiffDelta, num float64) (git.DiffForEachHunkCallback, error) {
		var (
			cs                           *ChangeStatistic
			path                         string
			analyzeFunctionsInLineLoop   = false
			analyzeToolResultsInLineLoop = false
			isCodeFile                   = false
			newFunctions                 *Functions
			oldFunctions                 *Functions
			toolResults                  []tools.Result
		)

		// change statistics code
		switch delta.Status {
		case git.DeltaDeleted:
			path = delta.OldFile.Path
			log.Debugf("%v: Deleted Delta %s\n", c, path)
		case git.DeltaAdded, git.DeltaModified:
			path = delta.NewFile.Path
			blob, err := repo.LookupBlob(delta.NewFile.Oid)
			if err != nil {
				log.Warnf("%v: LookupBlob(%s) %v", c, delta.NewFile.Oid, err)
				break
			}
			lines, err := LinesInBlob(blob)
			if err != nil {
				log.Warnf("%v: %s: %v", c, path, err)
			}
			totalLinesInFiles += lines
			log.Debugf("%v: Added/Modified Delta %s\n", c, path)
		case git.DeltaRenamed:
			log.Debugf("%v: RENAMED %s\n", c, path)
			fallthrough
		default:
			log.Debugf("%v: Skip Delta %+v with status %d\n", c, delta, int(delta.Status))
			return emptyEachHunkCB, nil
		}
		cs, err := FileChanges(repo, gitCommit, path)
		if err != nil {
			log.Warnf("%v FileChanges(): %v\n", c, err)
		}
		totalChanges.Add(cs)

		isCodeFile = IsCodeFile(path)
		if !isCodeFile {
			log.Debugf("%v: ignoring %s since not code", c, path)
		}

		// function information on file/delta level:
		// only handle completely new or deleted files here
		// Only run function information on code
		if isCodeFile {
			switch delta.Status {
			case git.DeltaAdded:
				functions, err := FunctionsForFile(repo, &delta.NewFile)
				if err != nil {
					log.Warnf("%v FunctionsForFile(%v) (new): %v", c, &delta.NewFile, err)
				} else {
					for _, f := range functions.Functions() {
						f.CommitId = c.Id
						f.State = "added"
						c.Functions = append(c.Functions, f)
					}
				}
				flawfinderResults, err := tools.Flawfinder.Analyze(repo, &delta.NewFile)
				if err != nil {
					log.Warnf("%v FlawfinderResults(%v) (new): %v", c, &delta.NewFile, err)
				} else {
					c.ToolResults = append(c.ToolResults, flawfinderResults...)
				}
				ratsResults, err := tools.Rats.Analyze(repo, &delta.NewFile)
				if err != nil {
					log.Warnf("%v RatsResults(%v) (new): %v", c, &delta.NewFile, err)
				} else {
					c.ToolResults = append(c.ToolResults, ratsResults...)
				}
			case git.DeltaDeleted:
				functions, err := FunctionsForFile(repo, &delta.OldFile)
				if err != nil {
					log.Warnf("%v FunctionsForFile(%s) (old): %v", c, &delta.OldFile, err)
					break
				}
				for _, f := range functions.Functions() {
					f.CommitId = c.Id
					f.State = "deleted"
					c.Functions = append(c.Functions, f)
				}
			case git.DeltaModified:
				flawfinderResults, err := tools.Flawfinder.Analyze(repo, &delta.NewFile)
				if err != nil {
					log.Warnf("%v FlawfinderResults(%v) (new): %v", c, &delta.NewFile, err)
				} else {
					analyzeToolResultsInLineLoop = true
				}
				ratsResults, err := tools.Flawfinder.Analyze(repo, &delta.NewFile)
				if err != nil {
					log.Warnf("%v RatsResults(%v) (new): %v", c, &delta.NewFile, err)
				} else {
					analyzeToolResultsInLineLoop = true
				}
				toolResults = tools.Merge(flawfinderResults, ratsResults)

				// need to handle this on hunk level
				newFunctions, err = FunctionsForFile(repo, &delta.NewFile)
				if err != nil {
					log.Warnf("%v FunctionsForFile(%s) (mod new): %v", c, &delta.NewFile, err)
					break
				}
				oldFunctions, err = FunctionsForFile(repo, &delta.OldFile)
				//log.Infof("old file %s %s", delta.OldFile.Path, delta.OldFile.Oid.String())
				if err != nil {
					log.Warnf("%v FunctionsForFile(%s) (mod old): %v", c, &delta.OldFile, err)
					break
				}
				// analyze functions in new file here
				analyzeFunctionsInLineLoop = true
				c.Functions = append(c.Functions,
					AddedAndDeletedFunctions(newFunctions, oldFunctions)...)
			}
		}

		// each hunk
		return func(hunk git.DiffHunk) (git.DiffForEachLineCallback, error) {
			c.HunkCount++

			funIter := newFunctions.NewEmptyFunctionIterator()
			// each line
			return func(line git.DiffLine) error {
				//log.Infof("\n%+v", line)
				//log.Infof("analyze: %v", analyzeFunctionsInLineLoop)
				if isCodeFile && analyzeFunctionsInLineLoop {
					var lineno int = -1
					if line.Origin == git.DiffLineAddition {
						lineno = line.NewLineno
					}
					if line.Origin == git.DiffLineDeletion {
						lineno = line.OldLineno
					}
					if lineno != -1 {
						//log.Infof("line %d", lineno)
						f, analyzeFunctionsInLineLoop := funIter(lineno)
						if analyzeFunctionsInLineLoop && f.ContainsLine(lineno) {
							f.State = "modified"
							c.Functions = append(c.Functions, f)
							log.Infof("function %+v", f)
						}
					}
				}
				if isCodeFile && analyzeToolResultsInLineLoop && line.Origin == git.DiffLineAddition {
					r := tools.ResultsAtLine(toolResults, uint(line.NewLineno))
					c.ToolResults = append(c.ToolResults, r...)
				}

				return nil
			}, nil
		}, nil
	}, git.DiffDetailLines)

	if c.PatchLengthFromDB == 0 {
		numDeltas, _ := diff.NumDeltas()
		for i := 0; i < numDeltas; i++ {
			p, err := diff.Patch(i)
			if err != nil {
				log.Error(err)
			}
			defer p.Free()
			s, err := p.String()
			if err != nil {
				log.Error(err)
			}
			c.Patch += s
		}
		c.Patch = fixInvalidUtf8(c.Patch)
	}

	c.FutureChanges = totalChanges.FutureChanges
	c.PastChanges = totalChanges.PastChanges
	c.FutureDifferentAuthors = totalChanges.FutureAuthors
	c.PastDifferentAuthors = totalChanges.PastAuthors

	stats, err := diff.Stats()
	if err != nil {
		return
	}
	c.Additions = int64(stats.Insertions())
	c.Deletions = int64(stats.Deletions())
	c.FilesChanged = int64(stats.FilesChanged())
	//if totalLinesInFiles == 0 {
	//c.RelativeCodeChurn = 0
	//} else {
	//c.RelativeCodeChurn = float64(c.Additions+c.Deletions) / float64(totalLinesInFiles)
	//}

	log.Debugf("%v total changes: %+v", c, totalChanges)

	return
}

func (c *Commit) SetPatchKeywords() {
	if c.Patch == "" {
		return
	}
	keywords := map[string]sql.NullString{
		"auto":             sql.NullString{String: "0", Valid: true},
		"break":            sql.NullString{String: "0", Valid: true},
		"case":             sql.NullString{String: "0", Valid: true},
		"char":             sql.NullString{String: "0", Valid: true},
		"const":            sql.NullString{String: "0", Valid: true},
		"continue":         sql.NullString{String: "0", Valid: true},
		"default":          sql.NullString{String: "0", Valid: true},
		"do":               sql.NullString{String: "0", Valid: true},
		"double":           sql.NullString{String: "0", Valid: true},
		"else":             sql.NullString{String: "0", Valid: true},
		"enum":             sql.NullString{String: "0", Valid: true},
		"extern":           sql.NullString{String: "0", Valid: true},
		"float":            sql.NullString{String: "0", Valid: true},
		"for":              sql.NullString{String: "0", Valid: true},
		"goto":             sql.NullString{String: "0", Valid: true},
		"if":               sql.NullString{String: "0", Valid: true},
		"int":              sql.NullString{String: "0", Valid: true},
		"long":             sql.NullString{String: "0", Valid: true},
		"register":         sql.NullString{String: "0", Valid: true},
		"return":           sql.NullString{String: "0", Valid: true},
		"short":            sql.NullString{String: "0", Valid: true},
		"signed":           sql.NullString{String: "0", Valid: true},
		"sizeof":           sql.NullString{String: "0", Valid: true},
		"static":           sql.NullString{String: "0", Valid: true},
		"struct":           sql.NullString{String: "0", Valid: true},
		"switch":           sql.NullString{String: "0", Valid: true},
		"typedef":          sql.NullString{String: "0", Valid: true},
		"union":            sql.NullString{String: "0", Valid: true},
		"unsigned":         sql.NullString{String: "0", Valid: true},
		"void":             sql.NullString{String: "0", Valid: true},
		"volatile":         sql.NullString{String: "0", Valid: true},
		"while":            sql.NullString{String: "0", Valid: true},
		"asm":              sql.NullString{String: "0", Valid: true},
		"dynamic_cast":     sql.NullString{String: "0", Valid: true},
		"namespace":        sql.NullString{String: "0", Valid: true},
		"reinterpret_cast": sql.NullString{String: "0", Valid: true},
		"try":              sql.NullString{String: "0", Valid: true},
		"bool":             sql.NullString{String: "0", Valid: true},
		"explicit":         sql.NullString{String: "0", Valid: true},
		"new":              sql.NullString{String: "0", Valid: true},
		"static_cast":      sql.NullString{String: "0", Valid: true},
		"typeid":           sql.NullString{String: "0", Valid: true},
		"catch":            sql.NullString{String: "0", Valid: true},
		"false":            sql.NullString{String: "0", Valid: true},
		"operator":         sql.NullString{String: "0", Valid: true},
		"template":         sql.NullString{String: "0", Valid: true},
		"typename":         sql.NullString{String: "0", Valid: true},
		"class":            sql.NullString{String: "0", Valid: true},
		"friend":           sql.NullString{String: "0", Valid: true},
		"private":          sql.NullString{String: "0", Valid: true},
		"this":             sql.NullString{String: "0", Valid: true},
		"using":            sql.NullString{String: "0", Valid: true},
		"const_cast":       sql.NullString{String: "0", Valid: true},
		"inline":           sql.NullString{String: "0", Valid: true},
		"public":           sql.NullString{String: "0", Valid: true},
		"throw":            sql.NullString{String: "0", Valid: true},
		"virtual":          sql.NullString{String: "0", Valid: true},
		"delete":           sql.NullString{String: "0", Valid: true},
		"mutable":          sql.NullString{String: "0", Valid: true},
		"protected":        sql.NullString{String: "0", Valid: true},
		"true":             sql.NullString{String: "0", Valid: true},
		"wchar_t":          sql.NullString{String: "0", Valid: true},
		"malloc":           sql.NullString{String: "0", Valid: true},
		"calloc":           sql.NullString{String: "0", Valid: true},
		"realloc":          sql.NullString{String: "0", Valid: true},
		"free":             sql.NullString{String: "0", Valid: true},
		"alloca":           sql.NullString{String: "0", Valid: true},
		"alloc":            sql.NullString{String: "0", Valid: true},
	}
	for _, token := range regexp.MustCompile(`\W`).Split(c.Patch, -1) {
		if token == "" {
			continue
		}
		if cntS, ok := keywords[token]; ok {
			cnt, err := strconv.Atoi(cntS.String)
			if err != nil {
				cnt = 0
			}
			cntS.String = strconv.Itoa(cnt + 1)
			keywords[token] = cntS
		}
	}
	c.PatchKeywords.Map = keywords
	return
}
