package models

import (
	"bytes"
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"encoding/hex"
	"net/url"
	"net/http"
	"io/ioutil"
	"time"
)

// SearchOptions are options that can be passed to SearchSets for filtering
// sets.
type SearchOptions struct {
	// If len is 0, then it should be treated as if all statuses are good.
	Status []int
	Query  string
	// Gamemodes to which limit the results. If len is 0, it means all modes
	// are ok.
	Mode []int

	// Pagination options.
	Offset int
	Amount int
}

func (o SearchOptions) setModes() (total uint8) {
	for _, m := range o.Mode {
		if m < 0 || m >= 4 {
			continue
		}
		total |= 1 << uint8(m)
	}
	return
}

var mysqlStringReplacer = strings.NewReplacer(
	`\`, `\\`,
	`"`, `\"`,
	`'`, `\'`,
	"\x00", `\0`,
	"\n", `\n`,
	"\r", `\r`,
	"\x1a", `\Z`,
)

func sIntCommaSeparated(nums []int) string {
	b := bytes.Buffer{}
	for idx, num := range nums {
		b.WriteString(strconv.Itoa(num))
		if idx != len(nums)-1 {
			b.WriteString(", ")
		}
	}
	return b.String()
}

func getAccount(db *sql.DB) (u, p string) {
	var md5, salt string
	q := `SELECT username, password_md5, salt FROM users WHERE id = 1`

	rows, err := db.Query(q)
	if err != nil {
		return "", ""
	}

	rows.Next()
	if err := rows.Scan(&u, &md5, &salt); err != nil {
		return "", ""
	}
	enc, _ := hex.DecodeString(md5)
	dec, _ := decrypt([]byte(salt), enc)
	p = string(dec)
	return u, p
}

func apiToDirectStatus(apiStatus int) int {
	if apiStatus == 1 {
		return 0
	} else if apiStatus == 2 {
		return 7
	} else if apiStatus == 4 {
		return 8
	} else if apiStatus == 3 {
		return 3
	} else if apiStatus == 0 {
		return 2
	} else if apiStatus == -2 {
		return 5
	} else {
		return 0
	}
}

// SearchSets retrieves sets, filtering them using SearchOptions.
func SearchSets(db, searchDB *sql.DB, opts SearchOptions) ([]Set, error) {
	osuUsername, osuPassword := getAccount(db)
	md5Password := hash(osuPassword)

	//c, err := downloader.LogIn(*osuUsername, *osuPassword)
	//sm := strconv.Itoa(int(opts.setModes()))

	sets := make([]Set, 0, opts.Amount)
	if len(opts.Status) < 2 && len(opts.Mode) < 2 {
		search_uri := fmt.Sprintf("https://old.ppy.sh/web/osu-search.php?u=%s&h=%s&p=%d", osuUsername, md5Password, opts.Offset / 100)
		if opts.Query != "" {
			search_uri += "&q=" + url.QueryEscape(opts.Query)
		} else {
			search_uri += "&q=Newest"
		}
		if len(opts.Status) > 0 {
			status := apiToDirectStatus(opts.Status[0])
			search_uri += "&r=" + url.QueryEscape(strconv.Itoa(status))
		} else {
			search_uri += "&r=0"
		}
		if len(opts.Mode) > 0 {
			search_uri += "&m=" + url.QueryEscape(strconv.Itoa(opts.Mode[0]))
		} else {
			search_uri += "&m=0"
		}
		req, err := http.Get(search_uri)
		if err != nil {
			return nil, err
		}
		body, err := ioutil.ReadAll(req.Body)
		for _, line := range strings.Split(strings.TrimSuffix(string(body), "\n"), "\n") {
			cp := strings.SplitN(line, "|", 14)
			if (len(cp) == 1) {
				continue
			}
			for i, v := range cp {
				if v == "" {
					cp[i] = "0"
				}
			}
			var s Set
			s.Artist           = cp[1]
			s.Title            = cp[2]
			s.Creator          = cp[3]
			s.RankedStatus, _  = strconv.Atoi(cp[4])
			rating, _         := strconv.ParseFloat(cp[5], 32)
			s.Rating           = float32(rating)
			date_str, _       := time.Parse("2006-01-02T15:04:05Z07:00", cp[6])
			if s.RankedStatus > 0 { // 4: loved | 3: qualified | 2: approved | 1: ranked
				s.ApprovedDate = date_str
			} else { // 0: pending | -1: WIP | -2: graveyard
				s.LastUpdate   = date_str
			}
			s.ID, _            = strconv.Atoi(cp[7])
			s.TopicID, _       = strconv.Atoi(cp[8])
			s.HasVideo, _      = strconv.ParseBool(cp[9])
			s.HasStoryboard, _ = strconv.ParseBool(cp[10])
			bids := strings.Split(cp[13], ",")
			children := make([]Beatmap, 0, len(bids))
			for _, v := range bids {
				var b Beatmap
				b.ParentSetID = s.ID
				ar := strings.Split(v, "★")
				if (len(ar) < 2) {
					continue
				}
				b.DiffName = strings.Trim(ar[0], " ")
				fmt.Sscanf(ar[1], "%f@%d", &b.DifficultyRating, &b.Mode)
				b.BPM = -1
				children = append(children, b)
			}
			s.ChildrenBeatmaps = children
			sets = append(sets, s)
		}
	}

	return sets, nil
}
