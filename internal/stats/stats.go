package stats

import (
	"fmt"
	"math"
	"sort"
	"time"
)

type ContributorStat struct {
	Name     string
	Email    string
	Commits  int
	Additions int64
	Deletions int64
}

type FileStat struct {
	Path       string
	Commits    int
	Additions  int64
	Deletions  int64
	Churn      int64
	UniqueDevs int
}

type ActivityBucket struct {
	Period    string
	Commits  int
	Additions int64
	Deletions int64
}

type Summary struct {
	TotalCommits    int
	TotalDevs       int
	TotalFiles      int
	TotalAdditions  int64
	TotalDeletions  int64
	MergeCommits    int
	AvgAdditions    float64
	AvgDeletions    float64
	AvgFilesChanged float64
	FirstCommitDate string
	LastCommitDate  string
}

type BusFactorResult struct {
	Path      string
	BusFactor int
	TopDevs   []string
}

func ComputeSummary(ds *Dataset) Summary {
	s := Summary{
		TotalCommits: len(ds.Commits),
		TotalDevs:    len(ds.Devs),
	}

	uniqueFiles := make(map[string]struct{})
	parentCount := make(map[string]int)
	for _, p := range ds.Parents {
		parentCount[p.SHA]++
	}

	var totalFiles int64
	for _, c := range ds.Commits {
		s.TotalAdditions += c.Additions
		s.TotalDeletions += c.Deletions
		totalFiles += int64(c.FilesChanged)
		if parentCount[c.SHA] > 1 {
			s.MergeCommits++
		}
	}

	for _, f := range ds.Files {
		if f.PathCurrent != "" {
			uniqueFiles[f.PathCurrent] = struct{}{}
		}
	}
	s.TotalFiles = len(uniqueFiles)

	if s.TotalCommits > 0 {
		s.AvgAdditions = float64(s.TotalAdditions) / float64(s.TotalCommits)
		s.AvgDeletions = float64(s.TotalDeletions) / float64(s.TotalCommits)
		s.AvgFilesChanged = float64(totalFiles) / float64(s.TotalCommits)
	}

	if len(ds.Commits) > 0 {
		var earliest, latest time.Time
		for _, c := range ds.Commits {
			t := parseDate(c.AuthorDate)
			if t.IsZero() {
				continue
			}
			if earliest.IsZero() || t.Before(earliest) {
				earliest = t
			}
			if latest.IsZero() || t.After(latest) {
				latest = t
			}
		}
		if !earliest.IsZero() {
			s.FirstCommitDate = earliest.Format("2006-01-02")
		}
		if !latest.IsZero() {
			s.LastCommitDate = latest.Format("2006-01-02")
		}
	}

	return s
}

func TopContributors(ds *Dataset, n int) []ContributorStat {
	byEmail := make(map[string]*ContributorStat)

	for _, c := range ds.Commits {
		key := c.AuthorEmail
		cs, ok := byEmail[key]
		if !ok {
			cs = &ContributorStat{Name: c.AuthorName, Email: c.AuthorEmail}
			byEmail[key] = cs
		}
		cs.Commits++
		cs.Additions += c.Additions
		cs.Deletions += c.Deletions
	}

	result := make([]ContributorStat, 0, len(byEmail))
	for _, cs := range byEmail {
		result = append(result, *cs)
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].Commits > result[j].Commits
	})

	if n > 0 && n < len(result) {
		result = result[:n]
	}

	return result
}

func FileHotspots(ds *Dataset, n int) []FileStat {
	type fileAcc struct {
		commits   int
		additions int64
		deletions int64
		devs      map[string]struct{}
	}

	byPath := make(map[string]*fileAcc)

	commitAuthor := make(map[string]string)
	for _, c := range ds.Commits {
		commitAuthor[c.SHA] = c.AuthorEmail
	}

	for _, f := range ds.Files {
		path := f.PathCurrent
		if path == "" {
			path = f.PathPrevious
		}
		acc, ok := byPath[path]
		if !ok {
			acc = &fileAcc{devs: make(map[string]struct{})}
			byPath[path] = acc
		}
		acc.commits++
		acc.additions += f.Additions
		acc.deletions += f.Deletions
		if email, ok := commitAuthor[f.Commit]; ok {
			acc.devs[email] = struct{}{}
		}
	}

	result := make([]FileStat, 0, len(byPath))
	for path, acc := range byPath {
		result = append(result, FileStat{
			Path:       path,
			Commits:    acc.commits,
			Additions:  acc.additions,
			Deletions:  acc.deletions,
			Churn:      acc.additions + acc.deletions,
			UniqueDevs: len(acc.devs),
		})
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].Commits > result[j].Commits
	})

	if n > 0 && n < len(result) {
		result = result[:n]
	}

	return result
}

