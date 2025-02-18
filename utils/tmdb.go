package utils

import (
	"errors"
	"math"
	"sort"
	"sync"
	"time"

	tmdb "github.com/cyruzin/golang-tmdb"
)

var (
	tmdbClient *tmdb.Client
	mu         sync.RWMutex
)

type IHateTMdb struct {
	OriginalName string  `json:"original_name"`
	ID           int64   `json:"id"`
	GenreIDs     []int64 `json:"genre_ids"`
}

func SearchForShow(search string, date time.Time) (*tmdb.TVDetails, error) {
	client := TMDBClient()
	if client == nil {
		return nil, errors.New("TMDB client is nil")
	}

	options := make(map[string]string)
	options["language"] = "en-US"

	shows, err := client.GetSearchTVShow(search, options)
	if err != nil {
		return nil, err
	}
	if shows == nil || shows.Results == nil || len(shows.Results) == 0 {
		return nil, errors.New("no results found")
	}

	sort.Slice(shows.Results, func(i, j int) bool {
		dateI, errI := ParseDate(shows.Results[i].FirstAirDate)
		dateJ, errJ := ParseDate(shows.Results[j].FirstAirDate)
		if errI != nil || errJ != nil {
			return false
		}
		diffI := math.Abs(date.Sub(dateI).Hours())
		diffJ := math.Abs(date.Sub(dateJ).Hours())
		return diffI < diffJ
	})

	var showFound *IHateTMdb
	for _, result := range shows.Results {
		details, err := client.GetTVDetails(int(result.ID), options)
		if err != nil || details == nil || details.Genres == nil {
			continue
		}
		for _, genre := range details.Genres {
			if genre.Name == "Animation" {
				showFound = &IHateTMdb{
					OriginalName: result.Name,
					ID:           result.ID,
				}
				break
			}
		}
		if showFound != nil {
			break
		}
	}

	if showFound == nil {
		return nil, errors.New("no animation genre found")
	}

	options["language"] = "fr-FR"
	show, err := client.GetTVDetails(int(showFound.ID), options)
	if err != nil {
		return nil, err
	}
	if show == nil {
		return nil, errors.New("failed to retrieve show details")
	}

	show.Name = showFound.OriginalName

	return show, nil
}

func GetTrailerForShow(tmdbId int64) string {
	client := TMDBClient()

	videos, err := client.GetTVVideos(int(tmdbId), nil)
	if err != nil {
		return ""
	}

	for _, video := range videos.Results {
		if video.Site == "YouTube" && video.Type == "Trailer" {
			return tmdb.GetVideoURL(video.Key)
		}
	}

	return ""
}

func ShowById(tmdbId int64) (*tmdb.TVDetails, error) {
	client := TMDBClient()

	options := make(map[string]string)
	options["language"] = "fr-FR"

	show, err := client.GetTVDetails(int(tmdbId), options)
	if err != nil {
		return nil, err
	}

	return show, nil
}

func TMDBClient() *tmdb.Client {
	mu.RLock()
	defer mu.RUnlock()

	if tmdbClient == nil {
		tc, err := tmdb.Init("22e1cee5a6f13b16922d0150df043f99")
		if err != nil {
			panic(err)
		}

		tmdbClient = tc
	}

	tmdbClient.SetClientAutoRetry()

	return tmdbClient
}

func ParseDate(dateStr string) (time.Time, error) {
	formats := []string{"2006-01-02", "2006-01-02T15:04:05Z07:00", "Jan 2006", "2006", "02 Jan 2006"}
	var err error
	var date time.Time
	for _, format := range formats {
		date, err = time.Parse(format, dateStr)
		if err == nil {
			return date, nil
		}
	}
	return date, err
}
