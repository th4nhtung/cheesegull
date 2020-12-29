package models

import (
	"io/ioutil"
	"fmt"
	"strconv"
	"encoding/json"
	"net/http"
	"database/sql"
	"time"
)

// Set represents a set of beatmaps usually sharing the same song.
type Set struct {
	ID                     int `json:"SetID"`
	ChildrenBeatmaps       []Beatmap
	TopicID                int
	RankedStatus           int
	SubmitDate             time.Time
	ApprovedDate           time.Time
	LastUpdate             time.Time
	LastChecked            time.Time
	Artist                 string
	ArtistUnicode          string
	Title                  string
	TitleUnicode           string
	Creator                string
	Source                 string
	Tags                   string
	HasVideo               bool
	HasStoryboard          bool
	DownloadUnavailable    bool
	AudioUnavailable       bool
	Genre                  int
	Language               int
	Favourites             int
	Rating                 float32
}

const setFields = `id, ranked_status, approved_date, last_update, last_checked,
artist, title, creator, source, tags, has_video, genre,
language, favourites`

// FetchSetsForBatchUpdate fetches limit sets from the database, sorted by
// LastChecked (asc, older first). Results are further filtered: if the set's
// RankedStatus is 3, 0 or -1 (qualified, pending or WIP), at least 30 minutes
// must have passed from LastChecked. For all other statuses, at least 4 days
// must have passed from LastChecked.
func FetchSetsForBatchUpdate(db *sql.DB, limit int) ([]Set, error) {
	n := time.Now()
	rows, err := db.Query(`
SELECT `+setFields+` FROM sets
WHERE (ranked_status IN (3, 0, -1) AND last_checked <= ?) OR last_checked <= ?
ORDER BY last_checked ASC
LIMIT ?`,
		n.Add(-time.Minute*30),
		n.Add(-time.Hour*24*4),
		limit,
	)
	if err != nil {
		return nil, err
	}

	sets := make([]Set, 0, limit)
	for rows.Next() {
		var s Set
		err = rows.Scan(
			&s.ID, &s.RankedStatus, &s.ApprovedDate, &s.LastUpdate, &s.LastChecked,
			&s.Artist, &s.Title, &s.Creator, &s.Source, &s.Tags, &s.HasVideo, &s.Genre,
			&s.Language, &s.Favourites,
		)
		if err != nil {
			return nil, err
		}
		sets = append(sets, s)
	}

	return sets, rows.Err()
}

func parseOsuDatetime(str string) time.Time {
	t, _ := time.Parse("2006-01-02 15:04:05", str)
	return t
}

