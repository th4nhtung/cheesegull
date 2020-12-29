package models

import (
	"io/ioutil"
	"fmt"
	"strconv"
	"encoding/json"
	"database/sql"
	"net/http"
)

// Beatmap represents a single beatmap (difficulty) on osu!.
type Beatmap struct {
	ID                   int `json:"BeatmapID"`
	ParentSetID          int
	DiffName             string
	FileMD5              string
	Mode                 int
	BPM                  float64
	AR                   float32
	OD                   float32
	CS                   float32
	HP                   float32
	TotalLength          int
	HitLength            int
	CountNormal          int
	CountSlider          int
	CountSpinner         int
	Playcount            int
	Passcount            int
	MaxCombo             int
	DifficultyRating     float64
}

const beatmapFields = `
id, parent_set_id, diff_name, file_md5, mode, bpm,
ar, od, cs, hp, total_length, hit_length,
playcount, passcount, max_combo, difficulty_rating`

func readBeatmapsFromRows(rows *sql.Rows, capacity int) ([]Beatmap, error) {
	var err error
	bms := make([]Beatmap, 0, capacity)
	for rows.Next() {
		var b Beatmap
		err = rows.Scan(
			&b.ID, &b.ParentSetID, &b.DiffName, &b.FileMD5, &b.Mode, &b.BPM,
			&b.AR, &b.OD, &b.CS, &b.HP, &b.TotalLength, &b.HitLength,
			&b.Playcount, &b.Passcount, &b.MaxCombo, &b.DifficultyRating,
		)
		if err != nil {
			return nil, err
		}
		bms = append(bms, b)
	}

	return bms, rows.Err()
}

func inClause(length int) string {
	if length <= 0 {
		return ""
	}
	b := make([]byte, length*3-2)
	for i := 0; i < length; i++ {
		b[i*3] = '?'
		if i != length-1 {
			b[i*3+1] = ','
			b[i*3+2] = ' '
		}
	}
	return string(b)
}

func sIntToSInterface(i []int) []interface{} {
	args := make([]interface{}, len(i))
	for idx, id := range i {
		args[idx] = id
	}
	return args
}

// FetchBeatmaps retrieves a list of beatmap knowing their IDs.
func FetchBeatmaps(db *sql.DB, ids ...int) ([]Beatmap, error) {
	if len(ids) == 0 {
		return nil, nil
	}

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

	bms := make([]Beatmap, 0, len(ids))
	for _, id := range ids {
		req, err := http.Get(fmt.Sprintf("https://old.ppy.sh/api/get_beatmaps?k=%s&b=%d", api_key, id))
		if err != nil {
			return nil, err
		}
		body, err := ioutil.ReadAll(req.Body)
		var data []map[string]interface{}
		if err = json.Unmarshal(body, &data); err != nil || len(data) == 0 {
			return nil, err
		}
		var bm Beatmap
		bm.ID, _                   = strconv.Atoi(data[0]["beatmap_id"].(string))
		bm.ParentSetID, _          = strconv.Atoi(data[0]["beatmapset_id"].(string))
		bm.DiffName                = data[0]["version"].(string)
		bm.FileMD5                 = data[0]["file_md5"].(string)
		bm.Mode, _                 = strconv.Atoi(data[0]["mode"].(string))
		bm.BPM, _                  = strconv.ParseFloat(data[0]["bpm"].(string), 64)
		AR, _                     := strconv.ParseFloat(data[0]["diff_approach"].(string), 32)
		OD, _                     := strconv.ParseFloat(data[0]["diff_overall"].(string), 32)
		CS, _                     := strconv.ParseFloat(data[0]["diff_size"].(string), 32)
		HP, _                     := strconv.ParseFloat(data[0]["diff_drain"].(string), 32)
		bm.AR                      = float32(AR)
		bm.OD                      = float32(OD)
		bm.CS                      = float32(CS)
		bm.HP                      = float32(HP)
		bm.TotalLength, _          = strconv.Atoi(data[0]["total_length"].(string))
		bm.HitLength, _            = strconv.Atoi(data[0]["hit_length"].(string))
		bm.CountNormal, _          = strconv.Atoi(data[0]["count_normal"].(string))
		bm.CountSlider, _          = strconv.Atoi(data[0]["count_slider"].(string))
		bm.CountSpinner, _         = strconv.Atoi(data[0]["count_spinner"].(string))
		bm.Playcount, _            = strconv.Atoi(data[0]["playcount"].(string))
		bm.Passcount, _            = strconv.Atoi(data[0]["passcount"].(string))
		bm.MaxCombo, _             = strconv.Atoi(data[0]["max_combo"].(string))
		bm.DifficultyRating, _     = strconv.ParseFloat(data[0]["difficultyrating"].(string), 64)
		bms = append(bms, bm)
	}
	return bms, nil
}

// CreateBeatmaps adds beatmaps in the database.
func CreateBeatmaps(db *sql.DB, bms ...Beatmap) error {
	if len(bms) == 0 {
		return nil
	}

	q := `INSERT INTO beatmaps(` + beatmapFields + `) VALUES `
	const valuePlaceholder = `(
		?, ?, ?, ?, ?, ?,
		?, ?, ?, ?, ?, ?,
		?, ?, ?, ?
	)`

	args := make([]interface{}, 0, 15*4)
	for idx, bm := range bms {
		if idx != 0 {
			q += ", "
		}
		q += valuePlaceholder
		args = append(args,
			bm.ID, bm.ParentSetID, bm.DiffName, bm.FileMD5, bm.Mode, bm.BPM,
			bm.AR, bm.OD, bm.CS, bm.HP, bm.TotalLength, bm.HitLength,
			bm.Playcount, bm.Passcount, bm.MaxCombo, bm.DifficultyRating,
		)
	}

	_, err := db.Exec(q, args...)
	return err
}