func ActivityOverTime(ds *Dataset, granularity string) []ActivityBucket {
	buckets := make(map[string]*ActivityBucket)
	var order []string

	for _, c := range ds.Commits {
		t := parseDate(c.AuthorDate)
		if t.IsZero() {
			continue
		}

		var key string
		switch granularity {
		case "day":
			key = t.Format("2006-01-02")
		case "week":
			y, w := t.ISOWeek()
			key = fmt.Sprintf("%04d-W%02d", y, w)
		case "year":
			key = t.Format("2006")
		default:
			key = t.Format("2006-01")
		}

		b, ok := buckets[key]
		if !ok {
			b = &ActivityBucket{Period: key}
			buckets[key] = b
			order = append(order, key)
		}
		b.Commits++
		b.Additions += c.Additions
		b.Deletions += c.Deletions
	}

	sort.Strings(order)
	result := make([]ActivityBucket, len(order))
	for i, key := range order {
		result[i] = *buckets[key]
	}

	return result
}

func BusFactor(ds *Dataset, n int) []BusFactorResult {
	commitAuthor := make(map[string]string)
	for _, c := range ds.Commits {
		commitAuthor[c.SHA] = c.AuthorEmail
	}

	type devLines struct {
		email string
		lines int64
	}

	fileDevs := make(map[string]map[string]int64)

	for _, f := range ds.Files {
		path := f.PathCurrent
		if path == "" {
			continue
		}
		email, ok := commitAuthor[f.Commit]
		if !ok {
			continue
		}
		if fileDevs[path] == nil {
			fileDevs[path] = make(map[string]int64)
		}
		fileDevs[path][email] += f.Additions + f.Deletions
	}

	result := make([]BusFactorResult, 0, len(fileDevs))

	for path, devMap := range fileDevs {
		if len(devMap) == 0 {
			continue
		}

		devs := make([]devLines, 0, len(devMap))
		var totalLines int64
		for email, lines := range devMap {
			devs = append(devs, devLines{email: email, lines: lines})
			totalLines += lines
		}

		sort.Slice(devs, func(i, j int) bool {
			return devs[i].lines > devs[j].lines
		})

		threshold := float64(totalLines) * 0.8
		var cumulative int64
		busFactor := 0
		var topDevs []string
		for _, d := range devs {
			cumulative += d.lines
			busFactor++
			topDevs = append(topDevs, d.email)
			if float64(cumulative) >= threshold {
				break
			}
		}

		result = append(result, BusFactorResult{
			Path:      path,
			BusFactor: busFactor,
			TopDevs:   topDevs,
		})
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].BusFactor < result[j].BusFactor
	})

	if n > 0 && n < len(result) {
		result = result[:n]
	}

	return result
}

type CouplingResult struct {
	FileA       string
	FileB       string
	CoChanges   int
	CouplingPct float64
	ChangesA    int
	ChangesB    int
}

