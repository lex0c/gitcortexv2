package stats

import (
	"fmt"
	"math"
	"sort"
)

type ContributorStat struct {
	Name      string
	Email     string
	Commits   int
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
	Commits   int
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

type CouplingResult struct {
	FileA       string
	FileB       string
	CoChanges   int
	CouplingPct float64
	ChangesA    int
	ChangesB    int
}

type ChurnRiskResult struct {
	Path           string
	RecentChurn    float64
	BusFactor      int
	RiskScore      float64
	TotalChanges   int
	LastChangeDate string
}

type WorkingPattern struct {
	Hour    int
	Day     string
	Commits int
}

type DevEdge struct {
	DevA        string
	DevB        string
	SharedFiles int
	Weight      float64
}

// --- Stats from pre-aggregated Dataset ---

func ComputeSummary(ds *Dataset) Summary {
	s := Summary{
		TotalCommits:   ds.CommitCount,
		TotalDevs:      ds.DevCount,
		TotalFiles:     ds.UniqueFileCount,
		TotalAdditions: ds.TotalAdditions,
		TotalDeletions: ds.TotalDeletions,
		MergeCommits:   ds.MergeCount,
	}

	if s.TotalCommits > 0 {
		s.AvgAdditions = float64(ds.TotalAdditions) / float64(s.TotalCommits)
		s.AvgDeletions = float64(ds.TotalDeletions) / float64(s.TotalCommits)
		s.AvgFilesChanged = float64(ds.TotalFilesChanged) / float64(s.TotalCommits)
	}

	if !ds.Earliest.IsZero() {
		s.FirstCommitDate = ds.Earliest.Format("2006-01-02")
	}
	if !ds.Latest.IsZero() {
		s.LastCommitDate = ds.Latest.Format("2006-01-02")
	}

	return s
}

func TopContributors(ds *Dataset, n int) []ContributorStat {
	result := make([]ContributorStat, 0, len(ds.contributors))
	for _, cs := range ds.contributors {
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
	result := make([]FileStat, 0, len(ds.files))
	for path, fe := range ds.files {
		result = append(result, FileStat{
			Path:       path,
			Commits:    fe.commits,
			Additions:  fe.additions,
			Deletions:  fe.deletions,
			Churn:      fe.additions + fe.deletions,
			UniqueDevs: len(fe.devLines),
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

	for _, cm := range ds.commits {
		if cm.date.IsZero() {
			continue
		}

		var key string
		switch granularity {
		case "day":
			key = cm.date.Format("2006-01-02")
		case "week":
			y, w := cm.date.ISOWeek()
			key = fmt.Sprintf("%04d-W%02d", y, w)
		case "year":
			key = cm.date.Format("2006")
		default:
			key = cm.date.Format("2006-01")
		}

		b, ok := buckets[key]
		if !ok {
			b = &ActivityBucket{Period: key}
			buckets[key] = b
			order = append(order, key)
		}
		b.Commits++
		b.Additions += cm.add
		b.Deletions += cm.del
	}

	sort.Strings(order)
	result := make([]ActivityBucket, len(order))
	for i, key := range order {
		result[i] = *buckets[key]
	}
	return result
}

func BusFactor(ds *Dataset, n int) []BusFactorResult {
	type devLines struct {
		email string
		lines int64
	}

	var result []BusFactorResult

	for path, fe := range ds.files {
		if len(fe.devLines) == 0 {
			continue
		}

		devs := make([]devLines, 0, len(fe.devLines))
		var totalLines int64
		for email, lines := range fe.devLines {
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

func FileCoupling(ds *Dataset, n, _, minCoChanges int) []CouplingResult {
	var results []CouplingResult

	for p, count := range ds.couplingPairs {
		if count < minCoChanges {
			continue
		}

		ca := ds.couplingFileChanges[p.a]
		cb := ds.couplingFileChanges[p.b]
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

func ChurnRisk(ds *Dataset, n, _ int) []ChurnRiskResult {
	var results []ChurnRiskResult

	for path, fe := range ds.files {
		bf := len(fe.devLines)
		if bf < 1 {
			bf = 1
		}

		risk := fe.recentChurn / float64(bf)

		lastDate := ""
		if !fe.lastChange.IsZero() {
			lastDate = fe.lastChange.Format("2006-01-02")
		}

		results = append(results, ChurnRiskResult{
			Path:           path,
			RecentChurn:    math.Round(fe.recentChurn*10) / 10,
			BusFactor:      len(fe.devLines),
			RiskScore:      math.Round(risk*10) / 10,
			TotalChanges:   fe.commits,
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

func WorkingPatterns(ds *Dataset) []WorkingPattern {
	days := []string{"Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"}
	var results []WorkingPattern

	for d := 0; d < 7; d++ {
		for h := 0; h < 24; h++ {
			if ds.workGrid[d][h] > 0 {
				results = append(results, WorkingPattern{
					Hour:    h,
					Day:     days[d],
					Commits: ds.workGrid[d][h],
				})
			}
		}
	}
	return results
}

func DeveloperNetwork(ds *Dataset, n, minSharedFiles int) []DevEdge {
	type devPair struct{ a, b string }
	pairFiles := make(map[devPair]int)
	devFileCount := make(map[string]int)

	for _, fe := range ds.files {
		devs := make([]string, 0, len(fe.devLines))
		for email := range fe.devLines {
			devs = append(devs, email)
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