// FetchSet retrieves a single set to show, alongside its children beatmaps.
func FetchSet(db *sql.DB, id int, withChildren bool) (*Set, error) {
	var s Set
	q := `SELECT email FROM users WHERE id = 1`

	rows, err := db.Query(q)
	if err != nil {
		return nil, err
	}

	var api_key string
	rows.Next()
	if err := rows.Scan(&api_key); err != nil {
		return nil, err
	}

	req, err := http.Get(fmt.Sprintf("https://old.ppy.sh/api/get_beatmaps?k=%s&s=%d", api_key, id))
	if err != nil {
		return nil, err
	}
	body, err := ioutil.ReadAll(req.Body)
	var data []map[string]interface{}
	if err = json.Unmarshal(body, &data); err != nil || len(data) == 0 {
		return nil, err
	}
	s.ID, _                     = strconv.Atoi(data[0]["beatmapset_id"].(string))
	s.RankedStatus, _           = strconv.Atoi(data[0]["approved"].(string))
	str, ok := data[0]["approved_date"].(string)
	if ok {
		s.ApprovedDate          = parseOsuDatetime(str)
	} else {
		s.ApprovedDate          = time.Time{}
	}
	s.SubmitDate                = parseOsuDatetime(data[0]["submit_date"].(string))
	s.LastUpdate                = parseOsuDatetime(data[0]["last_update"].(string))
	s.LastChecked               = time.Now().UTC().Truncate(time.Second)
	s.Artist                    = data[0]["artist"].(string)
	str, ok = data[0]["artist_unicode"].(string)
	if ok {
		s.ArtistUnicode         = str
	} else {
		s.ArtistUnicode         = ""
	}
	s.Title                     = data[0]["title"].(string)
	str, ok = data[0]["title_unicode"].(string)
	if ok {
		s.TitleUnicode          = str
	} else {
		s.TitleUnicode          = ""
	}
	s.Creator                   = data[0]["creator"].(string)
	s.Source                    = data[0]["source"].(string)
	s.Tags                      = data[0]["tags"].(string)
	s.HasVideo, _               = strconv.ParseBool(data[0]["video"].(string))
	s.HasStoryboard, _          = strconv.ParseBool(data[0]["storyboard"].(string))
	s.DownloadUnavailable, _    = strconv.ParseBool(data[0]["download_unavailable"].(string))
	s.AudioUnavailable, _       = strconv.ParseBool(data[0]["audio_unavailable"].(string))
	s.Genre, _                  = strconv.Atoi(data[0]["genre_id"].(string))
	s.Language, _               = strconv.Atoi(data[0]["language_id"].(string))
	s.Favourites, _             = strconv.Atoi(data[0]["favourite_count"].(string))
	rating, _                  := strconv.ParseFloat(data[0]["rating"].(string), 32)
	s.Rating                    = float32(rating)
	if withChildren {
		for i, ids := 0, make([]int, 0, 8); i <= len(data); i++ {
			if (i == len(data)) {
				s.ChildrenBeatmaps, err = FetchBeatmaps(db, ids...)
				if err != nil {
					return nil, err
				}
			} else {
				id, _ := strconv.Atoi(data[i]["beatmap_id"].(string))
				ids = append(ids, id)
			}
		}
	}
	return &s, err
}

// DeleteSet deletes a set from the database, removing also its children
// beatmaps.
func DeleteSet(db *sql.DB, set int) error {
	_, err := db.Exec("DELETE FROM beatmaps WHERE parent_set_id = ?", set)
	if err != nil {
		return err
	}
	_, err = db.Exec("DELETE FROM sets WHERE id = ?", set)
	return err
}

// createSetModes will generate the correct value for setModes, which is
// basically a bitwise enum containing the modes that are on a certain set.
func createSetModes(bms []Beatmap) (setModes uint8) {
	for _, bm := range bms {
		m := bm.Mode
		if m < 0 || m >= 4 {
			continue
		}
		setModes |= 1 << uint(m)
	}
	return setModes
}

// CreateSet creates (and updates) a beatmap set in the database.
func CreateSet(db *sql.DB, s Set) error {
	// delete existing set, if any.
	// This is mostly a lazy way to make sure updates work as well.
	err := DeleteSet(db, s.ID)
	if err != nil {
		return err
	}

	_, err = db.Exec(`
INSERT INTO sets(
	id, ranked_status, approved_date, last_update, last_checked,
	artist, title, creator, source, tags, has_video, genre,
	language, favourites, set_modes
)
VALUES (
	?, ?, ?, ?, ?,
	?, ?, ?, ?, ?, ?, ?,
	?, ?, ?
)`, s.ID, s.RankedStatus, s.ApprovedDate, s.LastUpdate, s.LastChecked,
		s.Artist, s.Title, s.Creator, s.Source, s.Tags, s.HasVideo, s.Genre,
		s.Language, s.Favourites, createSetModes(s.ChildrenBeatmaps))
	if err != nil {
		return err
	}

	return CreateBeatmaps(db, s.ChildrenBeatmaps...)
}

// BiggestSetID retrieves the biggest set ID in the sets database. This is used
// by discovery to have a starting point from which to discover new beatmaps.
func BiggestSetID(db *sql.DB) (int, error) {
	var i int
	err := db.QueryRow("SELECT id FROM sets ORDER BY id DESC LIMIT 1").Scan(&i)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return i, err
}