// FileCoupling finds files that frequently change together in the same commits.
// maxFilesPerCommit filters out bulk commits (renames, imports) that add noise.
// minCoChanges sets the minimum co-occurrence threshold.
func FileCoupling(ds *Dataset, n, maxFilesPerCommit, minCoChanges int) []CouplingResult {
	commitFiles := make(map[string][]string)
	for _, f := range ds.Files {
		path := f.PathCurrent
		if path == "" {
			path = f.PathPrevious
		}
		if path == "" {
			continue
		}
		commitFiles[f.Commit] = append(commitFiles[f.Commit], path)
	}

	fileChanges := make(map[string]int)
	type pair struct{ a, b string }
	pairCount := make(map[pair]int)

	for _, files := range commitFiles {
		if len(files) < 2 || len(files) > maxFilesPerCommit {
			continue
		}

		seen := make(map[string]bool, len(files))
		unique := make([]string, 0, len(files))
		for _, f := range files {
			if !seen[f] {
				seen[f] = true
				unique = append(unique, f)
				fileChanges[f]++
			}
		}

		for i := 0; i < len(unique); i++ {
			for j := i + 1; j < len(unique); j++ {
				a, b := unique[i], unique[j]
				if a > b {
					a, b = b, a
				}
				pairCount[pair{a, b}]++
			}
		}
	}

	var results []CouplingResult
	for p, count := range pairCount {
		if count < minCoChanges {
			continue
		}

		ca, cb := fileChanges[p.a], fileChanges[p.b]
		denom := ca
		if cb < denom {
			denom = cb
		}

		pct := 0.0
		if denom > 0 {
			pct = float64(count) / float64(denom) * 100
		}

		results = append(results, CouplingResult{
			FileA:       p.a,
			FileB:       p.b,
			CoChanges:   count,
			CouplingPct: pct,
			ChangesA:    ca,
			ChangesB:    cb,
		})
	}

	sort.Slice(results, func(i, j int) bool {
		if results[i].CoChanges != results[j].CoChanges {
			return results[i].CoChanges > results[j].CoChanges
		}
		return results[i].CouplingPct > results[j].CouplingPct
	})

	if n > 0 && n < len(results) {
		results = results[:n]
	}

	return results
}

// --- Churn Risk ---

type ChurnRiskResult struct {
	Path           string
	RecentChurn    float64
	BusFactor      int
	RiskScore      float64
	TotalChanges   int
	LastChangeDate string
}

// ChurnRisk ranks files by recency-weighted churn combined with bus factor.
// halfLifeDays controls how fast old changes decay (default 90 = changes lose
// half their weight every 90 days).
func ChurnRisk(ds *Dataset, n, halfLifeDays int) []ChurnRiskResult {
	now := time.Now()
	lambda := math.Ln2 / float64(halfLifeDays)

	commitDates := make(map[string]time.Time)
	commitAuthors := make(map[string]string)
	for _, c := range ds.Commits {
		t := parseDate(c.AuthorDate)
		if !t.IsZero() {
			commitDates[c.SHA] = t
		}
		commitAuthors[c.SHA] = c.AuthorEmail
	}

	type fileAcc struct {
		recentChurn float64
		changes     int
		lastChange  time.Time
		devs        map[string]struct{}
	}

	byPath := make(map[string]*fileAcc)

	for _, f := range ds.Files {
		path := f.PathCurrent
		if path == "" {
			continue
		}

		acc, ok := byPath[path]
		if !ok {
			acc = &fileAcc{devs: make(map[string]struct{})}
			byPath[path] = acc
		}

		acc.changes++
		churn := float64(f.Additions + f.Deletions)

		t, hasDate := commitDates[f.Commit]
		if hasDate {
			days := now.Sub(t).Hours() / 24
			weight := math.Exp(-lambda * days)
			acc.recentChurn += churn * weight
			if t.After(acc.lastChange) {
				acc.lastChange = t
			}
		}

		if email, ok := commitAuthors[f.Commit]; ok {
			acc.devs[email] = struct{}{}
		}
	}

	var results []ChurnRiskResult
	for path, acc := range byPath {
		bf := len(acc.devs)
		if bf < 1 {
			bf = 1
		}

		risk := acc.recentChurn / float64(bf)

		lastDate := ""
		if !acc.lastChange.IsZero() {
			lastDate = acc.lastChange.Format("2006-01-02")
		}

		results = append(results, ChurnRiskResult{
			Path:           path,
			RecentChurn:    math.Round(acc.recentChurn*10) / 10,
			BusFactor:      len(acc.devs),
			RiskScore:      math.Round(risk*10) / 10,
			TotalChanges:   acc.changes,
			LastChangeDate: lastDate,
		})
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].RiskScore > results[j].RiskScore
	})

	if n > 0 && n < len(results) {
		results = results[:n]
	}

	return results
}

// --- Working Patterns ---

type WorkingPattern struct {
	Hour     int
	Day      string
	Commits  int
}

