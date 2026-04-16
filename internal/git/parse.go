package git

import "strconv"

type NumstatEntry struct {
	Additions int64
	Deletions int64
}

type Totals struct {
	Additions int64
	Deletions int64
}

type RawEntry struct {
	Status  string
	OldHash string
	NewHash string
	PathOld string
	PathNew string
}

func parseInt64(s string) (int64, error) {
	if s == "-" {
		return 0, nil
	}
	return strconv.ParseInt(s, 10, 64)
}