func WorkingPatterns(ds *Dataset) []WorkingPattern {
	days := []string{"Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"}
	dayIndex := map[time.Weekday]int{
		time.Monday: 0, time.Tuesday: 1, time.Wednesday: 2,
		time.Thursday: 3, time.Friday: 4, time.Saturday: 5, time.Sunday: 6,
	}

	grid := [7][24]int{}

	for _, c := range ds.Commits {
		t := parseDate(c.AuthorDate)
		if t.IsZero() {
			continue
		}
		di := dayIndex[t.Weekday()]
		grid[di][t.Hour()]++
	}

	var results []WorkingPattern
	for d := 0; d < 7; d++ {
		for h := 0; h < 24; h++ {
			if grid[d][h] > 0 {
				results = append(results, WorkingPattern{
					Hour:    h,
					Day:     days[d],
					Commits: grid[d][h],
				})
			}
		}
	}

	return results
}

// --- Developer Network ---

type DevEdge struct {
	DevA        string
	DevB        string
	SharedFiles int
	Weight      float64
}

// DeveloperNetwork builds a collaboration graph where edges connect developers
// who modified the same files. Weight = shared files / max(files_A, files_B).
func DeveloperNetwork(ds *Dataset, n, minSharedFiles int) []DevEdge {
	commitAuthor := make(map[string]string)
	for _, c := range ds.Commits {
		commitAuthor[c.SHA] = c.AuthorEmail
	}

	fileDevs := make(map[string]map[string]struct{})
	for _, f := range ds.Files {
		path := f.PathCurrent
		if path == "" {
			continue
		}
		email, ok := commitAuthor[f.Commit]
		if !ok {
			continue
		}
		if fileDevs[path] == nil {
			fileDevs[path] = make(map[string]struct{})
		}
		fileDevs[path][email] = struct{}{}
	}

	type devPair struct{ a, b string }
	pairFiles := make(map[devPair]int)
	devFileCount := make(map[string]int)

	for _, devSet := range fileDevs {
		devs := make([]string, 0, len(devSet))
		for d := range devSet {
			devs = append(devs, d)
		}
		for _, d := range devs {
			devFileCount[d]++
		}
		for i := 0; i < len(devs); i++ {
			for j := i + 1; j < len(devs); j++ {
				a, b := devs[i], devs[j]
				if a > b {
					a, b = b, a
				}
				pairFiles[devPair{a, b}]++
			}
		}
	}

	var results []DevEdge
	for p, shared := range pairFiles {
		if shared < minSharedFiles {
			continue
		}
		maxFiles := devFileCount[p.a]
		if devFileCount[p.b] > maxFiles {
			maxFiles = devFileCount[p.b]
		}
		weight := 0.0
		if maxFiles > 0 {
			weight = float64(shared) / float64(maxFiles) * 100
		}

		results = append(results, DevEdge{
			DevA:        p.a,
			DevB:        p.b,
			SharedFiles: shared,
			Weight:      math.Round(weight*10) / 10,
		})
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].SharedFiles > results[j].SharedFiles
	})

	if n > 0 && n < len(results) {
		results = results[:n]
	}

	return results
}

// --- Period Filtering ---

// FilterByDateRange returns a new Dataset containing only records within the date range.
func FilterByDateRange(ds *Dataset, from, to string) *Dataset {
	fromTime := parseDate(from + "T00:00:00Z")
	toTime := parseDate(to + "T23:59:59Z")

	if fromTime.IsZero() && toTime.IsZero() {
		return ds
	}

	commitSet := make(map[string]bool)
	var filtered Dataset

	for _, c := range ds.Commits {
		t := parseDate(c.AuthorDate)
		if t.IsZero() {
			continue
		}
		if !fromTime.IsZero() && t.Before(fromTime) {
			continue
		}
		if !toTime.IsZero() && t.After(toTime) {
			continue
		}
		filtered.Commits = append(filtered.Commits, c)
		commitSet[c.SHA] = true
	}

	for _, f := range ds.Files {
		if commitSet[f.Commit] {
			filtered.Files = append(filtered.Files, f)
		}
	}
	for _, p := range ds.Parents {
		if commitSet[p.SHA] {
			filtered.Parents = append(filtered.Parents, p)
		}
	}

	devSeen := make(map[string]bool)
	for _, d := range ds.Devs {
		if !devSeen[d.DevID] {
			devSeen[d.DevID] = true
			filtered.Devs = append(filtered.Devs, d)
		}
	}

	return &filtered
}

